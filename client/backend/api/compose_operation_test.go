package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dockerpanel/backend/pkg/database"
	"dockerpanel/backend/pkg/docker"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComposeOperationTaskRejectsMissingProjectBeforeEnqueue(t *testing.T) {
	setupComposeDeployContractTest(t)
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.POST("/compose/:name/stop/tasks", stopProjectTask)

	request := httptest.NewRequest(http.MethodPost, "/compose/we-mp-rss/stop/tasks", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	require.Equal(t, http.StatusNotFound, response.Code)
	require.Contains(t, strings.ToLower(response.Body.String()), "compose 项目目录不存在")
}

func TestComposeDestructiveTasksRejectMissingProjectBeforeEnqueue(t *testing.T) {
	setupComposeDeployContractTest(t)
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.POST("/compose/:name/down/tasks", downProjectTask)
	router.POST("/compose/:name/remove/tasks", removeProjectTask)

	for _, path := range []string{"/compose/missing/down/tasks", "/compose/missing/remove/tasks"} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, path, nil))
		require.Equal(t, http.StatusNotFound, response.Code, response.Body.String())
		require.Contains(t, response.Body.String(), "Compose 项目目录不存在")
	}
}

func TestUpdateComposeProjectMetadataStoresAndClearsRemark(t *testing.T) {
	setupComposeDeployContractTest(t)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.PUT("/compose/projects/:name/metadata", updateComposeProjectMetadata)

	request := httptest.NewRequest(http.MethodPut, "/compose/projects/media/metadata", strings.NewReader(`{"remark":"家庭媒体中心"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())

	remark, err := database.GetComposeProjectRemark(database.LocalEnvironmentID, "media")
	require.NoError(t, err)
	require.Equal(t, "家庭媒体中心", remark)

	request = httptest.NewRequest(http.MethodPut, "/compose/projects/media/metadata", strings.NewReader(`{"remark":""}`))
	request.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	remark, err = database.GetComposeProjectRemark(database.LocalEnvironmentID, "media")
	require.NoError(t, err)
	require.Empty(t, remark)
}

func TestUpdateComposeProjectMetadataValidatesInput(t *testing.T) {
	setupComposeDeployContractTest(t)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.PUT("/compose/projects/:name/metadata", updateComposeProjectMetadata)

	tests := []struct {
		name string
		path string
		body any
	}{
		{name: "invalid project", path: "/compose/projects/%5C/metadata", body: map[string]string{"remark": "x"}},
		{name: "remark too long", path: "/compose/projects/media/metadata", body: map[string]string{"remark": strings.Repeat("字", 129)}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			raw, err := json.Marshal(test.body)
			require.NoError(t, err)
			request := httptest.NewRequest(http.MethodPut, test.path, strings.NewReader(string(raw)))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			require.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
		})
	}
}

func TestUpdateContainerRemarkStoresAndIsolatesFromComposeProjects(t *testing.T) {
	setupComposeDeployContractTest(t)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.PUT("/compose/projects/:name/metadata", updateComposeProjectMetadata)
	router.PUT("/compose/containers/:name/remark", updateContainerRemark)
	router.GET("/compose/container-remarks", listContainerRemarks)

	put := func(path, body string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPut, path, strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		return response
	}

	response := put("/compose/projects/media/metadata", `{"remark":"项目备注"}`)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	response = put("/compose/containers/we-mp-rss/remark", `{"remark":"容器备注"}`)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())

	remark, err := database.GetComposeProjectRemark(database.LocalEnvironmentID, "container:we-mp-rss")
	require.NoError(t, err)
	require.Equal(t, "容器备注", remark)

	request := httptest.NewRequest(http.MethodGet, "/compose/container-remarks", nil)
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	var remarks map[string]string
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &remarks))
	require.Equal(t, map[string]string{"we-mp-rss": "容器备注"}, remarks)

	response = put("/compose/containers/we-mp-rss/remark", `{"remark":""}`)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	remark, err = database.GetComposeProjectRemark(database.LocalEnvironmentID, "container:we-mp-rss")
	require.NoError(t, err)
	require.Empty(t, remark)
}

func TestUpdateContainerRemarkValidatesInput(t *testing.T) {
	setupComposeDeployContractTest(t)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.PUT("/compose/containers/:name/remark", updateContainerRemark)

	tests := []struct {
		name string
		path string
		body any
	}{
		{name: "invalid container name", path: "/compose/containers/%E4%B8%AD%E6%96%87/remark", body: map[string]string{"remark": "x"}},
		{name: "remark too long", path: "/compose/containers/we-mp-rss/remark", body: map[string]string{"remark": strings.Repeat("字", 129)}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			raw, err := json.Marshal(test.body)
			require.NoError(t, err)
			request := httptest.NewRequest(http.MethodPut, test.path, strings.NewReader(string(raw)))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			require.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
		})
	}
}

// ===== Compose 项目强制停止（0 宽限期 ContainerStop） =====

func TestKillComposeProjectContainersKillsLiveContainers(t *testing.T) {
	var killed []string
	var timeouts []*int
	mockClient := &forceStopCaptureClient{
		MockDockerClient: &docker.MockDockerClient{
			ContainerListFunc: func(_ context.Context, options types.ContainerListOptions) ([]types.Container, error) {
				assert.True(t, options.All)
				labelFilters := options.Filters.Get("label")
				assert.Contains(t, labelFilters, "com.docker.compose.project=demo")
				return []types.Container{
					{ID: "running-1", Names: []string{"/demo-web-1"}, State: "running"},
					{ID: "running-2", Names: []string{"/demo-db-1"}, State: "running"},
					{ID: "restarting-1", Names: []string{"/demo-worker-1"}, State: "restarting"},
					{ID: "paused-1", Names: []string{"/demo-cache-1"}, State: "paused"},
					{ID: "exited-1", Names: []string{"/demo-old-1"}, State: "exited"},
				}, nil
			},
		},
		stop: func(_ context.Context, id string, options container.StopOptions) error {
			assert.NotNil(t, options.Timeout)
			assert.Zero(t, *options.Timeout)
			killed = append(killed, id)
			timeouts = append(timeouts, options.Timeout)
			return nil
		},
	}

	killedCount, err := killComposeProjectContainers(context.Background(), mockClient, "demo")
	assert.NoError(t, err)
	assert.Equal(t, 4, killedCount)
	assert.Equal(t, []string{"running-1", "running-2", "restarting-1", "paused-1"}, killed)
	assert.Len(t, timeouts, 4)
}

func TestKillComposeProjectContainersAggregatesFailures(t *testing.T) {
	mockClient := &forceStopCaptureClient{
		MockDockerClient: &docker.MockDockerClient{
			ContainerListFunc: func(context.Context, types.ContainerListOptions) ([]types.Container, error) {
				return []types.Container{
					{ID: "ok-1", Names: []string{"/demo-web-1"}, State: "running"},
					{ID: "bad-1", Names: []string{"/demo-db-1"}, State: "running"},
				}, nil
			},
		},
		stop: func(_ context.Context, id string, _ container.StopOptions) error {
			if id == "bad-1" {
				return errors.New("kill failed")
			}
			return nil
		},
	}

	killedCount, err := killComposeProjectContainers(context.Background(), mockClient, "demo")
	assert.Error(t, err)
	assert.Equal(t, 1, killedCount)
	assert.Contains(t, err.Error(), "demo-db-1")
}

type composeLifecycleCaptureClient struct {
	list    func(context.Context, types.ContainerListOptions) ([]types.Container, error)
	stop    func(context.Context, string, container.StopOptions) error
	restart func(context.Context, string, container.StopOptions) error
}

func (client *composeLifecycleCaptureClient) ContainerList(ctx context.Context, options types.ContainerListOptions) ([]types.Container, error) {
	return client.list(ctx, options)
}

func (client *composeLifecycleCaptureClient) ContainerStop(ctx context.Context, id string, options container.StopOptions) error {
	return client.stop(ctx, id, options)
}

func (client *composeLifecycleCaptureClient) ContainerRestart(ctx context.Context, id string, options container.StopOptions) error {
	return client.restart(ctx, id, options)
}

func TestComposeLifecycleUsesProjectLabelsAndVerifiesState(t *testing.T) {
	t.Run("stop ignores stale compose service names", func(t *testing.T) {
		listCalls := 0
		stopped := []string{}
		client := &composeLifecycleCaptureClient{
			list: func(_ context.Context, options types.ContainerListOptions) ([]types.Container, error) {
				assert.Contains(t, options.Filters.Get("label"), "com.docker.compose.project=wallabag")
				listCalls++
				if listCalls == 1 {
					return []types.Container{{ID: "runtime-container", Names: []string{"/wallabag"}, State: "running", Labels: map[string]string{"com.docker.compose.service": "wallabag"}}}, nil
				}
				return []types.Container{{ID: "runtime-container", Names: []string{"/wallabag"}, State: "exited"}}, nil
			},
			stop: func(_ context.Context, id string, options container.StopOptions) error {
				stopped = append(stopped, id)
				require.NotNil(t, options.Timeout)
				require.Equal(t, 2, *options.Timeout)
				return nil
			},
			restart: func(context.Context, string, container.StopOptions) error { return nil },
		}

		count, err := stopComposeProjectContainers(context.Background(), client, "wallabag", 2)
		require.NoError(t, err)
		require.Equal(t, 1, count)
		require.Equal(t, []string{"runtime-container"}, stopped)
	})

	t.Run("stop rejects a fake success", func(t *testing.T) {
		client := &composeLifecycleCaptureClient{
			list: func(context.Context, types.ContainerListOptions) ([]types.Container, error) {
				return []types.Container{{ID: "still-running", Names: []string{"/wallabag"}, State: "running"}}, nil
			},
			stop:    func(context.Context, string, container.StopOptions) error { return nil },
			restart: func(context.Context, string, container.StopOptions) error { return nil },
		}

		_, err := stopComposeProjectContainers(context.Background(), client, "wallabag", 2)
		require.ErrorContains(t, err, "仍在运行")
	})

	t.Run("restart rejects missing runtime containers", func(t *testing.T) {
		client := &composeLifecycleCaptureClient{
			list:    func(context.Context, types.ContainerListOptions) ([]types.Container, error) { return nil, nil },
			stop:    func(context.Context, string, container.StopOptions) error { return nil },
			restart: func(context.Context, string, container.StopOptions) error { return nil },
		}

		_, err := restartComposeProjectContainers(context.Background(), client, "wallabag", 2)
		require.ErrorContains(t, err, "未找到")
	})

	t.Run("restart addresses the observed runtime containers", func(t *testing.T) {
		restarted := []string{}
		listCalls := 0
		client := &composeLifecycleCaptureClient{
			list: func(_ context.Context, options types.ContainerListOptions) ([]types.Container, error) {
				assert.Contains(t, options.Filters.Get("label"), "com.docker.compose.project=wallabag")
				listCalls++
				return []types.Container{{ID: "runtime-container", Names: []string{"/wallabag"}, State: "running"}}, nil
			},
			stop: func(context.Context, string, container.StopOptions) error { return nil },
			restart: func(_ context.Context, id string, options container.StopOptions) error {
				restarted = append(restarted, id)
				require.NotNil(t, options.Timeout)
				require.Equal(t, 2, *options.Timeout)
				return nil
			},
		}

		count, err := restartComposeProjectContainers(context.Background(), client, "wallabag", 2)
		require.NoError(t, err)
		require.Equal(t, 1, count)
		require.Equal(t, []string{"runtime-container"}, restarted)
		require.Equal(t, 2, listCalls)
	})

	t.Run("restart rejects a fake success", func(t *testing.T) {
		listCalls := 0
		client := &composeLifecycleCaptureClient{
			list: func(context.Context, types.ContainerListOptions) ([]types.Container, error) {
				listCalls++
				if listCalls == 1 {
					return []types.Container{{ID: "runtime-container", Names: []string{"/wallabag"}, State: "running"}}, nil
				}
				return []types.Container{{ID: "runtime-container", Names: []string{"/wallabag"}, State: "exited"}}, nil
			},
			stop:    func(context.Context, string, container.StopOptions) error { return nil },
			restart: func(context.Context, string, container.StopOptions) error { return nil },
		}

		_, err := restartComposeProjectContainers(context.Background(), client, "wallabag", 2)
		require.ErrorContains(t, err, "重启后仅 0/1 个容器处于运行态")
	})
}

func TestKillProjectTaskRejectsInvalidProjectName(t *testing.T) {
	setupComposeDeployContractTest(t)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/compose/:name/kill/tasks", killProjectTask)

	request := httptest.NewRequest(http.MethodPost, "/compose/%5C/kill/tasks", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	require.Equal(t, http.StatusBadRequest, response.Code)
}

func TestKillProjectTaskRejectsMissingProject(t *testing.T) {
	setupComposeDeployContractTest(t)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/compose/:name/kill/tasks", killProjectTask)

	request := httptest.NewRequest(http.MethodPost, "/compose/missing-proj/kill/tasks", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	require.Equal(t, http.StatusNotFound, response.Code)
	require.Contains(t, response.Body.String(), "Compose 项目目录不存在")
}

func TestKillProjectTaskEnqueuesComposeKillTask(t *testing.T) {
	projectRoot := setupComposeDeployContractTest(t)
	gin.SetMode(gin.TestMode)
	require.NoError(t, os.MkdirAll(filepath.Join(projectRoot, "media"), 0755))

	router := gin.New()
	router.POST("/compose/:name/kill/tasks", killProjectTask)

	request := httptest.NewRequest(http.MethodPost, "/compose/media/kill/tasks", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	var payload map[string]any
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &payload))
	assert.Equal(t, "compose_kill", payload["type"])
	assert.Equal(t, "media", payload["project"])
	assert.NotEmpty(t, payload["taskId"])
	taskID, _ := payload["taskId"].(string)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		task, err := database.GetTask(taskID)
		if err == nil && (task.Status == "success" || task.Status == "error") {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("compose kill task %s did not finish before test cleanup", taskID)
}
