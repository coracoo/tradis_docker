package database

import (
	"path/filepath"
	"strings"
	"testing"
)

func setupNotificationTestDB(t *testing.T) {
	t.Helper()
	if db != nil {
		_ = db.Close()
		db = nil
	}
	if err := InitDB(filepath.Join(t.TempDir(), "data.db")); err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	t.Cleanup(func() {
		if db != nil {
			_ = db.Close()
			db = nil
		}
	})
}

func setNotificationCategoriesForTest(t *testing.T, raw string) {
	t.Helper()
	if _, err := db.Exec(`INSERT OR REPLACE INTO global_settings (key, value) VALUES ('notification_enabled_categories', ?)`, raw); err != nil {
		t.Fatalf("set notification categories failed: %v", err)
	}
}

func TestSaveNotificationFiltersByCategory(t *testing.T) {
	setupNotificationTestDB(t)
	setNotificationCategoriesForTest(t, `["deploy_task"]`)

	allowed := &Notification{Type: "success", Category: "deploy_task", Message: "应用部署完成"}
	if err := SaveNotification(allowed); err != nil {
		t.Fatalf("SaveNotification allowed failed: %v", err)
	}
	if allowed.ID == 0 {
		t.Fatalf("allowed notification was not inserted")
	}

	filtered := &Notification{Type: "info", Category: "git_task", Message: "GitHub 应用同步完成"}
	if err := SaveNotification(filtered); err != nil {
		t.Fatalf("SaveNotification filtered failed: %v", err)
	}
	if filtered.ID != 0 {
		t.Fatalf("filtered notification should not be inserted, got id %d", filtered.ID)
	}

	list, err := GetNotifications(10)
	if err != nil {
		t.Fatalf("GetNotifications failed: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(list))
	}
	if list[0].Category != "deploy_task" {
		t.Fatalf("expected deploy_task category, got %q", list[0].Category)
	}
}

func TestSaveNotificationSkipsAllCategoriesWhenPreferencesAreEmpty(t *testing.T) {
	setupNotificationTestDB(t)
	setNotificationCategoriesForTest(t, `[]`)

	n := &Notification{Type: "warning", Category: "deploy_task", Message: "部署需要处理"}
	if err := SaveNotification(n); err != nil {
		t.Fatalf("SaveNotification failed: %v", err)
	}
	if n.ID != 0 {
		t.Fatalf("notification should not be inserted when all categories are disabled, got id %d", n.ID)
	}

	list, err := GetNotifications(10)
	if err != nil {
		t.Fatalf("GetNotifications failed: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected no notifications, got %#v", list)
	}
}

func TestSaveNotificationDefaultsEmptyCategoryToSystem(t *testing.T) {
	setupNotificationTestDB(t)
	setNotificationCategoriesForTest(t, `["system"]`)

	n := &Notification{Type: "info", Message: "普通系统消息"}
	if err := SaveNotification(n); err != nil {
		t.Fatalf("SaveNotification failed: %v", err)
	}
	if n.ID == 0 {
		t.Fatalf("system notification was not inserted")
	}

	list, err := GetNotifications(10)
	if err != nil {
		t.Fatalf("GetNotifications failed: %v", err)
	}
	if len(list) != 1 || list[0].Category != "system" {
		t.Fatalf("expected one system notification, got %#v", list)
	}
}

func TestSaveNotificationRedactsRemoteURL(t *testing.T) {
	setupNotificationTestDB(t)
	setNotificationCategoriesForTest(t, `["system"]`)

	n := &Notification{
		Type:     "error",
		Category: "system",
		Message:  `request to https://official.example.test/api?token=secret failed`,
	}
	if err := SaveNotification(n); err != nil {
		t.Fatalf("SaveNotification failed: %v", err)
	}

	list, err := GetNotifications(10)
	if err != nil || len(list) != 1 {
		t.Fatalf("GetNotifications = %#v, %v", list, err)
	}
	if strings.Contains(list[0].Message, "https://") || strings.Contains(list[0].Message, "example.test") {
		t.Fatalf("notification leaked remote URL: %s", list[0].Message)
	}
}

func TestSaveNotificationRedactsSensitiveAssignments(t *testing.T) {
	setupNotificationTestDB(t)
	setNotificationCategoriesForTest(t, `["system"]`)

	n := &Notification{Type: "error", Category: "system", Message: "同步失败: API_TOKEN=notification-secret"}
	if err := SaveNotification(n); err != nil {
		t.Fatalf("SaveNotification error = %v", err)
	}
	items, err := GetNotifications(10)
	if err != nil || len(items) != 1 {
		t.Fatalf("GetNotifications = %#v, %v", items, err)
	}
	if strings.Contains(items[0].Message, "notification-secret") || !strings.Contains(items[0].Message, "[REDACTED]") {
		t.Fatalf("notification leaked a secret: %#v", items[0])
	}
}

func TestBackfillNotificationCategories(t *testing.T) {
	setupNotificationTestDB(t)

	messages := []string{
		"应用部署成功：nginx",
		"GitHub 应用自动同步完成：保存 1 个项目",
		"AI 导航识别完成：补全 2 个项目",
		"卷备份任务完成",
		"应用备份完成：nginx",
		"普通系统消息",
	}
	for _, message := range messages {
		if _, err := db.Exec(`INSERT INTO notifications (type, category, message, read, hidden, created_at) VALUES ('info', 'system', ?, 0, 0, CURRENT_TIMESTAMP)`, message); err != nil {
			t.Fatalf("insert old notification failed: %v", err)
		}
	}

	if err := backfillNotificationCategories(); err != nil {
		t.Fatalf("backfillNotificationCategories failed: %v", err)
	}

	rows, err := db.Query(`SELECT message, category FROM notifications ORDER BY id ASC`)
	if err != nil {
		t.Fatalf("query notifications failed: %v", err)
	}
	defer rows.Close()

	got := map[string]string{}
	for rows.Next() {
		var message string
		var category string
		if err := rows.Scan(&message, &category); err != nil {
			t.Fatalf("scan notification failed: %v", err)
		}
		got[message] = category
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows failed: %v", err)
	}

	want := map[string]string{
		"应用部署成功：nginx":             "deploy_task",
		"GitHub 应用自动同步完成：保存 1 个项目": "git_task",
		"AI 导航识别完成：补全 2 个项目":       "navigation_task",
		"卷备份任务完成":                  "volume_backup_task",
		"应用备份完成：nginx":             "app_protection_task",
		"普通系统消息":                   "system",
	}
	for message, category := range want {
		if got[message] != category {
			t.Fatalf("message %q expected category %q, got %q", message, category, got[message])
		}
	}
}

func TestGetNotificationsBeforeIDDoesNotShiftWhenNewRowsArrive(t *testing.T) {
	setupNotificationTestDB(t)

	for i := 1; i <= 5; i++ {
		if _, err := db.Exec(`INSERT INTO notifications (type, category, message, read, hidden, created_at) VALUES ('info', 'system', ?, 0, 0, CURRENT_TIMESTAMP)`, i); err != nil {
			t.Fatalf("insert notification %d failed: %v", i, err)
		}
	}

	first, err := GetNotificationsBeforeID(2, 0)
	if err != nil {
		t.Fatalf("GetNotificationsBeforeID first page failed: %v", err)
	}
	if len(first) != 2 || first[0].ID != 5 || first[1].ID != 4 {
		t.Fatalf("unexpected first page: %#v", first)
	}

	if _, err := db.Exec(`INSERT INTO notifications (type, category, message, read, hidden, created_at) VALUES ('info', 'system', 'new', 0, 0, CURRENT_TIMESTAMP)`); err != nil {
		t.Fatalf("insert new notification failed: %v", err)
	}

	second, err := GetNotificationsBeforeID(2, first[len(first)-1].ID)
	if err != nil {
		t.Fatalf("GetNotificationsBeforeID second page failed: %v", err)
	}
	if len(second) != 2 || second[0].ID != 3 || second[1].ID != 2 {
		t.Fatalf("unexpected second page after insert: %#v", second)
	}
}
