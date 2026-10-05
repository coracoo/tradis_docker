package api

import (
	"bytes"
	"context"
	"dockerpanel/backend/pkg/cleanup"
	"dockerpanel/backend/pkg/database"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/volume"
	"github.com/gin-gonic/gin"
)

type cleanupDockerFake struct {
	mu sync.Mutex

	diskUsage types.DiskUsage
	networks  []types.NetworkResource
	volumes   volume.ListResponse

	diskUsageFunc func(context.Context, types.DiskUsageOptions) (types.DiskUsage, error)
	volumeListErr error

	containerRemoveOptions []types.ContainerRemoveOptions
	imageRemoveIDs         []string
	imageRemoveOptions     []types.ImageRemoveOptions
	containerRemoveErrors  map[string]error
	volumeRemoveNames      []string
	networkRemoveIDs       []string
	buildCachePruneOptions []types.BuildCachePruneOptions
	buildCacheReport       *types.BuildCachePruneReport

	diskUsageCalls  int
	diskUsageOpts   []types.DiskUsageOptions
	volumeListCalls int
	volumeListOpts  []volume.ListOptions
	removeCalls     int
	pruneCalls      int
	closeCalls      int
}

func (fake *cleanupDockerFake) DiskUsage(ctx context.Context, options types.DiskUsageOptions) (types.DiskUsage, error) {
	fake.mu.Lock()
	fake.diskUsageCalls++
	fake.diskUsageOpts = append(fake.diskUsageOpts, options)
	callback := fake.diskUsageFunc
	usage := fake.diskUsage
	fake.mu.Unlock()
	if callback != nil {
		return callback(ctx, options)
	}
	return usage, nil
}

func (fake *cleanupDockerFake) VolumeList(_ context.Context, options volume.ListOptions) (volume.ListResponse, error) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.volumeListCalls++
	fake.volumeListOpts = append(fake.volumeListOpts, options)
	return fake.volumes, fake.volumeListErr
}

func (fake *cleanupDockerFake) NetworkList(context.Context, types.NetworkListOptions) ([]types.NetworkResource, error) {
	return fake.networks, nil
}

func (fake *cleanupDockerFake) ContainerRemove(_ context.Context, id string, options types.ContainerRemoveOptions) error {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.removeCalls++
	fake.containerRemoveOptions = append(fake.containerRemoveOptions, options)
	if err := fake.containerRemoveErrors[id]; err != nil {
		return err
	}
	return nil
}

func (fake *cleanupDockerFake) ImageRemove(_ context.Context, id string, options types.ImageRemoveOptions) ([]types.ImageDeleteResponseItem, error) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.removeCalls++
	fake.imageRemoveIDs = append(fake.imageRemoveIDs, id)
	fake.imageRemoveOptions = append(fake.imageRemoveOptions, options)
	return []types.ImageDeleteResponseItem{{Deleted: id}}, nil
}

func (fake *cleanupDockerFake) VolumeRemove(_ context.Context, name string, _ bool) error {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.removeCalls++
	fake.volumeRemoveNames = append(fake.volumeRemoveNames, name)
	return nil
}

func (fake *cleanupDockerFake) NetworkRemove(_ context.Context, id string) error {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.removeCalls++
	fake.networkRemoveIDs = append(fake.networkRemoveIDs, id)
	return nil
}

func (fake *cleanupDockerFake) BuildCachePrune(_ context.Context, options types.BuildCachePruneOptions) (*types.BuildCachePruneReport, error) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.pruneCalls++
	fake.buildCachePruneOptions = append(fake.buildCachePruneOptions, options)
	if fake.buildCacheReport != nil {
		return fake.buildCacheReport, nil
	}
	return &types.BuildCachePruneReport{CachesDeleted: []string{"cache"}, SpaceReclaimed: 1024}, nil
}

func (fake *cleanupDockerFake) Close() error {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.closeCalls++
	return nil
}

