//go:build community

package api

import (
	"dockerpanel/backend/pkg/cloudflare"
	"dockerpanel/backend/pkg/deployment"
	"dockerpanel/backend/pkg/secrets"
	"dockerpanel/backend/pkg/settings"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

var communityAllowedKVRouteKeys = map[string]struct{}{
	kvOverviewStatOrderKey: {},
	kvResourceColumnWidths: {},
	kvResourceTableColumns: {},
	kvNavigationShowHidden: {},
	kvOnboardingTourKey:    {},
}

var communityAllowedSettingsFields = map[string]struct{}{
	"lanUrl":                        {},
	"wanUrl":                        {},
	"appStoreCDNURL":                {},
	"tutorialRSSURL":                {},
	"advancedMode":                  {},
	"allocPortStart":                {},
	"allocPortEnd":                  {},
	"allowAutoAllocPort":            {},
	"imageUpdateIntervalMinutes":    {},
	"aiEnabled":                     {},
	"aiBaseUrl":                     {},
	"aiApiKey":                      {},
	"aiModel":                       {},
	"aiUtilityModel":                {},
	"aiTemperature":                 {},
	"aiMaxTokens":                   {},
	"aiAllowCreateCategory":         {},
	"aiNavigationPrompt":            {},
	"aiComposePrompt":               {},
	"volumeBackupEnabled":           {},
	"volumeBackupImage":             {},
	"volumeBackupEnv":               {},
	"volumeBackupCronExpression":    {},
	"volumeBackupVolumes":           {},
	"volumeBackupArchiveDir":        {},
	"volumeBackupMountDockerSock":   {},
	"notificationEnabledCategories": {},
}

// DefaultClientVersion is written through ldflags by image builds. The source
// value keeps local community builds identifiable without an official service.
var DefaultClientVersion = "v0.9.7" // x-release-please-version

const (
	kvClientVersionKey     = "client_version"
	kvOverviewStatOrderKey = "overview_stat_order"
	kvResourceColumnWidths = "resource_column_widths"
	kvResourceTableColumns = "resource_table_columns"
	kvNavigationShowHidden = "navigation_show_hidden"
	kvOnboardingTourKey    = "onboarding_tour"
)

func normalizeKVRouteKey(raw string) (string, bool) {
	key := strings.TrimSpace(raw)
	if key == "" {
		return "", false
	}
	_, ok := communityAllowedKVRouteKeys[key]
	return key, ok
}

func RegisterSettingsRoutes(r *gin.RouterGroup) {
	group := r.Group("/settings")
	group.GET("", getGlobalSettings)
	group.POST("", updateGlobalSettings)
	group.GET("/global", getGlobalSettings)
	group.POST("/global", updateGlobalSettings)
	group.GET("/version-status", getVersionStatus)
	group.GET("/deployment-defaults", getCommunityDeploymentDefaults)
	group.PUT("/deployment-defaults", updateCommunityDeploymentDefaults)
	group.GET("/kv/:key", getKVSetting)
	group.POST("/kv/:key", setKVSetting)
	registerEditionVersionRoutes(group)
}

func getCommunityDeploymentDefaults(c *gin.Context) {
	profile, err := resolveEnvironmentProfile("local", deployment.ProfileOverrides{})
	if err != nil {
		respondError(c, http.StatusInternalServerError, "读取 NAS 部署默认值失败", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"profile": profile})
}

func updateCommunityDeploymentDefaults(c *gin.Context) {
	var overrides deployment.ProfileOverrides
	if err := c.ShouldBindJSON(&overrides); err != nil {
		respondError(c, http.StatusBadRequest, "NAS 部署默认值格式无效", err)
		return
	}
	profile, err := saveLocalEnvironmentProfile(overrides)
	if errors.Is(err, errInvalidLocalEnvironmentProfile) {
		respondError(c, http.StatusBadRequest, "NAS 部署默认值无效", err)
		return
	}
	if err != nil {
		respondError(c, http.StatusInternalServerError, "保存 NAS 部署默认值失败", err)
		return
	}
	recordLocalEnvironmentProfileUpdated()
	c.JSON(http.StatusOK, gin.H{"profile": profile})
}

func getGlobalSettings(c *gin.Context) {
	configured, err := settings.GetSettings()
	if err != nil {
		respondError(c, http.StatusInternalServerError, "Failed to get settings", err)
		return
	}
	c.JSON(http.StatusOK, configured)
}

type UpdateSettingsRequest struct {
	LanUrl                        *string  `json:"lanUrl"`
	WanUrl                        *string  `json:"wanUrl"`
	AppStoreCDNURL                *string  `json:"appStoreCDNURL"`
	TutorialRSSURL                *string  `json:"tutorialRSSURL"`
	AdvancedMode                  *bool    `json:"advancedMode"`
	AllocPortStart                *int     `json:"allocPortStart"`
	AllocPortEnd                  *int     `json:"allocPortEnd"`
	AllowAutoAllocPort            *bool    `json:"allowAutoAllocPort"`
	ImageUpdateIntervalMinutes    *int     `json:"imageUpdateIntervalMinutes"`
	AiEnabled                     *bool    `json:"aiEnabled"`
	AiBaseUrl                     *string  `json:"aiBaseUrl"`
	AiApiKey                      *string  `json:"aiApiKey"`
	AiModel                       *string  `json:"aiModel"`
	AiUtilityModel                *string  `json:"aiUtilityModel"`
	AiTemperature                 *float64 `json:"aiTemperature"`
	AiMaxTokens                   *int     `json:"aiMaxTokens"`
	AiAllowCreateCategory         *bool    `json:"aiAllowCreateCategory"`
	AiNavigationPrompt            *string  `json:"aiNavigationPrompt"`
	AiComposePrompt               *string  `json:"aiComposePrompt"`
	VolumeBackupEnabled           *bool    `json:"volumeBackupEnabled"`
	VolumeBackupImage             *string  `json:"volumeBackupImage"`
	VolumeBackupEnv               *string  `json:"volumeBackupEnv"`
	VolumeBackupCronExpression    *string  `json:"volumeBackupCronExpression"`
	VolumeBackupVolumes           []string `json:"volumeBackupVolumes"`
	VolumeBackupArchiveDir        *string  `json:"volumeBackupArchiveDir"`
	VolumeBackupMountDockerSock   *bool    `json:"volumeBackupMountDockerSock"`
	NotificationEnabledCategories []string `json:"notificationEnabledCategories"`
}

func mergeSettingsUpdate(current settings.Settings, req UpdateSettingsRequest) settings.Settings {
	merged := current
	if req.LanUrl != nil {
		merged.LanUrl = strings.TrimSpace(*req.LanUrl)
	}
	if req.WanUrl != nil {
		merged.WanUrl = strings.TrimSpace(*req.WanUrl)
	}
	if req.AppStoreCDNURL != nil {
		merged.AppStoreCDNURL = strings.TrimSpace(*req.AppStoreCDNURL)
	}
	if req.TutorialRSSURL != nil {
		merged.TutorialRSSURL = strings.TrimSpace(*req.TutorialRSSURL)
	}
	if req.AdvancedMode != nil {
		merged.AdvancedMode = *req.AdvancedMode
	}
	if req.AllocPortStart != nil {
		merged.AllocPortStart = *req.AllocPortStart
	}
	if req.AllocPortEnd != nil {
		merged.AllocPortEnd = *req.AllocPortEnd
	}
	if req.AllowAutoAllocPort != nil {
		merged.AllowAutoAllocPort = *req.AllowAutoAllocPort
	}
	if req.ImageUpdateIntervalMinutes != nil {
		merged.ImageUpdateIntervalMinutes = *req.ImageUpdateIntervalMinutes
	}
	if req.AiEnabled != nil {
		merged.AiEnabled = *req.AiEnabled
	}
	if req.AiBaseUrl != nil {
		merged.AiBaseUrl = strings.TrimSpace(*req.AiBaseUrl)
	}
	if req.AiModel != nil {
		merged.AiModel = strings.TrimSpace(*req.AiModel)
	}
	if req.AiUtilityModel != nil {
		merged.AiUtilityModel = strings.TrimSpace(*req.AiUtilityModel)
	}
	if req.AiTemperature != nil {
		merged.AiTemperature = *req.AiTemperature
	}
	if req.AiMaxTokens != nil {
		merged.AiMaxTokens = *req.AiMaxTokens
	}
	if req.AiAllowCreateCategory != nil {
		merged.AiAllowCreateCategory = *req.AiAllowCreateCategory
	}
	if req.AiNavigationPrompt != nil {
		merged.AiNavigationPrompt = strings.TrimSpace(*req.AiNavigationPrompt)
	}
	if req.AiComposePrompt != nil {
		merged.AiComposePrompt = strings.TrimSpace(*req.AiComposePrompt)
	}
	if req.VolumeBackupEnabled != nil {
		merged.VolumeBackupEnabled = *req.VolumeBackupEnabled
	}
	if req.VolumeBackupImage != nil {
		merged.VolumeBackupImage = strings.TrimSpace(*req.VolumeBackupImage)
	}
	if req.VolumeBackupEnv != nil {
		merged.VolumeBackupEnv = *req.VolumeBackupEnv
	}
	if req.VolumeBackupCronExpression != nil {
		merged.VolumeBackupCronExpression = strings.TrimSpace(*req.VolumeBackupCronExpression)
	}
	if req.VolumeBackupVolumes != nil {
		merged.VolumeBackupVolumes = req.VolumeBackupVolumes
	}
	if req.VolumeBackupArchiveDir != nil {
		merged.VolumeBackupArchiveDir = strings.TrimSpace(*req.VolumeBackupArchiveDir)
	}
	if req.VolumeBackupMountDockerSock != nil {
		merged.VolumeBackupMountDockerSock = *req.VolumeBackupMountDockerSock
	}
	if req.NotificationEnabledCategories != nil {
		merged.NotificationEnabledCategories = req.NotificationEnabledCategories
	}
	return merged
}

func updateGlobalSettings(c *gin.Context) {
	req, ok := bindCommunitySettingsUpdate(c)
	if !ok {
		return
	}
	if ((req.VolumeBackupEnv != nil && strings.TrimSpace(*req.VolumeBackupEnv) != "") ||
		(req.AiApiKey != nil && strings.TrimSpace(*req.AiApiKey) != "")) && !secrets.Available() {
		respondError(c, http.StatusInternalServerError, "Sensitive settings storage is unavailable", secrets.ErrMasterKeyUnavailable)
		return
	}
	current, err := settings.GetSettings()
	if err != nil {
		respondError(c, http.StatusInternalServerError, "Failed to get settings", err)
		return
	}
	merged := mergeSettingsUpdate(current, req)
	if merged.TutorialRSSURL != "" && !communityTutorialRSSURLAllowed(merged.TutorialRSSURL, merged.AppStoreCDNURL) {
		respondError(c, http.StatusBadRequest, "公开教程 RSS 地址必须位于当前 CDN 或公开模板仓库的教程目录", nil)
		return
	}
	if err := settings.UpdateSettings(merged); err != nil {
		respondError(c, http.StatusInternalServerError, "Failed to update settings", err)
		return
	}
	if req.AiApiKey != nil {
		if err := settings.SetSensitiveValue("ai_api_key", *req.AiApiKey); err != nil {
			respondError(c, http.StatusInternalServerError, "Failed to update AI api key", err)
			return
		}
	}
	if req.VolumeBackupEnv != nil {
		if err := settings.SetSensitiveValue("volume_backup_env", *req.VolumeBackupEnv); err != nil {
			respondError(c, http.StatusInternalServerError, "Failed to update volume backup environment", err)
			return
		}
	}
	if updated, err := settings.GetSettings(); err == nil {
		cloudflare.NotifyCDNURLChanged(current.AppStoreCDNURL, updated.AppStoreCDNURL)
		if strings.TrimSpace(current.AppStoreCDNURL) != strings.TrimSpace(updated.AppStoreCDNURL) {
			if err := invalidateAppStoreSourceCache(); err != nil {
				log.Printf("[appstore] 清理旧来源缓存失败: %v", err)
			}
		}
	}
	ScheduleSettingsMaintenance(merged)
	c.JSON(http.StatusOK, gin.H{"message": "Settings updated successfully"})
}

func bindCommunitySettingsUpdate(c *gin.Context) (UpdateSettingsRequest, bool) {
	var raw map[string]json.RawMessage
	if err := c.ShouldBindJSON(&raw); err != nil {
		respondError(c, http.StatusBadRequest, "Invalid request", err)
		return UpdateSettingsRequest{}, false
	}
	for field := range raw {
		if _, allowed := communityAllowedSettingsFields[field]; !allowed {
			respondErrorWithCode(c, http.StatusBadRequest, "community_feature_unavailable", "当前社区版不支持该设置", nil)
			return UpdateSettingsRequest{}, false
		}
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		respondError(c, http.StatusBadRequest, "Invalid request", err)
		return UpdateSettingsRequest{}, false
	}
	var req UpdateSettingsRequest
	if err := json.Unmarshal(encoded, &req); err != nil {
		respondError(c, http.StatusBadRequest, "Invalid request", err)
		return UpdateSettingsRequest{}, false
	}
	return req, true
}

func getKVSetting(c *gin.Context) {
	key, ok := normalizeKVRouteKey(c.Param("key"))
	if !ok {
		respondError(c, http.StatusNotFound, "Unsupported key", nil)
		return
	}
	value, err := settings.GetValue(key)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "Failed to get value", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"key": key, "value": value})
}

