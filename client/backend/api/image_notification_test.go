package api

import (
	"errors"
	"path/filepath"
	"testing"

	"dockerpanel/backend/pkg/database"
	"dockerpanel/backend/pkg/settings"
)

func TestFormatImageUpdateMessageUsesConciseActionResult(t *testing.T) {
	message := formatImageUpdateMessage([]string{"redis:latest", "nginx:latest", "redis:latest"})
	if message != "检测到镜像更新：nginx:latest、redis:latest" {
		t.Fatalf("message = %q", message)
	}
}

func initImageNotificationTestDB(t *testing.T) {
	t.Helper()
	_ = database.Close()
	if err := database.InitDB(filepath.Join(t.TempDir(), "image-notification.db")); err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	t.Setenv("TRADIS_DATA_DIR", t.TempDir())
}

func seedImageUpdateRecord(t *testing.T, repoTag string) database.ImageUpdate {
	t.Helper()
	record := database.ImageUpdate{
		RepoTag:      repoTag,
		ImageID:      "sha256:local",
		LocalDigest:  "sha256:local",
		RemoteDigest: "sha256:remote",
	}
	if err := database.SaveImageUpdate(&record); err != nil {
		t.Fatalf("SaveImageUpdate(%q): %v", repoTag, err)
	}
	return record
}

func unnotifiedImageUpdates(t *testing.T) []database.ImageUpdate {
	t.Helper()
	items, err := database.GetUnnotifiedImageUpdates()
	if err != nil {
		t.Fatalf("GetUnnotifiedImageUpdates: %v", err)
	}
	return items
}

func notificationCount(t *testing.T) int {
	t.Helper()
	items, err := database.GetNotifications(50)
	if err != nil {
		t.Fatalf("GetNotifications: %v", err)
	}
	return len(items)
}

func TestDeliverImageUpdateNotificationsSavesAndMarks(t *testing.T) {
	initImageNotificationTestDB(t)
	seedImageUpdateRecord(t, "nginx:latest")

	deliverImageUpdateNotifications(unnotifiedImageUpdates(t))

	if got := notificationCount(t); got != 1 {
		t.Fatalf("notifications = %d, want 1", got)
	}
	items, err := database.GetAllImageUpdates()
	if err != nil {
		t.Fatalf("GetAllImageUpdates: %v", err)
	}
	if len(items) != 1 || !items[0].Notified {
		t.Fatalf("records after deliver = %+v, want notified", items)
	}

	// 已 notified 的记录下轮调度不再重复通知（对 db 状态断言）
	deliverImageUpdateNotifications(unnotifiedImageUpdates(t))
	if got := notificationCount(t); got != 1 {
		t.Fatalf("notifications after second deliver = %d, want still 1", got)
	}
}

func TestDeliverImageUpdateNotificationsSaveFailureKeepsUnnotified(t *testing.T) {
	initImageNotificationTestDB(t)
	seedImageUpdateRecord(t, "nginx:latest")

	original := saveImageUpdateNotification
	saveImageUpdateNotification = func(*database.Notification) error {
		return errors.New("disk full")
	}
	t.Cleanup(func() { saveImageUpdateNotification = original })

	deliverImageUpdateNotifications(unnotifiedImageUpdates(t))

	if got := notificationCount(t); got != 0 {
		t.Fatalf("notifications = %d, want 0", got)
	}
	items, err := database.GetAllImageUpdates()
	if err != nil {
		t.Fatalf("GetAllImageUpdates: %v", err)
	}
	if len(items) != 1 || items[0].Notified {
		t.Fatalf("records after failed deliver = %+v, want kept unnotified", items)
	}
}

func TestDeliverImageUpdateNotificationsCategoryDisabledSkips(t *testing.T) {
	initImageNotificationTestDB(t)
	if err := settings.SetValue("notification_enabled_categories", `["deploy_task"]`); err != nil {
		t.Fatalf("SetValue: %v", err)
	}
	seedImageUpdateRecord(t, "nginx:latest")

	deliverImageUpdateNotifications(unnotifiedImageUpdates(t))

	if got := notificationCount(t); got != 0 {
		t.Fatalf("notifications = %d, want 0", got)
	}
	items, err := database.GetAllImageUpdates()
	if err != nil {
		t.Fatalf("GetAllImageUpdates: %v", err)
	}
	if len(items) != 1 || items[0].Notified {
		t.Fatalf("records with category disabled = %+v, want kept unnotified", items)
	}

	// 用户重开分类后下轮补发：记录被标记 notified 且产出一条通知
	if err := settings.SetValue("notification_enabled_categories", `["deploy_task","system"]`); err != nil {
		t.Fatalf("SetValue: %v", err)
	}
	deliverImageUpdateNotifications(unnotifiedImageUpdates(t))
	if got := notificationCount(t); got != 1 {
		t.Fatalf("notifications after re-enable = %d, want 1", got)
	}
	items, err = database.GetAllImageUpdates()
	if err != nil {
		t.Fatalf("GetAllImageUpdates: %v", err)
	}
	if len(items) != 1 || !items[0].Notified {
		t.Fatalf("records after re-enable = %+v, want notified", items)
	}
}

func TestMarkManualImageUpdatesSeenMarksNotified(t *testing.T) {
	initImageNotificationTestDB(t)
	seedImageUpdateRecord(t, "nginx:latest")

	markManualImageUpdatesSeen([]imageUpdateInfo{{RepoTag: "nginx:latest"}})

	items, err := database.GetAllImageUpdates()
	if err != nil {
		t.Fatalf("GetAllImageUpdates: %v", err)
	}
	if len(items) != 1 || !items[0].Notified {
		t.Fatalf("records after manual mark = %+v, want notified", items)
	}

	// 手动检查不会产生通知，调度下轮对已 notified 记录也不重复通知
	if got := notificationCount(t); got != 0 {
		t.Fatalf("notifications = %d, want 0", got)
	}
	deliverImageUpdateNotifications(unnotifiedImageUpdates(t))
	if got := notificationCount(t); got != 0 {
		t.Fatalf("notifications after scheduler deliver = %d, want 0", got)
	}
}
