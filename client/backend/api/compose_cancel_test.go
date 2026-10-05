package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"dockerpanel/backend/pkg/database"
	"dockerpanel/backend/pkg/deployment"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestCancelComposeTaskCancelsRunningDeploy(t *testing.T) {
	setupComposeDeployContractTest(t)
	originalRuntimeFactory := newLocalComposeServiceRuntime
	newLocalComposeServiceRuntime = func() deployment.ComposeRuntime { return blockingComposeRuntime{} }
	t.Cleanup(func() { newLocalComposeServiceRuntime = originalRuntimeFactory })
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/compose/tasks/:id/cancel", cancelComposeTask)

	taskID := "deploy-about-to-cancel"
	ctx, cancel := context.WithCancel(context.Background())
	registerComposeTaskCancel(taskID, cancel)
	defer unregisterComposeTaskCancel(taskID)

	go runComposeDeployTaskWithTypeContext(ctx, taskID, "compose_deploy", "canceled-project",
		"services:\n  app:\n    image: nginx:alpine\n", "", "", true, composeOperationOptions{})

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		task, err := database.GetTask(taskID)
		if err == nil && task.Status == "running" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	request := httptest.NewRequest(http.MethodPost, "/compose/tasks/"+taskID+"/cancel", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())

	require.Eventually(t, func() bool {
		task, err := database.GetTask(taskID)
		return err == nil && task.Status == "canceled"
	}, 3*time.Second, 20*time.Millisecond)
}

type blockingComposeRuntime struct{}

func (blockingComposeRuntime) ValidateConfig(context.Context, string, string, []string) error {
	return nil
}

func (blockingComposeRuntime) Run(ctx context.Context, _ string, _ []string, _ []string, _ func(string)) error {
	<-ctx.Done()
	return ctx.Err()
}

func (blockingComposeRuntime) Verify(context.Context, string) (deployment.VerificationResult, error) {
	return deployment.VerificationResult{Verified: true}, nil
}

func TestCancelComposeTaskTriggersRegisteredCancelFunc(t *testing.T) {
	setupComposeDeployContractTest(t)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/compose/tasks/:id/cancel", cancelComposeTask)

	taskID := "registered-cancelable-task"
	require.NoError(t, database.UpsertTask(taskID, "compose_deploy", "running"))

	var mu sync.Mutex
	cancelled := false
	registerComposeTaskCancel(taskID, func() {
		mu.Lock()
		cancelled = true
		mu.Unlock()
	})
	defer unregisterComposeTaskCancel(taskID)

	request := httptest.NewRequest(http.MethodPost, "/compose/tasks/"+taskID+"/cancel", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())

	mu.Lock()
	defer mu.Unlock()
	require.True(t, cancelled)
}

func TestCancelComposeTaskUnknownOrUnregistered(t *testing.T) {
	setupComposeDeployContractTest(t)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/compose/tasks/:id/cancel", cancelComposeTask)

	request := httptest.NewRequest(http.MethodPost, "/compose/tasks/nonexistent/cancel", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	require.Equal(t, http.StatusNotFound, response.Code, response.Body.String())

	taskID := "existing-but-not-registered"
	require.NoError(t, database.UpsertTask(taskID, "compose_deploy", "running"))

	request = httptest.NewRequest(http.MethodPost, "/compose/tasks/"+taskID+"/cancel", nil)
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	require.Equal(t, http.StatusConflict, response.Code, response.Body.String())
}