func setupCleanupAPITest(t *testing.T, fake *cleanupDockerFake) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	_ = database.Close()
	if err := database.InitDB(filepath.Join(t.TempDir(), "cleanup.db")); err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	previousFactory := cleanupDockerClientFactory
	previousIdentity := cleanupProtectedIdentity
	cleanupDockerClientFactory = func() (cleanupDockerClient, error) { return fake, nil }
	cleanupProtectedIdentity = func() (string, string) { return "", "" }
	resetCleanupEvaluationStateForTest()
	t.Cleanup(func() {
		cleanupDockerClientFactory = previousFactory
		cleanupProtectedIdentity = previousIdentity
		resetCleanupEvaluationStateForTest()
	})

	router := gin.New()
	RegisterCleanupRoutes(router.Group("/api"))
	return router
}

func resetCleanupEvaluationStateForTest() {
	invalidateCleanupEvaluationCache()
	cleanupEvaluationFlights.Forget("local")
}

func TestCollectCleanupEvaluationFallsBackWhenVolumeSizingFails(t *testing.T) {
	fake := &cleanupDockerFake{
		volumes: volume.ListResponse{Volumes: []*volume.Volume{{
			Name: "archive", Driver: "local", Scope: "local",
		}}},
		diskUsageFunc: func(_ context.Context, options types.DiskUsageOptions) (types.DiskUsage, error) {
			if len(options.Types) == 1 && options.Types[0] == types.VolumeObject {
				return types.DiskUsage{}, context.DeadlineExceeded
			}
			return types.DiskUsage{Images: []*types.ImageSummary{{
				ID: "sha256:dangling", RepoTags: []string{"<none>:<none>"}, Size: 512,
			}}}, nil
		},
	}

	evaluation, err := collectCleanupEvaluation(context.Background(), fake)
	if err != nil {
		t.Fatalf("collectCleanupEvaluation: %v", err)
	}
	if fake.diskUsageCalls != 2 {
		t.Fatalf("DiskUsage calls = %d, want core query plus volume sizing query", fake.diskUsageCalls)
	}
	if fake.volumeListCalls != 1 {
		t.Fatalf("VolumeList calls = %d, want metadata fallback", fake.volumeListCalls)
	}
	if values := fake.volumeListOpts[0].Filters.Get("dangling"); len(values) != 1 || values[0] != "true" {
		t.Fatalf("VolumeList filters = %#v, want dangling=true", fake.volumeListOpts[0].Filters)
	}
	volumeCategory := cleanupCategoryByKey(t, evaluation, cleanup.CategoryUnusedVolume)
	if len(volumeCategory.Items) != 1 || volumeCategory.Items[0].ID != "archive" {
		t.Fatalf("volume category = %#v", volumeCategory)
	}
	if volumeCategory.Items[0].SizeState != cleanup.SizeUnknown {
		t.Fatalf("volume size state = %q, want unknown", volumeCategory.Items[0].SizeState)
	}
}

func TestCleanupEvaluationSharesConcurrentDockerScan(t *testing.T) {
	started := make(chan struct{}, 2)
	release := make(chan struct{})
	fake := &cleanupDockerFake{
		diskUsageFunc: func(_ context.Context, options types.DiskUsageOptions) (types.DiskUsage, error) {
			if containsDiskUsageType(options, types.ContainerObject) {
				started <- struct{}{}
				<-release
			}
			return types.DiskUsage{}, nil
		},
		volumes: volume.ListResponse{Volumes: []*volume.Volume{}},
	}
	router := setupCleanupAPITest(t, fake)

	responses := make(chan *httptest.ResponseRecorder, 2)
	go func() { responses <- performCleanupRequest(t, router, http.MethodGet, "/api/cleanup/evaluation", nil) }()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("first Docker scan did not start")
	}
	go func() { responses <- performCleanupRequest(t, router, http.MethodGet, "/api/cleanup/evaluation", nil) }()
	time.Sleep(30 * time.Millisecond)
	close(release)

	for range 2 {
		if response := <-responses; response.Code != http.StatusOK {
			t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
		}
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	coreCalls := 0
	for _, options := range fake.diskUsageOpts {
		if containsDiskUsageType(options, types.ContainerObject) {
			coreCalls++
		}
	}
	if coreCalls != 1 {
		t.Fatalf("core DiskUsage calls = %d, want one shared scan", coreCalls)
	}
}