type kvRequest struct {
	Value string `json:"value"`
}

func setKVSetting(c *gin.Context) {
	key, ok := normalizeKVRouteKey(c.Param("key"))
	if !ok {
		respondError(c, http.StatusNotFound, "Unsupported key", nil)
		return
	}
	var req kvRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "Invalid request", err)
		return
	}
	if err := settings.SetValue(key, req.Value); err != nil {
		respondError(c, http.StatusInternalServerError, "Failed to set value", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "ok"})
}

type versionStatusResponse struct {
	LocalVersion     string `json:"localVersion"`
	ServerVersion    string `json:"serverVersion"`
	HasNewVersion    bool   `json:"hasNewVersion"`
	VersionCheckedAt string `json:"versionCheckedAt"`
	Channel          string `json:"channel"`
	Mandatory        bool   `json:"mandatory"`
	Release          any    `json:"release"`
}

func getVersionStatus(c *gin.Context) {
	status, err := editionVersionStatus()
	if err != nil {
		respondError(c, http.StatusInternalServerError, "Failed to get version status", err)
		return
	}
	c.JSON(http.StatusOK, status)
}

func InitClientVersionFromEnv() {
	if version := resolveLocalClientVersion(); version != "" {
		_ = settings.SetValue(kvClientVersionKey, version)
	}
}

