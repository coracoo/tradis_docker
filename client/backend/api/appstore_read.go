package api

import (
	"crypto/sha256"
	"dockerpanel/backend/pkg/cloudflare"
	"dockerpanel/backend/pkg/logging"
	"dockerpanel/backend/pkg/settings"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

var (
	settingsGetSettings = settings.GetSettings
	getAppCacheDirFunc  = func() string {
		return filepath.Join(settings.GetDataDir(), "apps")
	}
)

const (
	appStoreSourceOfficial = "official"
	appStoreSourceCustom   = "custom"
)

var errAppStoreContentSourceUnavailable = errors.New("应用商店内容源不可用")

func getAppCacheDir() string {
	return getAppCacheDirFunc()
}

func redactAppStoreURL(text string) string {
	redacted := settings.RedactAppStoreURL(text)
	if redacted == "" {
		return redacted
	}
	return redactEditionAppStoreURL(redacted)
}

func appStoreSourceForURL(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	u, err := url.Parse(value)
	if err != nil {
		return appStoreSourceCustom
	}
	host := strings.ToLower(strings.TrimSpace(u.Host))
	if host == "" {
		return appStoreSourceCustom
	}
	if isOfficialAppStoreHost(host) {
		return appStoreSourceOfficial
	}
	return appStoreSourceCustom
}

func currentAppStoreTemplateSource() string {
	return appStoreEditionTemplateSource()
}

func markAppStoreSource(apps []App, source string) {
	source = strings.TrimSpace(source)
	if source == "" {
		return
	}
	for i := range apps {
		if strings.TrimSpace(apps[i].Source) == "" {
			apps[i].Source = source
		}
		setEditionAppStoreSource(&apps[i])
	}
}

func resolveAppStoreAssetURL(raw string, base string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	if strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") || strings.HasPrefix(value, "data:") {
		return value
	}
	if strings.HasPrefix(value, "/uploads/") {
		base = strings.TrimRight(strings.TrimSpace(base), "/")
		if base != "" {
			return base + value
		}
	}
	return value
}

func resolveAppStoreAssets(app *App, base string) {
	if app == nil {
		return
	}
	app.Logo = resolveAppStoreAssetURL(app.Logo, base)
	for i, screenshot := range app.Screenshots {
		app.Screenshots[i] = resolveAppStoreAssetURL(screenshot, base)
	}
}

func containsLegacyAppStoreAssetProxyURL(apps []App) bool {
	const legacyPrefix = "/api/appstore/assets/"
	for _, app := range apps {
		if strings.HasPrefix(strings.TrimSpace(app.Logo), legacyPrefix) {
			return true
		}
		for _, screenshot := range app.Screenshots {
			if strings.HasPrefix(strings.TrimSpace(screenshot), legacyPrefix) {
				return true
			}
		}
	}
	return false
}

func getAppStoreCDNURL() string {
	s, err := settingsGetSettings()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(s.AppStoreCDNURL)
}

func getCDNClient() *cloudflare.CDNClient {
	cdnURL := getAppStoreCDNURL()
	if cdnURL == "" {
		return nil
	}
	return cloudflare.NewCDNClient(cdnURL)
}

func hasAppStoreReadSource() bool {
	return getCDNClient() != nil || appStoreEditionHasReadFallback()
}

type appListCacheState struct {
	fetchedAt     time.Time
	apps          []App
	versionHash   string
	etag          string
	lastModified  string
	lastErrAt     time.Time
	lastErrDigest string
}

var appListCacheMu sync.Mutex
var appListCache appListCacheState

type appListDiskCache struct {
	Apps         []App     `json:"apps"`
	VersionHash  string    `json:"version_hash"`
	FetchedAt    time.Time `json:"fetched_at"`
	ETag         string    `json:"etag,omitempty"`
	LastModified string    `json:"last_modified,omitempty"`
}

