package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dockerpanel/backend/pkg/database"
	"dockerpanel/backend/pkg/deployment"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestComposeLegacyGETDeployEndpointIsNotRegistered(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterComposeRoutes(router.Group("/api"))
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/compose/deploy/events?name=demo&compose=secret", nil)

	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusNotFound, recorder.Code)
}

func TestComposeDeployWaitsForProjectMutationLock(t *testing.T) {
	setupComposeDeployContractTest(t)
	unlock := lockComposeProjectMutation("locked-project")
	done := make(chan struct{})
	go func() {
		defer close(done)
		runComposeDeployTask(
			"compose-lock-contract",
			"locked-project",
			"services:\n  app:\n    image: busybox\n",
			"", "", false, composeOperationOptions{},
		)
	}()

	select {
	case <-done:
		unlock()
		t.Fatal("compose deployment mutated the project while another mutation held the lock")
	case <-time.After(50 * time.Millisecond):
	}
	unlock()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("compose deployment did not resume after the project lock was released")
	}
}

func TestComposeOperationWaitsForProjectMutationLock(t *testing.T) {
	projectRoot := setupComposeDeployContractTest(t)
	require.NoError(t, os.MkdirAll(filepath.Join(projectRoot, "operation-lock"), 0755))
	unlock := lockComposeProjectMutation("operation-lock")
	done := make(chan struct{})
	go func() {
		defer close(done)
		runComposeOperationTask("compose-operation-lock", "operation-lock", "unsupported", composeOperationOptions{})
	}()

	select {
	case <-done:
		unlock()
		t.Fatal("compose operation mutated the project while another mutation held the lock")
	case <-time.After(50 * time.Millisecond):
	}
	unlock()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("compose operation did not resume after the project lock was released")
	}
}

func TestComposeDeployReportsVerificationFailureAsTaskError(t *testing.T) {
	setupComposeDeployContractTest(t)
	originalRuntimeFactory := newLocalComposeServiceRuntime
	newLocalComposeServiceRuntime = func() deployment.ComposeRuntime {
		return composeServiceVerificationFailureRuntime{}
	}
	t.Cleanup(func() {
		newLocalComposeServiceRuntime = originalRuntimeFactory
	})

	runComposeDeployTask(
		"compose-verify-contract",
		"verify-project",
		"services:\n  app:\n    image: busybox\n",
		"", "", true, composeOperationOptions{},
	)

	task, err := database.GetTask("compose-verify-contract")
	require.NoError(t, err)
	require.Equal(t, "error", task.Status)
	require.Contains(t, task.Error, "containers_not_found")
}

func TestComposeAutoStartValidatesCandidateBeforeRunningCompose(t *testing.T) {
	setupComposeDeployContractTest(t)
	originalRunner, originalVerifier := composeDeploymentCommandRunner, composeDeploymentVerifier
	originalRuntimeFactory := newLocalComposeServiceRuntime
	t.Cleanup(func() {
		composeDeploymentCommandRunner, composeDeploymentVerifier = originalRunner, originalVerifier
		newLocalComposeServiceRuntime = originalRuntimeFactory
	})
	called := false
	composeDeploymentCommandRunner = func(context.Context, string, []string, func(string)) error {
		called = true
		return nil
	}
	composeDeploymentVerifier = func(context.Context, string) (deployment.VerificationResult, error) {
		return deployment.VerificationResult{Verified: true}, nil
	}
	newLocalComposeServiceRuntime = func() deployment.ComposeRuntime {
		return composeServiceValidationFailureRuntime{}
	}

	runComposeDeployTask(
		"compose-config-contract",
		"config-project",
		"services:\n  app:\n    image: busybox\n",
		"", "", true, composeOperationOptions{},
	)

	task, err := database.GetTask("compose-config-contract")
	require.NoError(t, err)
	require.Equal(t, "error", task.Status)
	require.Contains(t, task.Error, "invalid compose configuration")
	require.False(t, called, "compose up must not run when config validation fails")
}

type composeServiceValidationFailureRuntime struct{}

func (composeServiceValidationFailureRuntime) ValidateConfig(context.Context, string, string, []string) error {
	return errors.New("invalid compose configuration")
}

func (composeServiceValidationFailureRuntime) Run(context.Context, string, []string, []string, func(string)) error {
	return nil
}

func (composeServiceValidationFailureRuntime) Verify(context.Context, string) (deployment.VerificationResult, error) {
	return deployment.VerificationResult{Verified: true}, nil
}