func resolveLocalClientVersion() string {
	version := strings.TrimSpace(os.Getenv("CLIENT_VERSION"))
	if version == "" {
		version = strings.TrimSpace(os.Getenv("DOCKPIER_CLIENT_VERSION"))
	}
	if version != "" {
		return normalizeClientVersion(version)
	}
	if version = strings.TrimSpace(DefaultClientVersion); version != "" {
		return normalizeClientVersion(version)
	}
	version, _ = settings.GetValue(kvClientVersionKey)
	return normalizeClientVersion(version)
}

func normalizeClientVersion(raw string) string {
	version := strings.TrimSpace(raw)
	if version == "" {
		return ""
	}
	if !strings.HasPrefix(strings.ToLower(version), "v") {
		return "v" + version
	}
	return version
}

func compareSemver(a, b string) int {
	parse := func(raw string) ([3]int, string) {
		value := strings.TrimPrefix(strings.TrimSpace(raw), "v")
		value = strings.SplitN(value, "+", 2)[0]
		parts := strings.SplitN(value, "-", 2)
		numbers := strings.Split(parts[0], ".")
		var parsed [3]int
		for index := 0; index < len(parsed) && index < len(numbers); index++ {
			parsed[index], _ = strconv.Atoi(numbers[index])
		}
		preRelease := ""
		if len(parts) == 2 {
			preRelease = parts[1]
		}
		return parsed, preRelease
	}
	left, leftPre := parse(a)
	right, rightPre := parse(b)
	for index := range left {
		if left[index] > right[index] {
			return 1
		}
		if left[index] < right[index] {
			return -1
		}
	}
	if leftPre == rightPre {
		return 0
	}
	if leftPre == "" {
		return 1
	}
	if rightPre == "" {
		return -1
	}
	if leftPre > rightPre {
		return 1
	}
	return -1
}
