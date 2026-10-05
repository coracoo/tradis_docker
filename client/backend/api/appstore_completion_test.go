package api

import (
	"context"
	"errors"
	"os"
	"testing"

	"dockerpanel/backend/pkg/database"
	"dockerpanel/backend/pkg/deployment"
	"github.com/stretchr/testify/require"
)

func TestAppStoreDeployDoesNotReportUnverifiedCommandAsSuccess(t *testing.T) {
	root, _ := setupAppStoreDeployContractTest(t, map[string]App{
		"verify-app": {ID: 78, Name: "Verify App", Compose: "services:\n  web:\n    image: nginx:alpine\n"},
	})
	composeDeploymentVerifier = func(context.Context, string) (deployment.VerificationResult, error) {
		return deployment.VerificationResult{Verified: false}, nil
	}
	id := startAppStoreDeployContractTask(t, "verify-app", DeployRequest{})
	task := waitForContractTask(t, id)
	require.Equal(t, "error", task.Status)
	require.Contains(t, task.Error, "部署验证未通过")
	entries, err := os.ReadDir(root)
	require.NoError(t, err)
	require.Len(t, entries, 1, "unverified deployment must preserve its project")
}

func TestAppStoreCompletionRequiresVerification(t *testing.T) {
	original := composeDeploymentVerifier
	t.Cleanup(func() { composeDeploymentVerifier = original })
	for _, verified := range []bool{false, true} {
		composeDeploymentVerifier = func(_ context.Context, project string) (deployment.VerificationResult, error) {
			require.Equal(t, "demo", project)
			return deployment.VerificationResult{Verified: verified}, nil
		}
		_, err := verifyAppStoreDeployment(context.Background(), "demo")
		require.Equal(t, !verified, err != nil)
	}
	composeDeploymentVerifier = func(context.Context, string) (deployment.VerificationResult, error) {
		return deployment.VerificationResult{}, context.Canceled
	}
	_, err := verifyAppStoreDeployment(context.Background(), "demo")
	require.ErrorIs(t, err, context.Canceled)
}

func TestAppStoreFinishOnlyRetriesPersistence(t *testing.T) {
	original := appStoreFinishTask
	t.Cleanup(func() { appStoreFinishTask = original })
	calls := 0
	appStoreFinishTask = func(id, status string, result any, detail string) error {
		calls++
		require.Equal(t, "success", status)
		if calls == 1 {
			return errors.New("database busy")
		}
		return nil
	}
	require.NoError(t, finishAppStoreTask("task", "success", nil, ""))
	require.Equal(t, 2, calls)
}

func TestAppStoreFinishReportsPersistentFailure(t *testing.T) {
	setupComposeDeployContractTest(t)
	require.NoError(t, database.UpsertTask("app-finish-failure", "appstore_deploy", "running"))
	_, err := database.GetDB().Exec(`CREATE TRIGGER fail_app_finish BEFORE UPDATE ON tasks BEGIN SELECT RAISE(FAIL, 'write unavailable'); END`)
	require.NoError(t, err)
	require.Error(t, finishAppStoreTask("app-finish-failure", "success", nil, ""))
	task, err := database.GetTask("app-finish-failure")
	require.NoError(t, err)
	require.Equal(t, "running", task.Status)
}