type composeServiceVerificationFailureRuntime struct{}

func (composeServiceVerificationFailureRuntime) ValidateConfig(context.Context, string, string, []string) error {
	return nil
}

func (composeServiceVerificationFailureRuntime) Run(context.Context, string, []string, []string, func(string)) error {
	return nil
}

func (composeServiceVerificationFailureRuntime) Verify(context.Context, string) (deployment.VerificationResult, error) {
	return deployment.VerificationResult{Verified: false}, errors.New("containers_not_found")
}

func TestComposeDeployDelegatesVerificationToJobOnlyWhenRequested(t *testing.T) {
	setupComposeDeployContractTest(t)
	originalRunner, originalVerifier := composeDeploymentCommandRunner, composeDeploymentVerifier
	t.Cleanup(func() {
		composeDeploymentCommandRunner, composeDeploymentVerifier = originalRunner, originalVerifier
	})
	composeDeploymentCommandRunner = func(context.Context, string, []string, func(string)) error { return nil }
	composeDeploymentVerifier = func(context.Context, string) (deployment.VerificationResult, error) {
		t.Fatal("job-owned execution must leave verification to coordinator")
		return deployment.VerificationResult{}, nil
	}
	runComposeDeployTask("job-owned-verification", "job-verify-project",
		"services:\n  app:\n    image: busybox\n", "", "", true,
		composeOperationOptions{DeferVerification: true})
	record, err := database.GetTask("job-owned-verification")
	require.NoError(t, err)
	require.Equal(t, "success", record.Status)
	require.NotContains(t, record.ResultJSON, `"verified":true`)
}

func TestDefaultComposeDeploymentCommandRunnerUsesRootCandidate(t *testing.T) {
	projectDir := t.TempDir()
	rootCandidate := filepath.Join(projectDir, "docker-compose.yml")
	require.NoError(t, os.WriteFile(rootCandidate, []byte("services:\n  app:\n    image: nginx\n"), 0644))
	previous := composeDeploymentCommandRunnerWithSource
	t.Cleanup(func() { composeDeploymentCommandRunnerWithSource = previous })
	var captured string
	composeDeploymentCommandRunnerWithSource = func(_ context.Context, _ string, source string, _ []string, _ func(string)) error {
		captured = source
		return nil
	}

	require.NoError(t, defaultComposeDeploymentCommandRunner(context.Background(), projectDir, []string{"compose", "up", "-d"}, func(string) {}))
	require.Equal(t, rootCandidate, captured)
}

