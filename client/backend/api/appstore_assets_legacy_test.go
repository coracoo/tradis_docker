package api

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeAppListDiskCache(t *testing.T, apps []App) string {
	t.Helper()
	cacheDir := t.TempDir()
	originalGetCacheDir := getAppCacheDirFunc
	getAppCacheDirFunc = func() string { return cacheDir }
	appListCacheMu.Lock()
	appListCache = appListCacheState{}
	appListCacheMu.Unlock()
	t.Cleanup(func() {
		getAppCacheDirFunc = originalGetCacheDir
		appListCacheMu.Lock()
		appListCache = appListCacheState{}
		appListCacheMu.Unlock()
	})

	data, err := json.Marshal(appListDiskCache{
		Apps:        apps,
		VersionHash: "cached-version",
		FetchedAt:   time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(cacheDir, "app-list-cache.json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadAppListCacheRejectsLegacyAssetProxyURL(t *testing.T) {
	for _, testCase := range []struct {
		name string
		app  App
	}{
		{
			name: "logo",
			app:  App{Name: "legacy-logo", Logo: "/api/appstore/assets/logo.png"},
		},
		{
			name: "screenshot",
			app:  App{Name: "legacy-screenshot", Screenshots: []string{"/api/appstore/assets/screen.png"}},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			path := writeAppListDiskCache(t, []App{testCase.app})

			loadAppListCacheFromDisk()

			appListCacheMu.Lock()
			loadedCount := len(appListCache.apps)
			appListCacheMu.Unlock()
			if loadedCount != 0 {
				t.Fatalf("legacy cache loaded %d apps, want 0", loadedCount)
			}
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatalf("legacy cache file still exists: %v", err)
			}
		})
	}
}

func TestLoadAppListCacheKeepsCurrentAssetURLs(t *testing.T) {
	path := writeAppListDiskCache(t, []App{{
		Name:        "current",
		Logo:        "/uploads/logo.png",
		Screenshots: []string{"https://cdn.example/screen.png"},
	}})

	loadAppListCacheFromDisk()

	appListCacheMu.Lock()
	loaded := append([]App(nil), appListCache.apps...)
	appListCacheMu.Unlock()
	if len(loaded) != 1 || loaded[0].Name != "current" {
		t.Fatalf("current cache was not loaded: %#v", loaded)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("current cache file was removed: %v", err)
	}
}