func getAppListCacheMetaPath() string {
	return filepath.Join(getAppCacheDir(), "app-list-cache.json")
}

func saveAppListCacheToDisk() error {
	appListCacheMu.Lock()
	defer appListCacheMu.Unlock()
	if len(appListCache.apps) == 0 {
		_ = os.Remove(getAppListCacheMetaPath())
		return nil
	}
	if err := os.MkdirAll(getAppCacheDir(), 0755); err != nil {
		return err
	}
	data, err := json.Marshal(appListDiskCache{
		Apps:         appListCache.apps,
		VersionHash:  appListCache.versionHash,
		FetchedAt:    appListCache.fetchedAt,
		ETag:         appListCache.etag,
		LastModified: appListCache.lastModified,
	})
	if err != nil {
		return err
	}
	return os.WriteFile(getAppListCacheMetaPath(), data, 0644)
}

func loadAppListCacheFromDisk() {
	appListCacheMu.Lock()
	defer appListCacheMu.Unlock()
	if len(appListCache.apps) > 0 || appListCache.versionHash != "" {
		return
	}
	data, err := os.ReadFile(getAppListCacheMetaPath())
	if err != nil {
		return
	}
	var dc appListDiskCache
	if err := json.Unmarshal(data, &dc); err != nil {
		return
	}
	if len(dc.Apps) == 0 {
		return
	}
	if containsLegacyAppStoreAssetProxyURL(dc.Apps) {
		if err := os.Remove(getAppListCacheMetaPath()); err != nil && !errors.Is(err, os.ErrNotExist) {
			logging.Warn("expired AppStore list cache could not be removed", "error", err)
		}
		return
	}
	appListCache.apps = dc.Apps
	appListCache.versionHash = strings.TrimSpace(dc.VersionHash)
	appListCache.fetchedAt = dc.FetchedAt
	appListCache.etag = dc.ETag
	appListCache.lastModified = dc.LastModified
}

type appDetailCacheEntry struct {
	fetchedAt     time.Time
	app           App
	etag          string
	lastModified  string
	lastErrAt     time.Time
	lastErrDigest string
}

var appDetailCacheMu sync.Mutex
var appDetailCache = make(map[string]appDetailCacheEntry)

