package settings

import (
	"database/sql"
	"dockerpanel/backend/pkg/database"
	"dockerpanel/backend/pkg/logging"
	"dockerpanel/backend/pkg/secrets"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

var (
	ErrUnsupportedSensitiveSetting   = errors.New("unsupported sensitive setting")
	ErrSensitiveValueRequiresReentry = errors.New("stored sensitive value requires user re-entry")
)

var sensitiveSettingKeys = map[string]struct{}{
	"volume_backup_env": {},
	"ai_api_key":       {},
}

type ImageRemoteDigestBackoffPolicy struct {
	FirstFailBackoff   time.Duration
	SecondFailBackoff  time.Duration
	MaxConsecutiveFail int
}

func GetImageRemoteDigestBackoffPolicy() ImageRemoteDigestBackoffPolicy {
	return ImageRemoteDigestBackoffPolicy{
		FirstFailBackoff:   24 * time.Hour,
		SecondFailBackoff:  48 * time.Hour,
		MaxConsecutiveFail: 3,
	}
}

// IsDebugEnabled 判断是否启用调试日志（通过环境变量控制）。
func IsDebugEnabled() bool {
	return logging.IsDebugEnabled()
}

// InitSettingsTable 初始化设置表
func InitSettingsTable() error {
	db := database.GetDB()
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS global_settings (
			key TEXT PRIMARY KEY,
			value TEXT
		);
	`)
	if err != nil {
		return err
	}

	// Helper to insert default if not exists
	insertDefault := func(key, value string) error {
		var val string
		err := db.QueryRow("SELECT value FROM global_settings WHERE key = ?", key).Scan(&val)
		if err == sql.ErrNoRows {
			_, err = db.Exec("INSERT INTO global_settings (key, value) VALUES (?, ?)", key, value)
			return err
		}
		return nil
	}

	if err := insertDefault("lan_url", "http://localhost"); err != nil {
		return err
	}
	if err := insertDefault("wan_url", ""); err != nil {
		return err
	}
	if err := insertDefault("appstore_cdn_url", ""); err != nil {
		return err
	}
	if err := insertDefault("advanced_mode", "false"); err != nil {
		return err
	}
	if err := insertDefault("alloc_port_start", "55500"); err != nil {
		return err
	}
	if err := insertDefault("alloc_port_end", "56000"); err != nil {
		return err
	}
	if err := insertDefault("allow_auto_alloc_port", "false"); err != nil {
		return err
	}
	if err := insertDefault("image_update_interval_minutes", "120"); err != nil {
		return err
	}
	if err := initEditionSettingsDefaults(insertDefault); err != nil {
		return err
	}
	if err := insertDefault("volume_backup_enabled", "false"); err != nil {
		return err
	}
	if err := insertDefault("volume_backup_image", "offen/docker-volume-backup:latest"); err != nil {
		return err
	}
	if err := insertDefault("volume_backup_env", ""); err != nil {
		return err
	}
	if err := insertDefault("volume_backup_cron_expression", "@daily"); err != nil {
		return err
	}
	if err := insertDefault("volume_backup_volumes", "[]"); err != nil {
		return err
	}
	if err := insertDefault("volume_backup_archive_dir", ""); err != nil {
		return err
	}
	if err := insertDefault("volume_backup_mount_docker_sock", "true"); err != nil {
		return err
	}
	if encoded, err := json.Marshal(defaultNotificationEnabledCategories()); err != nil {
		return err
	} else if err := insertDefault("notification_enabled_categories", string(encoded)); err != nil {
		return err
	}
	return nil
}

func GetSettings() (Settings, error) {
	db := database.GetDB()
	var s Settings

	getValue := func(key string) string {
		var val string
		_ = db.QueryRow("SELECT value FROM global_settings WHERE key = ?", key).Scan(&val)
		return val
	}

	s.LanUrl = getValue("lan_url")
	s.WanUrl = getValue("wan_url")
	// 应用商店 CDN 地址优先级：环境变量 > DB(global_settings) > 内置默认值。
	// 内置默认值保证新部署开箱可用，无需手动配置即能读取应用商店模板。
	const defaultAppStoreCDNURL = "https://github.com/coracoo/tradis_templates"
	s.AppStoreCDNURL = strings.TrimSpace(getValue("appstore_cdn_url"))
	if env := strings.TrimSpace(os.Getenv("APPSTORE_CDN_URL")); env != "" {
		s.AppStoreCDNURL = env
	} else if s.AppStoreCDNURL == "" {
		s.AppStoreCDNURL = defaultAppStoreCDNURL
	}
	if !SupportsAppStoreContentSource {
		s.AppStoreCDNURL = ""
	}
	parseInt := func(v string, def int) int {
		if v == "" {
			return def
		}
		i, err := strconv.Atoi(v)
		if err != nil {
			return def
		}
		return i
	}

	parseBool := func(v string, def bool) bool {
		t := strings.TrimSpace(strings.ToLower(v))
		if t == "" {
			return def
		}
		if t == "1" || t == "true" || t == "yes" || t == "on" {
			return true
		}
		if t == "0" || t == "false" || t == "no" || t == "off" {
			return false
		}
		return def
	}

	s.AdvancedMode = parseBool(getValue("advanced_mode"), false)
	s.AllocPortStart = parseInt(getValue("alloc_port_start"), 55500)
	s.AllocPortEnd = parseInt(getValue("alloc_port_end"), 56000)
	s.AllowAutoAllocPort = parseBool(getValue("allow_auto_alloc_port"), false)
	s.ImageUpdateIntervalMinutes = parseInt(getValue("image_update_interval_minutes"), 120)
	loadEditionSettings(&s, getValue, parseInt, parseBool)
	s.VolumeBackupEnabled = parseBool(getValue("volume_backup_enabled"), false)
	s.VolumeBackupImage = strings.TrimSpace(getValue("volume_backup_image"))
	if s.VolumeBackupImage == "" {
		s.VolumeBackupImage = "offen/docker-volume-backup:latest"
	}
	backupEnv, backupEnvErr := GetVolumeBackupEnv()
	s.VolumeBackupEnv = ""
	s.VolumeBackupEnvSet = backupEnvErr == nil && strings.TrimSpace(backupEnv) != ""
	s.VolumeBackupEnvStored = secrets.IsSealed(strings.TrimSpace(getValue("volume_backup_env")))
	s.VolumeBackupCronExpression = strings.TrimSpace(getValue("volume_backup_cron_expression"))
	if s.VolumeBackupCronExpression == "" {
		s.VolumeBackupCronExpression = "@daily"
	}
	s.VolumeBackupVolumes = parseStringSlice(getValue("volume_backup_volumes"))
	s.VolumeBackupArchiveDir = strings.TrimSpace(getValue("volume_backup_archive_dir"))
	s.VolumeBackupMountDockerSock = parseBool(getValue("volume_backup_mount_docker_sock"), true)
	notificationCategoriesRaw := getValue("notification_enabled_categories")
	if strings.TrimSpace(notificationCategoriesRaw) == "" {
		s.NotificationEnabledCategories = defaultNotificationEnabledCategories()
	} else {
		s.NotificationEnabledCategories = parseStringSlice(notificationCategoriesRaw)
	}

	return s, nil
}

// GetDataDir 获取数据目录
func GetDataDir() string {
	if configured := strings.TrimSpace(os.Getenv("TRADIS_DATA_DIR")); configured != "" {
		return filepath.Clean(configured)
	}
	cwd, err := os.Getwd()
	if err != nil {
		if exe, eerr := os.Executable(); eerr == nil {
			return filepath.Join(filepath.Dir(exe), "data")
		}
		return filepath.Join(".", "data")
	}
	return filepath.Join(cwd, "data")
}

// GetProjectRoot 获取项目根目录
func GetProjectRoot() string {
	parent := filepath.Dir(GetDataDir())
	return filepath.Join(parent, "project")
}

func GetHostProjectRoot() string {
	v := strings.TrimSpace(os.Getenv("PROJECT_ROOT"))
	v = strings.TrimRight(v, "/")
	return v
}

// GetAppStoreBasePath 获取应用商店基础路径
func GetAppStoreBasePath() string {
	return GetDataDir()
}

// GetLanUrl 获取局域网地址
func GetLanUrl() string {
	s, _ := GetSettings()
	return s.LanUrl
}

// GetWanUrl 获取外网地址
func GetWanUrl() string {
	s, _ := GetSettings()
	return s.WanUrl
}

func UpdateSettings(s Settings) error {
	db := database.GetDB()

	logging.Debug(
		"settings update requested",
		"has_lan_url", strings.TrimSpace(s.LanUrl) != "",
		"has_wan_url", strings.TrimSpace(s.WanUrl) != "",
		"advanced_mode", s.AdvancedMode,
		"port_range_start", s.AllocPortStart,
		"port_range_end", s.AllocPortEnd,
		"auto_port", s.AllowAutoAllocPort,
		"image_update_interval_minutes", s.ImageUpdateIntervalMinutes,
		"volume_backup_enabled", s.VolumeBackupEnabled,
	)

	// Helper to update
	update := func(key, value string) error {
		_, err := db.Exec("INSERT OR REPLACE INTO global_settings (key, value) VALUES (?, ?)", key, value)
		return err
	}

	if err := update("lan_url", s.LanUrl); err != nil {
		return err
	}
	if err := update("wan_url", s.WanUrl); err != nil {
		return err
	}
	if err := update("appstore_cdn_url", strings.TrimSpace(s.AppStoreCDNURL)); err != nil {
		return err
	}
	if err := update("advanced_mode", strconv.FormatBool(s.AdvancedMode)); err != nil {
		return err
	}
	if err := update("alloc_port_start", strconv.Itoa(s.AllocPortStart)); err != nil {
		return err
	}
	if err := update("alloc_port_end", strconv.Itoa(s.AllocPortEnd)); err != nil {
		return err
	}
	if err := update("allow_auto_alloc_port", strconv.FormatBool(s.AllowAutoAllocPort)); err != nil {
		return err
	}
	if err := update("image_update_interval_minutes", strconv.Itoa(s.ImageUpdateIntervalMinutes)); err != nil {
		return err
	}
	if err := updateEditionSettings(&s, update); err != nil {
		return err
	}
	if err := update("volume_backup_enabled", strconv.FormatBool(s.VolumeBackupEnabled)); err != nil {
		return err
	}
	if err := update("volume_backup_image", strings.TrimSpace(s.VolumeBackupImage)); err != nil {
		return err
	}
	if err := update("volume_backup_cron_expression", strings.TrimSpace(s.VolumeBackupCronExpression)); err != nil {
		return err
	}
	volumesJSON := "[]"
	if b, err := json.Marshal(normalizeStringSlice(s.VolumeBackupVolumes)); err == nil {
		volumesJSON = string(b)
	}
	if err := update("volume_backup_volumes", volumesJSON); err != nil {
		return err
	}
	if err := update("volume_backup_archive_dir", strings.TrimSpace(s.VolumeBackupArchiveDir)); err != nil {
		return err
	}
	if err := update("volume_backup_mount_docker_sock", strconv.FormatBool(s.VolumeBackupMountDockerSock)); err != nil {
		return err
	}
	notificationCategoriesJSON := "[]"
	if b, err := json.Marshal(normalizeStringSlice(s.NotificationEnabledCategories)); err == nil {
		notificationCategoriesJSON = string(b)
	}
	if err := update("notification_enabled_categories", notificationCategoriesJSON); err != nil {
		return err
	}

	return nil
}

func normalizeStringSlice(list []string) []string {
	out := make([]string, 0, len(list))
	seen := make(map[string]struct{}, len(list))
	for _, v := range list {
		s := strings.TrimSpace(v)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

func parseStringSlice(raw string) []string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return nil
	}
	var list []string
	if json.Unmarshal([]byte(s), &list) == nil {
		return normalizeStringSlice(list)
	}
	return normalizeStringSlice(strings.Split(s, ","))
}

func GetValue(key string) (string, error) {
	return getRawValue(key)
}

func SetValue(key, value string) error {
	if isSensitiveSettingKey(key) {
		return SetSensitiveValue(key, value)
	}
	return setRawValue(key, value)
}

// SetValues atomically updates a group of settings after sealing any
// recognised sensitive values. Callers never observe a partially updated set.
func SetValues(values map[string]string) error {
	prepared := make(map[string]string, len(values))
	for key, value := range values {
		key = strings.TrimSpace(key)
		if isSensitiveSettingKey(key) && strings.TrimSpace(value) != "" {
			sealed, err := secrets.Seal(value, sensitiveSettingContext(key))
			if err != nil {
				return err
			}
			value = sealed
		}
		prepared[key] = value
	}
	tx, err := database.GetDB().Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for key, value := range prepared {
		if _, err := tx.Exec("INSERT OR REPLACE INTO global_settings (key, value) VALUES (?, ?)", key, value); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// SetSensitiveValue seals a recognised setting before it reaches SQLite. An
// empty value clears the stored secret and is safe to persist as an empty row.
func SetSensitiveValue(key, value string) error {
	key = strings.TrimSpace(key)
	if !isSensitiveSettingKey(key) {
		return ErrUnsupportedSensitiveSetting
	}
	if strings.TrimSpace(value) == "" {
		return setRawValue(key, "")
	}
	sealed, err := secrets.Seal(value, sensitiveSettingContext(key))
	if err != nil {
		return err
	}
	return setRawValue(key, sealed)
}

// GetSensitiveValue opens a recognised setting. Legacy plaintext values are
// migrated in place when a master key is available; without it callers must
// ask the user to enter the value again rather than silently using plaintext.
func GetSensitiveValue(key string) (string, error) {
	key = strings.TrimSpace(key)
	if !isSensitiveSettingKey(key) {
		return "", ErrUnsupportedSensitiveSetting
	}
	raw, err := getRawValue(key)
	if err != nil || strings.TrimSpace(raw) == "" {
		return raw, err
	}
	if !secrets.IsSealed(raw) {
		if !secrets.Available() {
			return "", ErrSensitiveValueRequiresReentry
		}
		if err := SetSensitiveValue(key, raw); err != nil {
			return "", err
		}
		return raw, nil
	}
	value, err := secrets.Open(raw, sensitiveSettingContext(key))
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrSensitiveValueRequiresReentry, err)
	}
	return value, nil
}

func GetVolumeBackupEnv() (string, error) {
	return GetSensitiveValue("volume_backup_env")
}

func isSensitiveSettingKey(key string) bool {
	key = strings.TrimSpace(key)
	if _, ok := sensitiveSettingKeys[key]; ok {
		return true
	}
	return isEditionSensitiveSettingKey(key)
}

func sensitiveSettingContext(key string) string {
	return "global_settings:" + strings.TrimSpace(key)
}

func getRawValue(key string) (string, error) {
	db := database.GetDB()
	var val string
	err := db.QueryRow("SELECT value FROM global_settings WHERE key = ?", key).Scan(&val)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return val, err
}

func setRawValue(key, value string) error {
	db := database.GetDB()
	_, err := db.Exec("INSERT OR REPLACE INTO global_settings (key, value) VALUES (?, ?)", key, value)
	return err
}
