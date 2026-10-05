package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"dockerpanel/backend/pkg/database"
	"dockerpanel/backend/pkg/docker"
	"github.com/docker/docker/api/types"
	"github.com/gin-gonic/gin"
)

const dockerDisplayTTL = 5 * time.Second

type dockerDisplayRead struct {
	Usage        types.DiskUsage
	FetchedAt    time.Time
	PoolDuration time.Duration
	ScanDuration time.Duration
	Cache        string
}

type dockerDisplayEntry struct {
	payload                    []byte
	fetchedAt                  time.Time
	poolDuration, scanDuration time.Duration
}

type dockerDisplayFlight struct {
	done       chan struct{}
	generation uint64
	entry      dockerDisplayEntry
	err        error
}

type dockerDisplayCache struct {
	mu         sync.Mutex
	now        func() time.Time
	generation uint64
	entries    map[string]dockerDisplayEntry
	flights    map[string]*dockerDisplayFlight
}

func newDockerDisplayCache(now func() time.Time) *dockerDisplayCache {
	return &dockerDisplayCache{now: now, entries: make(map[string]dockerDisplayEntry), flights: make(map[string]*dockerDisplayFlight)}
}

var dockerDiskDisplayCache = newDockerDisplayCache(time.Now)

func (cache *dockerDisplayCache) invalidate() {
	cache.mu.Lock()
	cache.generation++
	cache.entries = make(map[string]dockerDisplayEntry)
	cache.mu.Unlock()
}

func (cache *dockerDisplayCache) read(ctx context.Context, key string, force bool, fetch func(context.Context) (dockerDisplayRead, error)) (dockerDisplayRead, error) {
	ctx, cancel := docker.WithCleanupTimeoutFrom(ctx)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return dockerDisplayRead{}, err
	}
	if force {
		cache.invalidate()
	}
	for {
		if err := ctx.Err(); err != nil {
			return dockerDisplayRead{}, err
		}
		cache.mu.Lock()
		if entry, ok := cache.entries[key]; ok && cache.now().Sub(entry.fetchedAt) < dockerDisplayTTL {
			cache.mu.Unlock()
			return decodeDockerDisplayEntry(entry, "hit")
		}
		flight := cache.flights[key]
		mode := "shared"
		if flight == nil {
			flight = &dockerDisplayFlight{done: make(chan struct{}), generation: cache.generation}
			cache.flights[key] = flight
			mode = "miss"
			// A subscriber disconnect does not cancel another subscriber's bounded read.
			go cache.scan(key, flight, fetch)
		}
		cache.mu.Unlock()
		select {
		case <-ctx.Done():
			return dockerDisplayRead{}, ctx.Err()
		case <-flight.done:
		}
		cache.mu.Lock()
		current := flight.generation == cache.generation
		cache.mu.Unlock()
		if !current {
			continue
		}
		if flight.err != nil {
			return dockerDisplayRead{}, flight.err
		}
		return decodeDockerDisplayEntry(flight.entry, mode)
	}
}

func (cache *dockerDisplayCache) scan(key string, flight *dockerDisplayFlight, fetch func(context.Context) (dockerDisplayRead, error)) {
	ctx, cancel := docker.WithCleanupTimeoutFrom(context.Background())
	defer cancel()
	result, err := fetch(ctx)
	var payload []byte
	if err == nil {
		payload, err = json.Marshal(result.Usage)
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	flight.err = err
	flight.entry = dockerDisplayEntry{payload: payload, fetchedAt: cache.now(), poolDuration: result.PoolDuration, scanDuration: result.ScanDuration}
	if err == nil && flight.generation == cache.generation {
		cache.entries[key] = flight.entry
	}
	delete(cache.flights, key)
	close(flight.done)
}

func decodeDockerDisplayEntry(entry dockerDisplayEntry, mode string) (dockerDisplayRead, error) {
	result := dockerDisplayRead{FetchedAt: entry.fetchedAt, PoolDuration: entry.poolDuration, ScanDuration: entry.scanDuration, Cache: mode}
	// Each HTTP consumer owns its decoded snapshot; request code cannot mutate the cache.
	err := json.Unmarshal(entry.payload, &result.Usage)
	return result, err
}

func readDockerDisplayUsage(c *gin.Context, kind string) (types.DiskUsage, bool) {
	environmentID, valid := requestEnvironmentScope(c)
	if !valid {
		return types.DiskUsage{}, false
	}
	if environmentID != database.LocalEnvironmentID {
		respondErrorWithCode(c, http.StatusNotImplemented, "DOCKER_READ_REMOTE_UNSUPPORTED", "远程设备暂不支持此 Docker 磁盘统计", nil)
		return types.DiskUsage{}, false
	}
	started := time.Now()
	key := fmt.Sprintf("%q:%q:%s", os.Getenv("DOCKER_HOST"), os.Getenv("DOCKER_SOCK"), kind)
	result, err := dockerDiskDisplayCache.read(c.Request.Context(), key, c.Query("force") == "1" || c.Query("refresh") == "1", func(ctx context.Context) (dockerDisplayRead, error) {
		acquireStarted := time.Now()
		cli, err := docker.NewDockerClientWithContext(ctx)
		read := dockerDisplayRead{PoolDuration: time.Since(acquireStarted)}
		if err != nil {
			return read, err
		}
		defer cli.Close()
		scanStarted := time.Now()
		if kind == "build" {
			read.Usage, err = dockerBuildCacheDiskUsage(ctx, cli)
		} else {
			read.Usage, err = cli.DiskUsage(ctx, types.DiskUsageOptions{})
		}
		read.ScanDuration = time.Since(scanStarted)
		return read, err
	})
	recordDockerReadTiming(c, "read", started)
	if err != nil {
		if errors.Is(err, docker.ErrPoolBusy) {
			respondErrorWithCode(c, http.StatusServiceUnavailable, "DOCKER_POOL_BUSY", "Docker 正忙，请稍后重试", err)
		} else {
			respondError(c, http.StatusInternalServerError, "读取 Docker 磁盘统计失败", err)
		}
		return types.DiskUsage{}, false
	}
	c.Header("X-Tradis-Read-Cache", result.Cache)
	c.Header("X-Tradis-Cache-Age-Ms", strconv.FormatInt(max(0, time.Since(result.FetchedAt).Milliseconds()), 10))
	if result.Cache != "hit" {
		appendDockerReadTiming(c, "pool", result.PoolDuration)
		appendDockerReadTiming(c, "scan", result.ScanDuration)
	}
	return result.Usage, true
}

func recordDockerReadTiming(c *gin.Context, stage string, started time.Time) {
	appendDockerReadTiming(c, stage, time.Since(started))
}

func appendDockerReadTiming(c *gin.Context, stage string, duration time.Duration) {
	value := fmt.Sprintf("%s;dur=%.2f", stage, float64(duration)/float64(time.Millisecond))
	if existing := c.Writer.Header().Get("Server-Timing"); existing != "" {
		value = existing + ", " + value
	}
	c.Header("Server-Timing", value)
}

func dockerDisplayInvalidationMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		path := strings.TrimPrefix(c.Request.URL.Path, "/api")
		resource := false
		for _, prefix := range []string{"/containers", "/images", "/volumes", "/networks", "/compose", "/cleanup", "/appstore/deploy"} {
			if path == prefix || strings.HasPrefix(path, prefix+"/") {
				resource = true
				break
			}
		}
		write := c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead && c.Request.Method != http.MethodOptions
		if resource && write {
			dockerDiskDisplayCache.invalidate()
			defer dockerDiskDisplayCache.invalidate()
		}
		c.Next()
	}
}