func invalidateAppStoreSourceCache() error {
	appListCacheMu.Lock()
	appListCache = appListCacheState{}
	appListCacheMu.Unlock()

	appDetailCacheMu.Lock()
	appDetailCache = make(map[string]appDetailCacheEntry)
	appDetailCacheMu.Unlock()

	files, err := os.ReadDir(getAppCacheDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(strings.ToLower(file.Name()), ".json") {
			continue
		}
		if err := os.Remove(filepath.Join(getAppCacheDir(), file.Name())); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

type appStoreCacheSummary struct {
	HasListCache     bool      `json:"hasListCache"`
	ListAppCount     int       `json:"listAppCount"`
	ListFetchedAt    time.Time `json:"listFetchedAt"`
	ListVersionHash  string    `json:"listVersionHash"`
	CachedDetailFile int       `json:"cachedDetailFileCount"`
}

type appStoreListMeta struct {
	VersionHash string `json:"version_hash"`
	GeneratedAt string `json:"generated_at,omitempty"`
	TotalCount  int    `json:"total_count"`
	Source      string `json:"source,omitempty"`
	FetchedAt   string `json:"fetched_at,omitempty"`
}

type appStoreMetaStatus struct {
	State        string            `json:"state"`
	Summary      string            `json:"summary"`
	InSync       bool              `json:"in_sync"`
	NeedsRefresh bool              `json:"needs_refresh"`
	Local        *appStoreListMeta `json:"local,omitempty"`
	Remote       *appStoreListMeta `json:"remote,omitempty"`
}

type appStoreListFallbackResult struct {
	Apps         []App
	AssetBase    string
	ETag         string
	LastModified string
	NotModified  bool
}

type appStoreDetailFallbackResult struct {
	App          *App
	AssetBase    string
	ETag         string
	LastModified string
	NotModified  bool
}

type appStoreVersionItem struct {
	ID          uint   `json:"id"`
	SortOrder   int    `json:"sort_order"`
	Name        string `json:"name"`
	Category    string `json:"category"`
	Description string `json:"description"`
	Version     string `json:"version"`
	Logo        string `json:"logo"`
	Website     string `json:"website"`
}

func buildAppListMeta(apps []App, source string, generatedAt time.Time) *appStoreListMeta {
	items := make([]appStoreVersionItem, 0, len(apps))
	for _, app := range apps {
		items = append(items, appStoreVersionItem{
			ID:          app.ID,
			SortOrder:   app.SortOrder,
			Name:        app.Name,
			Category:    app.Category,
			Description: app.Description,
			Version:     app.Version,
			Logo:        app.Logo,
			Website:     app.Website,
		})
	}
	payload, err := json.Marshal(items)
	if err != nil {
		return nil
	}
	sum := sha256.Sum256(payload)
	if generatedAt.IsZero() {
		generatedAt = time.Now()
	}
	return &appStoreListMeta{
		VersionHash: "sha256:" + hex.EncodeToString(sum[:]),
		GeneratedAt: generatedAt.UTC().Format(time.RFC3339),
		TotalCount:  len(items),
		Source:      source,
	}
}

func buildAppListMetaFromManifests(templates []cloudflare.TemplateManifest, source string, generatedAt time.Time) *appStoreListMeta {
	apps := make([]App, len(templates))
	for i, template := range templates {
		apps[i] = App{
			ID:              template.ID,
			SortOrder:       template.SortOrder,
			Name:            template.Name,
			Category:        template.Category,
			Description:     template.Description,
			Version:         template.Version,
			Logo:            template.Logo,
			Website:         template.Website,
			DeploymentCount: 0,
		}
	}
	return buildAppListMeta(apps, source, generatedAt)
}

func summarizeAppStoreCache() appStoreCacheSummary {
	summary := appStoreCacheSummary{}

	loadAppListCacheFromDisk()

	appListCacheMu.Lock()
	summary.HasListCache = len(appListCache.apps) > 0
	summary.ListAppCount = len(appListCache.apps)
	summary.ListFetchedAt = appListCache.fetchedAt
	summary.ListVersionHash = appListCache.versionHash
	appListCacheMu.Unlock()

	files, err := os.ReadDir(getAppCacheDir())
	if err != nil {
		return summary
	}
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		name := strings.ToLower(file.Name())
		if strings.HasSuffix(name, ".json") && name != "app-list-cache.json" && !isEditionAppStoreCacheFile(name) {
			summary.CachedDetailFile++
		}
	}
	return summary
}

func getCachedAppListMeta() *appStoreListMeta {
	loadAppListCacheFromDisk()

	appListCacheMu.Lock()
	defer appListCacheMu.Unlock()
	if appListCache.versionHash == "" && len(appListCache.apps) > 0 {
		if meta := buildAppListMeta(appListCache.apps, "cache", appListCache.fetchedAt); meta != nil {
			appListCache.versionHash = meta.VersionHash
		}
	}
	if appListCache.versionHash == "" {
		return nil
	}
	return &appStoreListMeta{
		VersionHash: appListCache.versionHash,
		TotalCount:  len(appListCache.apps),
		Source:      "cache",
		FetchedAt:   appListCache.fetchedAt.Format(time.RFC3339),
	}
}

func fetchRemoteAppListMeta(forceRefresh bool) (*appStoreListMeta, error) {
	if cdnClient := getCDNClient(); cdnClient != nil {
		path := "/api/templates/meta"
		if forceRefresh {
			path = fmt.Sprintf("%s?refresh=%d", path, time.Now().UnixNano())
		}
		if data, err := cdnClient.FetchRaw(path); err == nil {
			var meta appStoreListMeta
			if unmarshalErr := json.Unmarshal(data, &meta); unmarshalErr == nil && strings.TrimSpace(meta.VersionHash) != "" {
				meta.Source = "cdn"
				return &meta, nil
			}
		}
		if templates, err := cdnClient.FetchTemplates(); err == nil {
			if meta := buildAppListMetaFromManifests(templates, "cdn-list", time.Now()); meta != nil {
				return meta, nil
			}
		}
	}

	return fetchEditionAppStoreListMetaFallback(forceRefresh)
}

func getAppStoreListMeta(c *gin.Context) {
	meta, err := fetchRemoteAppListMeta(c.Query("refresh") == "1" || c.Query("force") == "1")
	if err != nil {
		respondError(c, http.StatusServiceUnavailable, "获取应用商店元数据失败", err)
		return
	}
	c.JSON(http.StatusOK, meta)
}

func buildAppStoreMetaStatus(local, remote *appStoreListMeta) appStoreMetaStatus {
	status := appStoreMetaStatus{
		State:   "unknown",
		Summary: "尚未获取模板版本信息",
		Local:   local,
		Remote:  remote,
	}

	if local == nil && remote == nil {
		status.State = "empty"
		status.Summary = "当前既没有本地模板缓存，也无法读取远端模板版本"
		status.NeedsRefresh = true
		return status
	}

	if remote == nil || strings.TrimSpace(remote.Source) == "cache" {
		status.State = "cache_only"
		status.Summary = "当前仅能读取本地缓存，暂时无法确认远端模板版本"
		status.NeedsRefresh = local == nil
		return status
	}

	if local == nil || strings.TrimSpace(local.VersionHash) == "" {
		status.State = "missing_local"
		status.Summary = "已检测到远端模板版本，但本地还没有对应缓存"
		status.NeedsRefresh = true
		return status
	}

	if strings.TrimSpace(local.VersionHash) == strings.TrimSpace(remote.VersionHash) {
		status.State = "synced"
		status.Summary = "本地模板缓存与远端版本一致"
		status.InSync = true
		return status
	}

	status.State = "outdated"
	status.Summary = "远端模板版本已更新，建议刷新本地模板列表"
	status.NeedsRefresh = true
	return status
}

func getAppStoreMetaStatus(c *gin.Context) {
	forceRefresh := c.Query("refresh") == "1" || c.Query("force") == "1"
	local := getCachedAppListMeta()
	remote, _ := fetchRemoteAppListMeta(forceRefresh)
	c.JSON(http.StatusOK, buildAppStoreMetaStatus(local, remote))
}

func getAppStoreStatus(c *gin.Context) {
	c.JSON(http.StatusOK, buildEditionAppStoreStatusSummary())
}

func listApps(c *gin.Context) {
	const ttl = 60 * time.Second
	forceRefresh := c.Query("refresh") == "1" || c.Query("force") == "1"

	loadAppListCacheFromDisk()
	if err := os.MkdirAll(getAppCacheDir(), 0755); err != nil {
		respondError(c, http.StatusInternalServerError, "创建缓存目录失败", err)
		return
	}

	now := time.Now()
	appListCacheMu.Lock()
	cachedApps := append([]App(nil), appListCache.apps...)
	cachedAt := appListCache.fetchedAt
	versionHash := appListCache.versionHash
	etag := appListCache.etag
	lastModified := appListCache.lastModified
	appListCacheMu.Unlock()
	cachedAssetBase := getAppStoreCDNURL()
	if strings.TrimSpace(cachedAssetBase) == "" {
		cachedAssetBase = appStoreEditionFallbackAssetBase()
	}
	markAppStoreSource(cachedApps, currentAppStoreTemplateSource())
	for i := range cachedApps {
		resolveAppStoreAssets(&cachedApps[i], cachedAssetBase)
	}
	applyEditionAppStoreCounts(cachedApps)

	if !forceRefresh && len(cachedApps) > 0 && !cachedAt.IsZero() && now.Sub(cachedAt) < ttl {
		c.JSON(http.StatusOK, cachedApps)
		return
	}

	var remoteMeta *appStoreListMeta
	if !forceRefresh {
		remoteMeta, _ = fetchRemoteAppListMeta(false)
		if len(cachedApps) > 0 && remoteMeta != nil && strings.TrimSpace(remoteMeta.VersionHash) != "" && remoteMeta.VersionHash == versionHash {
			appListCacheMu.Lock()
			appListCache.fetchedAt = now
			appListCache.versionHash = remoteMeta.VersionHash
			appListCacheMu.Unlock()
			c.JSON(http.StatusOK, cachedApps)
			return
		}
	}
	if !hasAppStoreReadSource() {
		if len(cachedApps) > 0 {
			c.JSON(http.StatusOK, cachedApps)
			return
		}
		respondError(c, http.StatusServiceUnavailable, "应用商店内容源不可用", errAppStoreContentSourceUnavailable)
		return
	}

	cdnClient := getCDNClient()
	if cdnClient != nil {
		logging.Debug("AppStore template list fetch using CDN route")
		var (
			templates []cloudflare.TemplateManifest
			err       error
		)
		if forceRefresh {
			refreshPath := fmt.Sprintf("/api/templates?refresh=%d", now.UnixNano())
			var data []byte
			data, err = cdnClient.FetchRaw(refreshPath)
			if err == nil {
				err = json.Unmarshal(data, &templates)
			}
		} else {
			templates, err = cdnClient.FetchTemplates()
		}
		if err == nil {
			logging.Debug("AppStore template list fetched from CDN", "count", len(templates))
			listMeta := buildAppListMetaFromManifests(templates, "cdn-list", now)
			apps := make([]App, len(templates))
			for i, t := range templates {
				apps[i] = App{
					ID:              t.ID,
					SortOrder:       t.SortOrder,
					Name:            t.Name,
					Category:        t.Category,
					Description:     t.Description,
					Version:         t.Version,
					Logo:            t.Logo,
					Website:         t.Website,
					DeploymentCount: 0,
					Source:          appStoreSourceForURL(getAppStoreCDNURL()),
				}
				resolveAppStoreAssets(&apps[i], getAppStoreCDNURL())
			}
			applyEditionAppStoreCounts(apps)
			appListCacheMu.Lock()
			appListCache.apps = apps
			appListCache.fetchedAt = now
			if remoteMeta != nil {
				appListCache.versionHash = remoteMeta.VersionHash
			} else if listMeta != nil {
				appListCache.versionHash = listMeta.VersionHash
			}
			appListCacheMu.Unlock()
			_ = saveAppListCacheToDisk()
			c.JSON(http.StatusOK, apps)
			return
		} else {
			logging.Warn("AppStore CDN fetch failed; using official fallback")
		}
	}

	fallback, err := fetchEditionAppStoreListFallback(forceRefresh, etag, lastModified)
	if err != nil {
		if len(cachedApps) > 0 {
			appListCacheMu.Lock()
			if now.Sub(appListCache.lastErrAt) > 60*time.Second || appListCache.lastErrDigest != err.Error() {
				appListCache.lastErrAt = now
				appListCache.lastErrDigest = err.Error()
				logging.Warn("AppStore content source fetch failed; using local cache")
			}
			appListCacheMu.Unlock()
			c.JSON(http.StatusOK, cachedApps)
			return
		}
		respondError(c, http.StatusServiceUnavailable, "应用商店内容源不可用", err)
		return
	}

	if fallback.NotModified && len(cachedApps) > 0 {
		appListCacheMu.Lock()
		appListCache.fetchedAt = now
		if remoteMeta != nil {
			appListCache.versionHash = remoteMeta.VersionHash
		}
		appListCacheMu.Unlock()
		c.JSON(http.StatusOK, cachedApps)
		return
	}

	apps := fallback.Apps
	listMeta := buildAppListMeta(apps, "origin-list", now)
	markAppStoreSource(apps, appStoreSourceForURL(fallback.AssetBase))
	for i := range apps {
		resolveAppStoreAssets(&apps[i], fallback.AssetBase)
	}
	applyEditionAppStoreCounts(apps)

	appListCacheMu.Lock()
	appListCache.apps = append([]App(nil), apps...)
	appListCache.fetchedAt = now
	if remoteMeta != nil {
		appListCache.versionHash = remoteMeta.VersionHash
	} else if listMeta != nil {
		appListCache.versionHash = listMeta.VersionHash
	}
	if fallback.ETag != "" {
		appListCache.etag = fallback.ETag
	}
	if fallback.LastModified != "" {
		appListCache.lastModified = fallback.LastModified
	}
	appListCacheMu.Unlock()
	_ = saveAppListCacheToDisk()

	c.JSON(http.StatusOK, apps)
}

func getAppFromCacheOrServer(idOrName string) (*App, error) {
	logging.Debug("AppStore template lookup started", "template", idOrName)

	key := strings.TrimSpace(idOrName)
	if key == "" {
		return nil, fmt.Errorf("无法获取应用详情 (ID/Name: %s)", idOrName)
	}

	const ttl = 60 * time.Second
	now := time.Now()

	appDetailCacheMu.Lock()
	entry, hasEntry := appDetailCache[key]
	appDetailCacheMu.Unlock()

	if hasEntry && strings.TrimSpace(entry.app.Name) != "" && !entry.fetchedAt.IsZero() && now.Sub(entry.fetchedAt) < ttl {
		cp := entry.app
		if strings.TrimSpace(cp.Source) == "" {
			cp.Source = currentAppStoreTemplateSource()
		}
		prepareEditionAppStoreDetail(&cp)
		applyEditionAppStoreCount(&cp)
		return &cp, nil
	}
	if !hasAppStoreReadSource() {
		return nil, fmt.Errorf("应用商店内容源不可用")
	}

	cdnClient := getCDNClient()
	if cdnClient != nil {
		logging.Debug("AppStore template detail fetch using CDN route")
		if template, err := cdnClient.FetchTemplate(key); err == nil {
			logging.Debug("AppStore template detail fetched from CDN", "template", template.Name)
			app := App{
				ID:                 template.ID,
				SortOrder:          template.SortOrder,
				Name:               template.Name,
				Category:           template.Category,
				Description:        template.Description,
				Version:            template.Version,
				Logo:               template.Logo,
				Website:            template.Website,
				Tutorial:           template.Tutorial,
				Dotenv:             template.Dotenv,
				Compose:            template.Compose,
				Screenshots:        template.Screenshots,
				Schema:             convertCloudflareVariables(template.Schema),
				SourceFiles:        template.SourceFiles,
				InputMetadata:      template.InputMetadata,
				Manifest:           template.Manifest,
				SourceDigest:       template.SourceDigest,
				ManifestDigest:     template.ManifestDigest,
				CompilerVersion:    template.CompilerVersion,
				CompileDiagnostics: template.CompileDiagnostics,
				DeploymentCount:    0,
				Source:             appStoreSourceForURL(getAppStoreCDNURL()),
			}
			resolveAppStoreAssets(&app, getAppStoreCDNURL())
			prepareEditionAppStoreDetail(&app)
			applyEditionAppStoreCount(&app)
			next := appDetailCacheEntry{
				fetchedAt: now,
				app:       app,
			}
			appDetailCacheMu.Lock()
			appDetailCache[key] = next
			appDetailCache[app.Name] = next
			appDetailCache[fmt.Sprintf("%v", app.ID)] = next
			appDetailCacheMu.Unlock()
			_ = writeAppDetailCache(&app)
			return &app, nil
		} else {
			logging.Warn("AppStore CDN detail fetch failed")
		}
	}

	fallback, fallbackErr := fetchEditionAppStoreDetailFallback(key, entry, hasEntry)
	if fallbackErr == nil && fallback.NotModified && hasEntry && strings.TrimSpace(entry.app.Name) != "" {
		appDetailCacheMu.Lock()
		next := appDetailCache[key]
		next.fetchedAt = now
		appDetailCache[key] = next
		appDetailCacheMu.Unlock()
		cp := entry.app
		if strings.TrimSpace(cp.Source) == "" {
			cp.Source = currentAppStoreTemplateSource()
		}
		prepareEditionAppStoreDetail(&cp)
		applyEditionAppStoreCount(&cp)
		return &cp, nil
	}
	if fallbackErr == nil && fallback.App != nil && strings.TrimSpace(fallback.App.Name) != "" {
		app := *fallback.App
		if strings.TrimSpace(app.Source) == "" {
			app.Source = appStoreSourceForURL(fallback.AssetBase)
		}
		resolveAppStoreAssets(&app, fallback.AssetBase)
		prepareEditionAppStoreDetail(&app)
		applyEditionAppStoreCount(&app)
		next := appDetailCacheEntry{fetchedAt: now, app: app}
		if hasEntry {
			next.etag = entry.etag
			next.lastModified = entry.lastModified
		}
		if fallback.ETag != "" {
			next.etag = fallback.ETag
		}
		if fallback.LastModified != "" {
			next.lastModified = fallback.LastModified
		}
		appDetailCacheMu.Lock()
		appDetailCache[key] = next
		appDetailCache[app.Name] = next
		appDetailCache[fmt.Sprintf("%v", app.ID)] = next
		appDetailCacheMu.Unlock()
		_ = writeAppDetailCache(&app)
		return &app, nil
	}

	if hasEntry && strings.TrimSpace(entry.app.Name) != "" {
		cp := entry.app
		if strings.TrimSpace(cp.Source) == "" {
			cp.Source = currentAppStoreTemplateSource()
		}
		prepareEditionAppStoreDetail(&cp)
		applyEditionAppStoreCount(&cp)
		return &cp, nil
	}

	var app App
	_ = os.MkdirAll(getAppCacheDir(), 0755)
	files, derr := os.ReadDir(getAppCacheDir())
	assetBase := appStoreEditionFallbackAssetBase()
	if strings.TrimSpace(assetBase) == "" {
		assetBase = getAppStoreCDNURL()
	}
	if derr == nil {
		for _, file := range files {
			if file.Name() == key+".json" {
				content, _ := os.ReadFile(filepath.Join(getAppCacheDir(), file.Name()))
				_ = json.Unmarshal(content, &app)
				if strings.TrimSpace(app.Name) != "" {
					resolveAppStoreAssets(&app, assetBase)
					prepareEditionAppStoreDetail(&app)
					applyEditionAppStoreCount(&app)
					return &app, nil
				}
			}

			content, err := os.ReadFile(filepath.Join(getAppCacheDir(), file.Name()))
			if err == nil {
				var cachedApp App
				if err := json.Unmarshal(content, &cachedApp); err == nil {
					if fmt.Sprintf("%v", cachedApp.ID) == key && strings.TrimSpace(cachedApp.Name) != "" {
						resolveAppStoreAssets(&cachedApp, assetBase)
						prepareEditionAppStoreDetail(&cachedApp)
						applyEditionAppStoreCount(&cachedApp)
						return &cachedApp, nil
					}
				}
			}
		}
	}

	return nil, fmt.Errorf("无法获取应用详情 (ID/Name: %s)", idOrName)
}

func getApp(c *gin.Context) {
	id := c.Param("id")
	app, err := getAppFromCacheOrServer(id)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "获取应用详情失败", err)
		return
	}
	c.JSON(http.StatusOK, enrichAppDetailManifest(app))
}
