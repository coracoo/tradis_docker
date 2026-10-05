package database

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"dockerpanel/backend/pkg/logging"
	"dockerpanel/backend/pkg/secrets"
)

const notificationChannelSecretContextPrefix = "notification_channel:"

var notificationCategories = []string{"deploy_task", "git_task", "navigation_task", "volume_backup_task", "app_protection_task", "system"}

type NotificationChannel struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Type       string            `json:"type"`
	Enabled    bool              `json:"enabled"`
	Categories []string          `json:"categories"`
	Config     map[string]string `json:"config"`
	Secrets    map[string]string `json:"-"`
	SecretSet  bool              `json:"secretSet"`
	CreatedAt  string            `json:"createdAt"`
	UpdatedAt  string            `json:"updatedAt"`
}

type NotificationDelivery struct {
	ID             int64  `json:"id"`
	EnvironmentID  string `json:"environmentId"`
	ChannelID      string `json:"channelId"`
	NotificationID int64  `json:"notificationId,omitempty"`
	DedupeKey      string `json:"-"`
	EventCategory  string `json:"eventCategory"`
	EventType      string `json:"eventType"`
	Status         string `json:"status"`
	Attempts       int    `json:"attempts"`
	ResponseCode   int    `json:"responseCode,omitempty"`
	LastError      string `json:"lastError,omitempty"`
	CreatedAt      string `json:"createdAt"`
	UpdatedAt      string `json:"updatedAt"`
}

func createNotificationChannelTables() error {
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS notification_channels (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			type TEXT NOT NULL,
			enabled INTEGER NOT NULL DEFAULT 1,
			categories_json TEXT NOT NULL DEFAULT '[]',
			config_json TEXT NOT NULL DEFAULT '{}',
			secret_json TEXT NOT NULL DEFAULT '',
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		)
	`); err != nil {
		return err
	}
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS notification_deliveries (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			environment_id TEXT NOT NULL DEFAULT 'local',
			channel_id TEXT NOT NULL,
			notification_id INTEGER,
			dedupe_key TEXT NOT NULL,
			event_category TEXT NOT NULL,
			event_type TEXT NOT NULL,
			status TEXT NOT NULL,
			attempts INTEGER NOT NULL DEFAULT 0,
			response_code INTEGER NOT NULL DEFAULT 0,
			last_error TEXT NOT NULL DEFAULT '',
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL,
			UNIQUE(environment_id, channel_id, dedupe_key)
		)
	`); err != nil {
		return err
	}
	if err := ensureTableColumns("notification_deliveries", []columnSpec{
		{Name: "environment_id", AddColumnSQL: "environment_id TEXT DEFAULT 'local'", BackfillSQL: []string{"UPDATE notification_deliveries SET environment_id = 'local' WHERE environment_id IS NULL OR environment_id = ''"}},
	}); err != nil {
		return err
	}
	if err := ensureNotificationDeliveryEnvironmentKey(); err != nil {
		return err
	}
	_, _ = db.Exec(`CREATE INDEX IF NOT EXISTS idx_notification_deliveries_channel_created ON notification_deliveries(channel_id, id DESC)`)
	_, _ = db.Exec(`CREATE INDEX IF NOT EXISTS idx_notification_deliveries_environment_channel ON notification_deliveries(environment_id, channel_id, id DESC)`)
	return nil
}

