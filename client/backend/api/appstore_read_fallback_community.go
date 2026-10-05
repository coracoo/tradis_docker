//go:build community

package api

import (
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
)

func redactEditionAppStoreURL(text string) string {
	return text
}

func appStoreEditionTemplateSource() string {
	return appStoreSourceForURL(getAppStoreCDNURL())
}

func appStoreEditionHasReadFallback() bool {
	return false
}

func appStoreEditionFallbackAssetBase() string {
	return ""
}

func fetchEditionAppStoreListMetaFallback(bool) (*appStoreListMeta, error) {
	if meta := getCachedAppListMeta(); meta != nil {
		return meta, nil
	}
	return nil, errAppStoreContentSourceUnavailable
}

func fetchEditionAppStoreListFallback(bool, string, string) (appStoreListFallbackResult, error) {
	return appStoreListFallbackResult{}, errAppStoreContentSourceUnavailable
}

func fetchEditionAppStoreDetailFallback(string, appDetailCacheEntry, bool) (appStoreDetailFallbackResult, error) {
	return appStoreDetailFallbackResult{}, errAppStoreContentSourceUnavailable
}

func buildEditionAppStoreStatusSummary() gin.H {
	cdnURL := strings.TrimSpace(getAppStoreCDNURL())
	cache := summarizeAppStoreCache()
	cdnStatus := getCDNStatusSummary()

	mode := "unconfigured"
	if cdnURL != "" {
		mode = "cdn"
	} else if cache.HasListCache || cache.CachedDetailFile > 0 {
		mode = "cache"
	}

	state := "offline"
	level := "error"
	label := "不可用"
	summary := "应用商店内容源不可用"
	if mode == "cdn" {
		if text := strings.TrimSpace(fmt.Sprintf("%v", cdnStatus["summary"])); text != "" {
			summary = text
		} else {
			summary = "应用商店已配置 CDN"
		}
		state = "ready"
		level = "success"
		label = "可用"
		if strings.TrimSpace(fmt.Sprintf("%v", cdnStatus["bestIp"])) == "" {
			state = "degraded"
			level = "warning"
			label = "降级"
		}
	} else if mode == "cache" {
		state = "cached"
		level = "warning"
		label = "仅缓存"
		summary = "应用商店远端暂不可用，当前仅可使用本地缓存"
	}

	return gin.H{
		"connected":        mode != "unconfigured",
		"state":            state,
		"level":            level,
		"label":            label,
		"mode":             mode,
		"summary":          summary,
		"readConfigured":   cdnURL != "",
		"writeConfigured":  false,
		"cdnConfigured":    cdnURL != "",
		"originConfigured": false,
		"cache":            cache,
	}
}

func prepareEditionAppStoreDetail(app *App) {
	if app != nil {
		app.Tutorial = ""
	}
}