func TestComposeDeployContextCancellationStopsBeforeProjectWrite(t *testing.T) {
	projectRoot := setupComposeDeployContractTest(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	runComposeDeployTaskWithTypeContext(
		ctx,
		"compose-cancel-contract",
		"ai_agent_deploy_compose",
		"canceled-project",
		"services:\n  app:\n    image: nginx:alpine\n",
		"",
		"",
		true,
		composeOperationOptions{},
	)

	task, err := database.GetTask("compose-cancel-contract")
	require.NoError(t, err)
	require.Equal(t, "canceled", task.Status)
	_, err = os.Stat(filepath.Join(projectRoot, "canceled-project"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func setupComposeDeployContractTest(t *testing.T) string {
	t.Helper()

	_ = database.Close()
	root := t.TempDir()
	workDir := filepath.Join(root, "backend")
	require.NoError(t, os.MkdirAll(workDir, 0755))
	originalWorkingDir, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(workDir))
	t.Cleanup(func() {
		_ = os.Chdir(originalWorkingDir)
	})

	t.Setenv("ADMIN_PASSWORD", "compose-contract-password")
	require.NoError(t, database.InitDB(filepath.Join(root, "data", "data.db")))
	t.Cleanup(func() {
		_ = database.Close()
	})

	projectRoot := filepath.Join(workDir, "project")
	t.Setenv("PROJECT_ROOT", projectRoot)
	return projectRoot
}

func TestComposeDeployCommandPlanContract(t *testing.T) {
	tests := []struct {
		name    string
		options composeOperationOptions
		want    composeDeployCommandPlan
	}{
		{
			name: "reuse local state",
			want: composeDeployCommandPlan{
				Up: []string{"compose", "up", "-d"},
			},
		},
		{
			name:    "pull images",
			options: composeOperationOptions{Pull: true},
			want: composeDeployCommandPlan{
				Pull: []string{"compose", "pull"},
				Up:   []string{"compose", "up", "-d", "--pull", "always"},
			},
		},
		{
			name:    "rebuild without cache",
			options: composeOperationOptions{Rebuild: true},
			want: composeDeployCommandPlan{
				Build: []string{"compose", "build", "--no-cache"},
				Up:    []string{"compose", "up", "-d"},
			},
		},
		{
			name:    "pull and rebuild",
			options: composeOperationOptions{Pull: true, Rebuild: true},
			want: composeDeployCommandPlan{
				Pull:  []string{"compose", "pull"},
				Build: []string{"compose", "build", "--pull", "--no-cache"},
				Up:    []string{"compose", "up", "-d"},
			},
		},
		{
			name:    "recreate without build",
			options: composeOperationOptions{Pull: true, Rebuild: true, Recreate: true},
			want: composeDeployCommandPlan{
				Up: []string{"compose", "up", "-d", "--force-recreate", "--no-build"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, buildComposeDeployCommandPlan(tt.options))
		})
	}
}

func TestComposeDeployCommandPlanAppliesProfilesToEveryCommand(t *testing.T) {
	plan := buildComposeDeployCommandPlan(composeOperationOptions{
		Pull:     true,
		Rebuild:  true,
		Profiles: []string{"falkordb", "worker"},
	})
	require.Equal(t, []string{"compose", "--profile", "falkordb", "--profile", "worker", "pull"}, plan.Pull)
	require.Equal(t, []string{"compose", "--profile", "falkordb", "--profile", "worker", "build", "--pull", "--no-cache"}, plan.Build)
	require.Equal(t, []string{"compose", "--profile", "falkordb", "--profile", "worker", "up", "-d"}, plan.Up)
}

func TestComposeSaveDeployContractPreservesInputAndRejectsExistingProject(t *testing.T) {
	projectRoot := setupComposeDeployContractTest(t)
	composeText := `services:
  app:
    image: nginx:${TAG:-latest}
    ports:
      - "8080:80"
    volumes:
      - app-data:/var/lib/app
      - ./config:/etc/app:ro
volumes:
  app-data:
`

	runComposeDeployTask(
		"compose-save-contract",
		"compose-contract",
		composeText,
		"TAG=old\nIGNORED=remove-me\n",
		`{"TAG":"stable","IGNORED":"remove-me"}`,
		false,
		composeOperationOptions{},
	)

	task, err := database.GetTask("compose-save-contract")
	require.NoError(t, err)
	require.Equal(t, "compose_deploy", task.Type)
	require.Equal(t, "success", task.Status, task.Error)
	require.JSONEq(t, `{"autoStart":false,"project":"compose-contract"}`, task.ResultJSON)

	projectDir := filepath.Join(projectRoot, "compose-contract")
	composeBytes, err := os.ReadFile(filepath.Join(projectDir, "docker-compose.yml"))
	require.NoError(t, err)
	require.Equal(t, composeText, string(composeBytes))
	require.Contains(t, string(composeBytes), "app-data:/var/lib/app")
	require.Contains(t, string(composeBytes), "./config:/etc/app:ro")
	require.Contains(t, string(composeBytes), `"8080:80"`)

	dotenvBytes, err := os.ReadFile(filepath.Join(projectDir, ".env"))
	require.NoError(t, err)
	require.Contains(t, string(dotenvBytes), "TAG=stable")
	require.NotContains(t, string(dotenvBytes), "IGNORED")

	notifications, err := database.GetNotifications(20)
	require.NoError(t, err)
	require.Condition(t, func() bool {
		for _, notification := range notifications {
			if notification.Category == "deploy_task" && notification.Type == "success" &&
				strings.Contains(notification.Message, "compose-contract 部署成功") {
				return true
			}
		}
		return false
	})
	requireNoCommercialAgentRunSideEffects(t)

	originalCompose := append([]byte(nil), composeBytes...)
	runComposeDeployTask(
		"compose-existing-contract",
		"compose-contract",
		"services:\n  replacement:\n    image: busybox\n",
		"",
		"",
		false,
		composeOperationOptions{},
	)

	existingTask, err := database.GetTask("compose-existing-contract")
	require.NoError(t, err)
	require.Equal(t, "error", existingTask.Status)
	require.Equal(t, "project exists", existingTask.Error)
	afterReject, err := os.ReadFile(filepath.Join(projectDir, "docker-compose.yml"))
	require.NoError(t, err)
	require.Equal(t, originalCompose, afterReject)
}

func TestComposeDeployReplaceUpdatesComposeWithoutDeletingWorkspace(t *testing.T) {
	projectRoot := setupComposeDeployContractTest(t)
	projectDir := filepath.Join(projectRoot, "managed-update")
	require.NoError(t, os.MkdirAll(projectDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "docker-compose.yml"), []byte("services:\n  app:\n    image: nginx:1.26\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "user-data.txt"), []byte("keep"), 0o644))

	runComposeDeployTaskWithType(
		"compose-replace-contract",
		"compose_deploy",
		"managed-update",
		"services:\n  app:\n    image: nginx:1.27\n",
		"",
		"",
		false,
		composeOperationOptions{Replace: true},
	)

	task, err := database.GetTask("compose-replace-contract")
	require.NoError(t, err)
	require.Equal(t, "success", task.Status, task.Error)
	composeBytes, err := os.ReadFile(filepath.Join(projectDir, "docker-compose.yml"))
	require.NoError(t, err)
	require.Contains(t, string(composeBytes), "nginx:1.27")
	data, err := os.ReadFile(filepath.Join(projectDir, "user-data.txt"))
	require.NoError(t, err)
	require.Equal(t, "keep", string(data), "declarative replace must preserve workspace files")
}

func TestComposeDeployAdoptsEmptyExistingDirectoryButRejectsOpaqueFailures(t *testing.T) {
	projectRoot := setupComposeDeployContractTest(t)

	// 空目录：采纳并写入本次 YAML（save-only 部署）。
	emptyDir := filepath.Join(projectRoot, "adopt-empty")
	require.NoError(t, os.MkdirAll(emptyDir, 0o755))
	runComposeDeployTask(
		"compose-adopt-empty",
		"adopt-empty",
		"services:\n  app:\n    image: busybox\n",
		"", "", false, composeOperationOptions{},
	)
	adopted, err := database.GetTask("compose-adopt-empty")
	require.NoError(t, err)
	require.Equal(t, "success", adopted.Status, adopted.Error)
	composeBytes, err := os.ReadFile(filepath.Join(emptyDir, "docker-compose.yml"))
	require.NoError(t, err)
	require.Contains(t, string(composeBytes), "busybox")

	// findComposeFile 返回非 not-found 错误（目录被同名文件占据导致读取失败）
	// 时不得采纳，必须按检查失败终止。
	opaquePath := filepath.Join(projectRoot, "adopt-opaque")
	require.NoError(t, os.WriteFile(opaquePath, []byte("not a dir"), 0o644))
	runComposeDeployTask(
		"compose-adopt-opaque",
		"adopt-opaque",
		"services:\n  app:\n    image: busybox\n",
		"", "", false, composeOperationOptions{},
	)
	opaque, err := database.GetTask("compose-adopt-opaque")
	require.NoError(t, err)
	require.Equal(t, "error", opaque.Status)
	require.NotEqual(t, "project exists", opaque.Error, "opaque failures must not be treated as an empty directory")
}

func TestComposeGitImportContractRejectsExistingProjectWithoutDownloading(t *testing.T) {
	projectRoot := setupComposeDeployContractTest(t)
	projectDir := filepath.Join(projectRoot, "git-contract")
	require.NoError(t, os.MkdirAll(projectDir, 0755))
	sentinel := []byte("services:\n  local:\n    image: local-only\n")
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "compose.yml"), sentinel, 0644))

	runComposeGitSyncTask(
		"git-import-existing-contract",
		"compose_git_import",
		"git-contract",
		"https://github.com/example/project",
		"",
		"",
		false,
	)

	task, err := database.GetTask("git-import-existing-contract")
	require.NoError(t, err)
	require.Equal(t, "compose_git_import", task.Type)
	require.Equal(t, "error", task.Status)
	require.Equal(t, "项目 'git-contract' 已存在", task.Error)

	actual, err := os.ReadFile(filepath.Join(projectDir, "compose.yml"))
	require.NoError(t, err)
	require.Equal(t, sentinel, actual)

	notifications, err := database.GetNotifications(20)
	require.NoError(t, err)
	require.Condition(t, func() bool {
		for _, notification := range notifications {
			if notification.Category == "git_task" && notification.Type == "error" &&
				strings.Contains(notification.Message, "Git 导入失败") {
				return true
			}
		}
		return false
	})
}