func CreateNotificationChannel(channel NotificationChannel) (NotificationChannel, error) {
	if db == nil {
		return NotificationChannel{}, fmt.Errorf("数据库连接未初始化")
	}
	channel, err := normalizeNotificationChannel(channel)
	if err != nil {
		return NotificationChannel{}, err
	}
	if strings.TrimSpace(channel.ID) == "" {
		channel.ID = fmt.Sprintf("notify-%d", time.Now().UnixNano())
	}
	configJSON, err := json.Marshal(channel.Config)
	if err != nil {
		return NotificationChannel{}, fmt.Errorf("通知通道配置无效: %w", err)
	}
	secretsJSON, err := sealNotificationChannelSecrets(channel.ID, channel.Secrets)
	if err != nil {
		return NotificationChannel{}, err
	}
	categoriesJSON, _ := json.Marshal(channel.Categories)
	now := time.Now().In(chinaLocation).Format(time.RFC3339)
	_, err = db.Exec(`INSERT INTO notification_channels (id, name, type, enabled, categories_json, config_json, secret_json, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		channel.ID, channel.Name, channel.Type, boolToInt(channel.Enabled), string(categoriesJSON), string(configJSON), secretsJSON, now, now)
	if err != nil {
		return NotificationChannel{}, err
	}
	return GetNotificationChannel(channel.ID)
}

// UpdateNotificationChannel preserves encrypted secrets unless replaceSecrets
// is true. This prevents edit forms from needing to read secret values back.
func UpdateNotificationChannel(channel NotificationChannel, replaceSecrets bool) (NotificationChannel, error) {
	current, err := getNotificationChannel(channel.ID, true)
	if err != nil {
		return NotificationChannel{}, err
	}
	if !replaceSecrets {
		channel.Secrets = current.Secrets
	}
	channel, err = normalizeNotificationChannel(channel)
	if err != nil {
		return NotificationChannel{}, err
	}
	configJSON, err := json.Marshal(channel.Config)
	if err != nil {
		return NotificationChannel{}, fmt.Errorf("通知通道配置无效: %w", err)
	}
	secretsJSON, err := sealNotificationChannelSecrets(channel.ID, channel.Secrets)
	if err != nil {
		return NotificationChannel{}, err
	}
	categoriesJSON, _ := json.Marshal(channel.Categories)
	result, err := db.Exec(`UPDATE notification_channels SET name = ?, type = ?, enabled = ?, categories_json = ?, config_json = ?, secret_json = ?, updated_at = ? WHERE id = ?`,
		channel.Name, channel.Type, boolToInt(channel.Enabled), string(categoriesJSON), string(configJSON), secretsJSON, time.Now().In(chinaLocation).Format(time.RFC3339), channel.ID)
	if err != nil {
		return NotificationChannel{}, err
	}
	changed, _ := result.RowsAffected()
	if changed == 0 {
		return NotificationChannel{}, fmt.Errorf("通知通道不存在")
	}
	return GetNotificationChannel(channel.ID)
}

func DeleteNotificationChannel(id string) error {
	result, err := db.Exec(`DELETE FROM notification_channels WHERE id = ?`, strings.TrimSpace(id))
	if err != nil {
		return err
	}
	changed, _ := result.RowsAffected()
	if changed == 0 {
		return fmt.Errorf("通知通道不存在")
	}
	return nil
}

func ListNotificationChannels() ([]NotificationChannel, error) {
	rows, err := db.Query(`SELECT id, name, type, enabled, categories_json, config_json, secret_json, created_at, updated_at FROM notification_channels ORDER BY updated_at DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	channels := []NotificationChannel{}
	for rows.Next() {
		channel, err := scanNotificationChannel(rows, false)
		if err != nil {
			return nil, err
		}
		channels = append(channels, channel)
	}
	return channels, rows.Err()
}

func GetNotificationChannel(id string) (NotificationChannel, error) {
	return getNotificationChannel(id, false)
}

// GetNotificationChannelForDelivery is intentionally for backend delivery
// code only. The Secrets field is excluded from JSON serialization.
func GetNotificationChannelForDelivery(id string) (NotificationChannel, error) {
	return getNotificationChannel(id, true)
}

func getNotificationChannel(id string, includeSecrets bool) (NotificationChannel, error) {
	row := db.QueryRow(`SELECT id, name, type, enabled, categories_json, config_json, secret_json, created_at, updated_at FROM notification_channels WHERE id = ?`, strings.TrimSpace(id))
	channel, err := scanNotificationChannel(row, includeSecrets)
	if err == sql.ErrNoRows {
		return NotificationChannel{}, fmt.Errorf("通知通道不存在")
	}
	return channel, err
}

func ListNotificationChannelsForDelivery(category string) ([]NotificationChannel, error) {
	channels, err := ListNotificationChannels()
	if err != nil {
		return nil, err
	}
	result := make([]NotificationChannel, 0, len(channels))
	for _, channel := range channels {
		if !channel.Enabled || !notificationChannelSupportsCategory(channel, category) {
			continue
		}
		loaded, err := GetNotificationChannelForDelivery(channel.ID)
		if err != nil {
			return nil, err
		}
		result = append(result, loaded)
	}
	return result, nil
}

func CreateNotificationDelivery(delivery NotificationDelivery) (NotificationDelivery, bool, error) {
	if strings.TrimSpace(delivery.ChannelID) == "" || strings.TrimSpace(delivery.DedupeKey) == "" {
		return NotificationDelivery{}, false, fmt.Errorf("投递记录缺少通道或去重键")
	}
	var err error
	delivery.EnvironmentID, err = normalizeNotificationEnvironmentID(delivery.EnvironmentID)
	if err != nil {
		return NotificationDelivery{}, false, err
	}
	now := time.Now().In(chinaLocation).Format(time.RFC3339)
	if delivery.CreatedAt == "" {
		delivery.CreatedAt = now
	}
	if delivery.UpdatedAt == "" {
		delivery.UpdatedAt = now
	}
	if delivery.Status == "" {
		delivery.Status = "pending"
	}
	result, err := db.Exec(`INSERT INTO notification_deliveries (environment_id, channel_id, notification_id, dedupe_key, event_category, event_type, status, attempts, response_code, last_error, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		delivery.EnvironmentID, delivery.ChannelID, delivery.NotificationID, delivery.DedupeKey, delivery.EventCategory, delivery.EventType, delivery.Status, delivery.Attempts, delivery.ResponseCode, logging.RedactText(secrets.RedactString(delivery.LastError)), delivery.CreatedAt, delivery.UpdatedAt)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			existing, getErr := getNotificationDeliveryByDedupe(delivery.EnvironmentID, delivery.ChannelID, delivery.DedupeKey)
			return existing, false, getErr
		}
		return NotificationDelivery{}, false, err
	}
	delivery.ID, _ = result.LastInsertId()
	return delivery, true, nil
}

func UpdateNotificationDelivery(delivery NotificationDelivery) error {
	var err error
	delivery.EnvironmentID, err = normalizeNotificationEnvironmentID(delivery.EnvironmentID)
	if err != nil {
		return err
	}
	_, err = db.Exec(`UPDATE notification_deliveries SET status = ?, attempts = ?, response_code = ?, last_error = ?, updated_at = ? WHERE id = ? AND environment_id = ?`,
		delivery.Status, delivery.Attempts, delivery.ResponseCode, logging.RedactText(secrets.RedactString(delivery.LastError)), delivery.UpdatedAt, delivery.ID, delivery.EnvironmentID)
	return err
}

func ListNotificationDeliveries(channelID string, limit int) ([]NotificationDelivery, error) {
	return ListNotificationDeliveriesInEnvironment(LocalEnvironmentID, channelID, limit)
}

func ListNotificationDeliveriesInEnvironment(environmentID string, channelID string, limit int) ([]NotificationDelivery, error) {
	var err error
	environmentID, err = normalizeNotificationEnvironmentID(environmentID)
	if err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	rows, err := db.Query(`SELECT id, environment_id, channel_id, COALESCE(notification_id, 0), dedupe_key, event_category, event_type, status, attempts, response_code, last_error, created_at, updated_at FROM notification_deliveries WHERE environment_id = ? AND channel_id = ? ORDER BY id DESC LIMIT ?`, environmentID, strings.TrimSpace(channelID), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []NotificationDelivery{}
	for rows.Next() {
		var delivery NotificationDelivery
		if err := rows.Scan(&delivery.ID, &delivery.EnvironmentID, &delivery.ChannelID, &delivery.NotificationID, &delivery.DedupeKey, &delivery.EventCategory, &delivery.EventType, &delivery.Status, &delivery.Attempts, &delivery.ResponseCode, &delivery.LastError, &delivery.CreatedAt, &delivery.UpdatedAt); err != nil {
			return nil, err
		}
		result = append(result, delivery)
	}
	return result, rows.Err()
}

func getNotificationDeliveryByDedupe(environmentID, channelID, dedupeKey string) (NotificationDelivery, error) {
	var delivery NotificationDelivery
	err := db.QueryRow(`SELECT id, environment_id, channel_id, COALESCE(notification_id, 0), dedupe_key, event_category, event_type, status, attempts, response_code, last_error, created_at, updated_at FROM notification_deliveries WHERE environment_id = ? AND channel_id = ? AND dedupe_key = ?`, environmentID, channelID, dedupeKey).Scan(&delivery.ID, &delivery.EnvironmentID, &delivery.ChannelID, &delivery.NotificationID, &delivery.DedupeKey, &delivery.EventCategory, &delivery.EventType, &delivery.Status, &delivery.Attempts, &delivery.ResponseCode, &delivery.LastError, &delivery.CreatedAt, &delivery.UpdatedAt)
	return delivery, err
}

func ensureNotificationDeliveryEnvironmentKey() error {
	rows, err := db.Query(`PRAGMA index_list(notification_deliveries)`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var sequence, unique, partial int
		var name, origin string
		if err := rows.Scan(&sequence, &name, &unique, &origin, &partial); err != nil {
			return err
		}
		if unique != 1 {
			continue
		}
		columns, err := indexColumns(name)
		if err != nil {
			return err
		}
		if len(columns) == 3 && columns[0] == "environment_id" && columns[1] == "channel_id" && columns[2] == "dedupe_key" {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`
		CREATE TABLE notification_deliveries_next (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			environment_id TEXT NOT NULL DEFAULT 'local',
			channel_id TEXT NOT NULL,
			notification_id INTEGER,
			dedupe_key TEXT NOT NULL,
			event_category TEXT NOT NULL,
			event_type TEXT NOT NULL,
			status TEXT NOT NULL,
			attempts INTEGER NOT NULL DEFAULT 0,
			response_code INTEGER NOT NULL DEFAULT 0,
			last_error TEXT NOT NULL DEFAULT '',
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL,
			UNIQUE(environment_id, channel_id, dedupe_key)
		)
	`); err != nil {
		return err
	}
	if _, err := tx.Exec(`
		INSERT INTO notification_deliveries_next (
			id, environment_id, channel_id, notification_id, dedupe_key, event_category, event_type,
			status, attempts, response_code, last_error, created_at, updated_at
		)
		SELECT id, COALESCE(NULLIF(environment_id, ''), 'local'), channel_id, notification_id, dedupe_key,
		       event_category, event_type, status, attempts, response_code, last_error, created_at, updated_at
		FROM notification_deliveries
	`); err != nil {
		return err
	}
	if _, err := tx.Exec(`DROP TABLE notification_deliveries`); err != nil {
		return err
	}
	if _, err := tx.Exec(`ALTER TABLE notification_deliveries_next RENAME TO notification_deliveries`); err != nil {
		return err
	}
	return tx.Commit()
}

func indexColumns(indexName string) ([]string, error) {
	if !isValidIdentifier(indexName) {
		return nil, fmt.Errorf("非法索引名: %s", indexName)
	}
	rows, err := db.Query(`PRAGMA index_info(` + indexName + `)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	columns := []string{}
	for rows.Next() {
		var sequence, cid int
		var name string
		if err := rows.Scan(&sequence, &cid, &name); err != nil {
			return nil, err
		}
		columns = append(columns, name)
	}
	return columns, rows.Err()
}

type notificationChannelScanner interface {
	Scan(...any) error
}

func scanNotificationChannel(scanner notificationChannelScanner, includeSecrets bool) (NotificationChannel, error) {
	var channel NotificationChannel
	var enabled int
	var categoriesJSON, configJSON, secretJSON string
	if err := scanner.Scan(&channel.ID, &channel.Name, &channel.Type, &enabled, &categoriesJSON, &configJSON, &secretJSON, &channel.CreatedAt, &channel.UpdatedAt); err != nil {
		return channel, err
	}
	channel.Enabled = enabled == 1
	channel.Categories = decodeNotificationStringSlice(categoriesJSON)
	channel.Config = decodeNotificationStringMap(configJSON)
	channel.SecretSet = strings.TrimSpace(secretJSON) != ""
	if includeSecrets && channel.SecretSet {
		opened, err := secrets.Open(secretJSON, notificationChannelSecretContext(channel.ID))
		if err != nil {
			return channel, fmt.Errorf("读取通知通道密钥失败，请重新配置: %w", err)
		}
		channel.Secrets = decodeNotificationStringMap(opened)
	}
	return channel, nil
}

func normalizeNotificationChannel(channel NotificationChannel) (NotificationChannel, error) {
	channel.ID = strings.TrimSpace(channel.ID)
	channel.Name = strings.TrimSpace(channel.Name)
	channel.Type = strings.ToLower(strings.TrimSpace(channel.Type))
	if channel.Name == "" {
		return channel, fmt.Errorf("通知通道名称不能为空")
	}
	switch channel.Type {
	case "webhook", "wecom", "ntfy", "gotify", "bark", "pushplus":
	default:
		return channel, fmt.Errorf("不支持的通知通道类型")
	}
	channel.Categories = normalizeNotificationChannelCategories(channel.Categories)
	if len(channel.Categories) == 0 {
		channel.Categories = append([]string(nil), notificationCategories...)
	}
	if channel.Config == nil {
		channel.Config = map[string]string{}
	}
	if err := validateNotificationChannelVisibleConfig(channel.Config); err != nil {
		return channel, err
	}
	if channel.Secrets == nil {
		channel.Secrets = map[string]string{}
	}
	return channel, nil
}

func validateNotificationChannelVisibleConfig(config map[string]string) error {
	for _, key := range []string{"url", "server"} {
		raw := strings.TrimSpace(config[key])
		if raw == "" {
			continue
		}
		parsed, err := url.Parse(raw)
		if err != nil {
			return fmt.Errorf("通知通道地址无效")
		}
		if parsed.User != nil {
			return fmt.Errorf("通知通道地址不能包含用户名或密码，请使用加密密钥字段")
		}
		for queryKey := range parsed.Query() {
			if secrets.IsSensitiveKey(queryKey) {
				return fmt.Errorf("通知通道地址不能包含敏感查询参数，请使用加密密钥字段")
			}
		}
	}
	return nil
}

func normalizeNotificationChannelCategories(values []string) []string {
	seen := map[string]bool{}
	result := []string{}
	for _, value := range values {
		category := normalizeNotificationCategory(value)
		if !seen[category] {
			seen[category] = true
			result = append(result, category)
		}
	}
	return result
}

func notificationChannelSupportsCategory(channel NotificationChannel, category string) bool {
	for _, value := range channel.Categories {
		if value == normalizeNotificationCategory(category) {
			return true
		}
	}
	return false
}

func sealNotificationChannelSecrets(id string, values map[string]string) (string, error) {
	clean := map[string]string{}
	for key, value := range values {
		if strings.TrimSpace(key) != "" && strings.TrimSpace(value) != "" {
			clean[strings.TrimSpace(key)] = strings.TrimSpace(value)
		}
	}
	if len(clean) == 0 {
		return "", nil
	}
	payload, err := json.Marshal(clean)
	if err != nil {
		return "", err
	}
	sealed, err := secrets.Seal(string(payload), notificationChannelSecretContext(id))
	if err != nil {
		return "", fmt.Errorf("保存通知通道密钥失败: %w", err)
	}
	return sealed, nil
}

func notificationChannelSecretContext(id string) string {
	return notificationChannelSecretContextPrefix + strings.TrimSpace(id)
}

func decodeNotificationStringMap(raw string) map[string]string {
	values := map[string]string{}
	_ = json.Unmarshal([]byte(raw), &values)
	return values
}

func decodeNotificationStringSlice(raw string) []string {
	var values []string
	_ = json.Unmarshal([]byte(raw), &values)
	return normalizeNotificationChannelCategories(values)
}