func TestCleanupEvaluationTimeoutUsesGatewayTimeout(t *testing.T) {
	if got := cleanupEvaluationHTTPStatus(context.DeadlineExceeded); got != http.StatusGatewayTimeout {
		t.Fatalf("status = %d, want %d", got, http.StatusGatewayTimeout)
	}
}

func TestCleanupEvaluationTimeoutReturnsActionableMessage(t *testing.T) {
	fake := &cleanupDockerFake{diskUsageFunc: func(context.Context, types.DiskUsageOptions) (types.DiskUsage, error) {
		return types.DiskUsage{}, context.DeadlineExceeded
	}}
	router := setupCleanupAPITest(t, fake)

	response := performCleanupRequest(t, router, http.MethodGet, "/api/cleanup/evaluation", nil)

	if response.Code != http.StatusGatewayTimeout {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
	if !bytes.Contains(response.Body.Bytes(), []byte("Docker 空间统计超时，请稍后重试")) {
		t.Fatalf("body = %s", response.Body.String())
	}
}

func containsDiskUsageType(options types.DiskUsageOptions, expected types.DiskUsageObject) bool {
	for _, kind := range options.Types {
		if kind == expected {
			return true
		}
	}
	return false
}

func cleanupCategoryByKey(t *testing.T, evaluation cleanup.Evaluation, key cleanup.Category) cleanup.CategoryEvaluation {
	t.Helper()
	for _, category := range evaluation.Categories {
		if category.Key == key {
			return category
		}
	}
	t.Fatalf("category %q missing from %#v", key, evaluation.Categories)
	return cleanup.CategoryEvaluation{}
}

func performCleanupRequest(t *testing.T, router http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(payload)
	}
	request := httptest.NewRequest(method, path, reader)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func TestCleanupEvaluationIsReadOnly(t *testing.T) {
	fake := &cleanupDockerFake{
		diskUsage: types.DiskUsage{
			Images: []*types.ImageSummary{{ID: "dangling", RepoTags: []string{"<none>:<none>"}, Size: 512}},
		},
	}
	router := setupCleanupAPITest(t, fake)

	response := performCleanupRequest(t, router, http.MethodGet, "/api/cleanup/evaluation", nil)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
	if fake.removeCalls != 0 || fake.pruneCalls != 0 {
		t.Fatalf("read-only evaluation removed=%d pruned=%d", fake.removeCalls, fake.pruneCalls)
	}
	if fake.diskUsageCalls != 2 {
		t.Fatalf("DiskUsage calls = %d, want core query plus volume sizing query", fake.diskUsageCalls)
	}
	if !containsDiskUsageType(fake.diskUsageOpts[0], types.ContainerObject) ||
		containsDiskUsageType(fake.diskUsageOpts[0], types.VolumeObject) {
		t.Fatalf("core DiskUsage options = %#v", fake.diskUsageOpts[0])
	}
	if len(fake.diskUsageOpts[1].Types) != 1 || fake.diskUsageOpts[1].Types[0] != types.VolumeObject {
		t.Fatalf("volume DiskUsage options = %#v", fake.diskUsageOpts[1])
	}
}

func TestStartCleanupRejectsVolumeWithoutHighRiskConfirmation(t *testing.T) {
	router := setupCleanupAPITest(t, &cleanupDockerFake{})
	response := performCleanupRequest(t, router, http.MethodPost, "/api/cleanup/tasks", map[string]any{
		"items":           []map[string]string{{"category": "unused_volume", "id": "media"}},
		"confirmHighRisk": false,
	})

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
}

func TestStartCleanupRejectsConcurrentTask(t *testing.T) {
	router := setupCleanupAPITest(t, &cleanupDockerFake{})
	if err := database.UpsertTask("cleanup-existing", cleanupTaskType, "running"); err != nil {
		t.Fatal(err)
	}
	response := performCleanupRequest(t, router, http.MethodPost, "/api/cleanup/tasks", map[string]any{
		"items": []map[string]string{{"category": "dangling_image", "id": "image"}},
	})
	if response.Code != http.StatusConflict {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
}

func TestExecuteCleanupItemsKeepsContainerVolumesAndAggregatesBuildCache(t *testing.T) {
	fake := &cleanupDockerFake{}
	items := []cleanup.Item{
		{Key: "stopped_container:container", Category: cleanup.CategoryStoppedContainer, ID: "container", Name: "container", SizeState: cleanup.SizeKnown},
		{Key: "build_cache:all-unused", Category: cleanup.CategoryBuildCache, ID: cleanup.BuildCacheAggregateID, Name: "cache", SizeState: cleanup.SizeKnown},
	}

	result := executeCleanupItems(context.Background(), fake, items, nil)

	if len(result.Failed) != 0 || len(result.Deleted) != 2 {
		t.Fatalf("result = %#v", result)
	}
	if len(fake.containerRemoveOptions) != 1 || fake.containerRemoveOptions[0].RemoveVolumes {
		t.Fatalf("container remove options = %#v", fake.containerRemoveOptions)
	}
	if len(fake.buildCachePruneOptions) != 1 || fake.buildCachePruneOptions[0].All {
		t.Fatalf("build cache prune options = %#v", fake.buildCachePruneOptions)
	}
}

func TestExecuteCleanupImageDoesNotPruneUnselectedParents(t *testing.T) {
	fake := &cleanupDockerFake{}
	items := []cleanup.Item{{
		Key: "unused_image:image", Category: cleanup.CategoryUnusedImage,
		ID: "image", Name: "image:latest", SizeState: cleanup.SizeKnown,
	}}

	result := executeCleanupItems(context.Background(), fake, items, nil)

	if len(result.Deleted) != 1 || len(fake.imageRemoveOptions) != 1 {
		t.Fatalf("result=%#v options=%#v", result, fake.imageRemoveOptions)
	}
	if fake.imageRemoveOptions[0].PruneChildren {
		t.Fatal("selected image cleanup must not recursively prune parent images")
	}
}

func TestExecuteCleanupItemsSkipsBuildCacheAlreadyRemoved(t *testing.T) {
	fake := &cleanupDockerFake{buildCacheReport: &types.BuildCachePruneReport{}}
	items := []cleanup.Item{{
		Key: "build_cache:all-unused", Category: cleanup.CategoryBuildCache,
		ID: cleanup.BuildCacheAggregateID, Name: "cache", SizeState: cleanup.SizeKnown,
	}}

	result := executeCleanupItems(context.Background(), fake, items, nil)

	if len(result.Deleted) != 0 || len(result.Skipped) != 1 || len(result.Failed) != 0 {
		t.Fatalf("result = %#v", result)
	}
}

func TestReserveCleanupTaskAllowsOnlyOneConcurrentTask(t *testing.T) {
	setupCleanupAPITest(t, &cleanupDockerFake{})
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, id := range []string{"cleanup-a", "cleanup-b"} {
		go func(taskID string) {
			<-start
			results <- reserveCleanupTask(taskID)
		}(id)
	}
	close(start)

	var accepted, rejected int
	for range 2 {
		err := <-results
		switch {
		case err == nil:
			accepted++
		case errors.Is(err, errCleanupTaskActive):
			rejected++
		default:
			t.Fatalf("unexpected reservation error: %v", err)
		}
	}
	if accepted != 1 || rejected != 1 {
		t.Fatalf("accepted=%d rejected=%d", accepted, rejected)
	}
}

func TestValidateCleanupSelectionsSkipsStaleCandidate(t *testing.T) {
	evaluation := cleanup.Evaluation{Categories: []cleanup.CategoryEvaluation{{
		Key:   cleanup.CategoryUnusedVolume,
		Items: []cleanup.Item{{Key: "unused_volume:still-free", Category: cleanup.CategoryUnusedVolume, ID: "still-free"}},
	}}}
	valid, skipped := validateCleanupSelections([]cleanup.Selection{
		{Category: cleanup.CategoryUnusedVolume, ID: "still-free"},
		{Category: cleanup.CategoryUnusedVolume, ID: "now-used"},
	}, evaluation)

	if len(valid) != 1 || valid[0].ID != "still-free" {
		t.Fatalf("valid = %#v", valid)
	}
	if len(skipped) != 1 || skipped[0].ID != "now-used" {
		t.Fatalf("skipped = %#v", skipped)
	}
}

func TestRecoverCleanupTasksMarksInterruptedWithoutDockerCalls(t *testing.T) {
	fake := &cleanupDockerFake{}
	setupCleanupAPITest(t, fake)
	if err := database.UpsertTask("cleanup-pending", cleanupTaskType, "pending"); err != nil {
		t.Fatal(err)
	}
	if err := database.UpsertTask("cleanup-running", cleanupTaskType, "running"); err != nil {
		t.Fatal(err)
	}

	RecoverCleanupTasks()

	for _, id := range []string{"cleanup-pending", "cleanup-running"} {
		task, err := database.GetTask(id)
		if err != nil {
			t.Fatal(err)
		}
		if task.Status != "error" || task.Error == "" {
			t.Fatalf("task %s = %#v", id, task)
		}
	}
	if fake.removeCalls != 0 || fake.pruneCalls != 0 {
		t.Fatalf("recovery executed Docker operations")
	}
}

func TestCleanupTaskPersistsResultAndNotification(t *testing.T) {
	fake := &cleanupDockerFake{
		diskUsage: types.DiskUsage{Containers: []*types.Container{{
			ID: "stopped", Names: []string{"/demo"}, State: "exited", SizeRw: 128,
		}}},
	}
	setupCleanupAPITest(t, fake)

	request := cleanupTaskRequest{Items: []cleanup.Selection{{Category: cleanup.CategoryStoppedContainer, ID: "stopped"}}}
	runCleanupTask("cleanup-task", request)

	task, err := database.GetTask("cleanup-task")
	if err != nil {
		t.Fatal(err)
	}
	if task.Status != "success" || task.ResultJSON == "" {
		t.Fatalf("task = %#v", task)
	}
	notifications, err := database.GetNotifications(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(notifications) != 1 || notifications[0].Category != "system" {
		t.Fatalf("notifications = %#v", notifications)
	}
}

func TestCleanupTaskMarksPartialFailureAsError(t *testing.T) {
	fake := &cleanupDockerFake{
		diskUsage: types.DiskUsage{Containers: []*types.Container{
			{ID: "success", Names: []string{"/success"}, State: "exited", SizeRw: 128},
			{ID: "failure", Names: []string{"/failure"}, State: "exited", SizeRw: 128},
		}},
		containerRemoveErrors: map[string]error{"failure": errors.New("remove failed")},
	}
	setupCleanupAPITest(t, fake)

	runCleanupTask("cleanup-partial", cleanupTaskRequest{Items: []cleanup.Selection{
		{Category: cleanup.CategoryStoppedContainer, ID: "success"},
		{Category: cleanup.CategoryStoppedContainer, ID: "failure"},
	}})

	task, err := database.GetTask("cleanup-partial")
	if err != nil {
		t.Fatal(err)
	}
	if task.Status != "error" || task.Error == "" {
		t.Fatalf("task = %#v", task)
	}
	var result cleanup.TaskResult
	if err := json.Unmarshal([]byte(task.ResultJSON), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Deleted) != 1 || len(result.Failed) != 1 {
		t.Fatalf("result = %#v", result)
	}
}

var _ cleanupDockerClient = (*cleanupDockerFake)(nil)
