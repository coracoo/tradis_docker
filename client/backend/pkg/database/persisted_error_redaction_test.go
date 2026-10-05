package database

import (
	"strings"
	"testing"
	"time"
)

const persistedRemoteError = `request to https://official.example.test/api?token=secret failed`

func assertNoRemoteURL(t *testing.T, label, value string) {
	t.Helper()
	if strings.Contains(value, "http://") ||
		strings.Contains(value, "https://") ||
		strings.Contains(value, "example.test") {
		t.Fatalf("%s leaked remote URL: %q", label, value)
	}
}

func TestNotificationDeliveryRedactsRemoteURLs(t *testing.T) {
	setupNotificationTestDB(t)

	delivery, inserted, err := CreateNotificationDelivery(NotificationDelivery{
		ChannelID:     "channel-redaction",
		DedupeKey:     "delivery-redaction",
		EventCategory: "system",
		EventType:     "test",
		Status:        "failed",
		LastError:     persistedRemoteError,
	})
	if err != nil || !inserted {
		t.Fatalf("CreateNotificationDelivery() = %#v, %v, %v", delivery, inserted, err)
	}

	items, err := ListNotificationDeliveries(delivery.ChannelID, 10)
	if err != nil || len(items) != 1 {
		t.Fatalf("ListNotificationDeliveries() = %#v, %v", items, err)
	}
	assertNoRemoteURL(t, "notification delivery create error", items[0].LastError)

	delivery.LastError = `retry https://retry.example.test/hook failed`
	delivery.UpdatedAt = time.Now().Format("2006-01-02 15:04:05")
	if err := UpdateNotificationDelivery(delivery); err != nil {
		t.Fatalf("UpdateNotificationDelivery() error = %v", err)
	}
	items, err = ListNotificationDeliveries(delivery.ChannelID, 10)
	if err != nil || len(items) != 1 {
		t.Fatalf("ListNotificationDeliveries() after update = %#v, %v", items, err)
	}
	assertNoRemoteURL(t, "notification delivery update error", items[0].LastError)
}

func TestImageRemoteDigestFailureRedactsRemoteURLs(t *testing.T) {
	setupNotificationTestDB(t)

	status, err := RecordImageRemoteDigestFailure("example/image:latest", persistedRemoteError, time.Hour, 2*time.Hour, 3)
	if err != nil {
		t.Fatalf("RecordImageRemoteDigestFailure() error = %v", err)
	}
	assertNoRemoteURL(t, "image remote digest error", status.LastError)
}
