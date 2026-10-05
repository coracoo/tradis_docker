package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestDockerDisplayCacheExpiryAndSnapshotIsolation(t *testing.T) {
	now := time.Now()
	cache := newDockerDisplayCache(func() time.Time { return now })
	calls := 0
	fetch := func(context.Context) (dockerDisplayRead, error) {
		calls++
		return dockerDisplayRead{Usage: types.DiskUsage{Images: []*types.ImageSummary{{ID: "original"}}}}, nil
	}
	first, err := cache.read(context.Background(), "all", false, fetch)
	require.NoError(t, err)
	first.Usage.Images[0].ID = "modified"
	next, err := cache.read(context.Background(), "all", false, fetch)
	require.NoError(t, err)
	require.Equal(t, "hit", next.Cache)
	require.Equal(t, "original", next.Usage.Images[0].ID)
	require.Equal(t, 1, calls)
	now = now.Add(5 * time.Second)
	_, err = cache.read(context.Background(), "all", false, fetch)
	require.NoError(t, err)
	require.Equal(t, 2, calls)
	_, err = cache.read(context.Background(), "build", false, fetch)
	require.NoError(t, err)
	require.Equal(t, 3, calls)
}

func TestDockerDisplayCacheMergesReadsAndHonorsSubscriberCancellation(t *testing.T) {
	cache := newDockerDisplayCache(time.Now)
	started, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	fetch := func(ctx context.Context) (dockerDisplayRead, error) {
		calls.Add(1)
		_, bounded := ctx.Deadline()
		if !bounded {
			return dockerDisplayRead{}, errors.New("missing deadline")
		}
		close(started)
		<-release
		return dockerDisplayRead{Usage: types.DiskUsage{LayersSize: 42}}, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	first := make(chan error, 1)
	go func() { _, err := cache.read(ctx, "all", false, fetch); first <- err }()
	<-started
	cancel()
	require.ErrorIs(t, <-first, context.Canceled)
	second := make(chan dockerDisplayRead, 1)
	go func() { value, _ := cache.read(context.Background(), "all", false, fetch); second <- value }()
	close(release)
	value := <-second
	require.Equal(t, int64(42), value.Usage.LayersSize)
	require.Equal(t, int32(1), calls.Load())
}

func TestDockerDisplayCacheInvalidationFencesOldScans(t *testing.T) {
	cache := newDockerDisplayCache(time.Now)
	started, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	fetch := func(context.Context) (dockerDisplayRead, error) {
		call := calls.Add(1)
		if call == 1 {
			close(started)
			<-release
		}
		return dockerDisplayRead{Usage: types.DiskUsage{LayersSize: int64(call)}}, nil
	}
	first := make(chan dockerDisplayRead, 1)
	go func() { value, _ := cache.read(context.Background(), "all", false, fetch); first <- value }()
	<-started
	cache.invalidate()
	close(release)
	value := <-first
	require.Equal(t, int64(2), value.Usage.LayersSize)
	value, err := cache.read(context.Background(), "all", false, fetch)
	require.NoError(t, err)
	require.Equal(t, int64(2), value.Usage.LayersSize)
	require.Equal(t, int32(2), calls.Load())
	value, err = cache.read(context.Background(), "all", true, fetch)
	require.NoError(t, err)
	require.Equal(t, int64(3), value.Usage.LayersSize)
}

func TestDockerDisplayCacheDoesNotCacheFailures(t *testing.T) {
	cache := newDockerDisplayCache(time.Now)
	calls := 0
	fetch := func(context.Context) (dockerDisplayRead, error) {
		calls++
		if calls == 1 {
			return dockerDisplayRead{}, errors.New("scan failed")
		}
		return dockerDisplayRead{}, nil
	}
	_, err := cache.read(context.Background(), "all", false, fetch)
	require.Error(t, err)
	_, err = cache.read(context.Background(), "all", false, fetch)
	require.NoError(t, err)
	require.Equal(t, 2, calls)
}

func TestDockerDisplayReadsRejectRemoteBeforeLocalAcquisition(t *testing.T) {
	for _, path := range []string{"/api/system/disk-usage", "/api/images/build-cache"} {
		router := gin.New()
		group := router.Group("/api")
		RegisterSystemRoutes(group)
		RegisterImageRoutes(group)
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("X-TRADIS-Environment", "remote-test")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		require.Equal(t, http.StatusNotImplemented, response.Code)
		require.Contains(t, response.Body.String(), "DOCKER_READ_REMOTE_UNSUPPORTED")
	}
}

func TestDockerReadTimingsUseOnlyFixedStageNames(t *testing.T) {
	response := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(response)
	recordDockerReadTiming(c, "pool", time.Now().Add(-time.Millisecond))
	recordDockerReadTiming(c, "list", time.Now().Add(-2*time.Millisecond))
	require.Contains(t, response.Header().Get("Server-Timing"), "pool;dur=")
	require.Contains(t, response.Header().Get("Server-Timing"), ", list;dur=")
}

func TestDockerDisplayHTTPReadsServeCachedSnapshots(t *testing.T) {
	original := dockerDiskDisplayCache
	dockerDiskDisplayCache = newDockerDisplayCache(time.Now)
	t.Cleanup(func() { dockerDiskDisplayCache = original })
	for _, read := range []struct{ path, kind string }{
		{"/api/system/disk-usage", "all"}, {"/api/images/build-cache", "build"},
	} {
		key := fmt.Sprintf("%q:%q:%s", os.Getenv("DOCKER_HOST"), os.Getenv("DOCKER_SOCK"), read.kind)
		_, err := dockerDiskDisplayCache.read(context.Background(), key, false, func(context.Context) (dockerDisplayRead, error) {
			return dockerDisplayRead{Usage: types.DiskUsage{LayersSize: 42}}, nil
		})
		require.NoError(t, err)
		router := gin.New()
		group := router.Group("/api")
		RegisterSystemRoutes(group)
		RegisterImageRoutes(group)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, read.path, nil))
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		require.Equal(t, "hit", response.Header().Get("X-Tradis-Read-Cache"))
		require.NotEmpty(t, response.Header().Get("X-Tradis-Cache-Age-Ms"))
		require.Contains(t, response.Header().Get("Server-Timing"), "read;dur=")
		require.NotContains(t, response.Header().Get("Server-Timing"), "scan;dur=")
	}
}

func TestDockerDisplayMutationInvalidatesBeforeAndAfterFailedWrite(t *testing.T) {
	original := dockerDiskDisplayCache
	dockerDiskDisplayCache = newDockerDisplayCache(time.Now)
	t.Cleanup(func() { dockerDiskDisplayCache = original })
	var calls atomic.Int32
	fetch := func(context.Context) (dockerDisplayRead, error) {
		return dockerDisplayRead{Usage: types.DiskUsage{LayersSize: int64(calls.Add(1))}}, nil
	}
	read := func() int64 {
		value, err := dockerDiskDisplayCache.read(context.Background(), "all", false, fetch)
		require.NoError(t, err)
		return value.Usage.LayersSize
	}
	require.Equal(t, int64(1), read())
	router := gin.New()
	router.Use(dockerDisplayInvalidationMiddleware())
	router.GET("/api/images", func(c *gin.Context) { require.Equal(t, int64(1), read()); c.Status(http.StatusOK) })
	router.DELETE("/api/images/test", func(c *gin.Context) { require.Equal(t, int64(2), read()); c.Status(http.StatusInternalServerError) })
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/images", nil))
	require.Equal(t, int64(1), read())
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodDelete, "/api/images/test", nil))
	require.Equal(t, int64(3), read())
}
