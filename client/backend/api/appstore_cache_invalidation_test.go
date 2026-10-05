package api

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestInvalidateAppStoreSourceCache(t *testing.T) {
	originalGetCacheDir := getAppCacheDirFunc
	cacheDir := t.TempDir()
	getAppCacheDirFunc = func() string { return cacheDir }
	t.Cleanup(func() { getAppCacheDirFunc = originalGetCacheDir })

	appListCacheMu.Lock()
	appListCache = appListCacheState{
		apps:      []App{{Name: "cached"}},
		fetchedAt: time.Now(),
	}
	appListCacheMu.Unlock()
	appDetailCacheMu.Lock()
	appDetailCache = map[string]appDetailCacheEntry{"cached": {app: App{Name: "cached"}}}
	appDetailCacheMu.Unlock()

	if err := os.WriteFile(filepath.Join(cacheDir, "app-list-cache.json"), []byte(`{}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "cached.json"), []byte(`{}`), 0644); err != nil {
		t.Fatal(err)
	}

	if err := invalidateAppStoreSourceCache(); err != nil {
		t.Fatal(err)
	}

	appListCacheMu.Lock()
	listCount := len(appListCache.apps)
	appListCacheMu.Unlock()
	appDetailCacheMu.Lock()
	detailCount := len(appDetailCache)
	appDetailCacheMu.Unlock()
	if listCount != 0 || detailCount != 0 {
		t.Fatalf("expected memory caches to be empty: list=%d detail=%d", listCount, detailCount)
	}
	if _, err := os.Stat(filepath.Join(cacheDir, "app-list-cache.json")); !os.IsNotExist(err) {
		t.Fatalf("expected disk list cache to be removed, err=%v", err)
	}
}
