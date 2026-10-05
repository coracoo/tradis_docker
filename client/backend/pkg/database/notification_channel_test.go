package database

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestNotificationChannelSecretsAreEncryptedAndNeverMarshaled(t *testing.T) {
	setupNotificationTestDB(t)
	t.Setenv("TRADIS_MASTER_KEY", strings.Repeat("01", 32))

	channel, err := CreateNotificationChannel(NotificationChannel{
		ID:         "channel-1",
		Name:       "家庭 ntfy",
		Type:       "ntfy",
		Enabled:    true,
		Categories: []string{"deploy_task"},
		Config:     map[string]string{"server": "https://ntfy.example", "topic": "tradis"},
		Secrets:    map[string]string{"token": "secret-token"},
	})
	if err != nil {
		t.Fatalf("CreateNotificationChannel() error = %v", err)
	}
	encoded, err := json.Marshal(channel)
	if err != nil {
		t.Fatalf("marshal channel: %v", err)
	}
	if strings.Contains(string(encoded), "secret-token") {
		t.Fatalf("channel response leaked secret: %s", encoded)
	}

	var stored string
	if err := db.QueryRow(`SELECT secret_json FROM notification_channels WHERE id = ?`, channel.ID).Scan(&stored); err != nil {
		t.Fatalf("query sealed secret: %v", err)
	}
	if strings.Contains(stored, "secret-token") || !strings.HasPrefix(stored, "tradis:v1:") {
		t.Fatalf("secret was not encrypted: %q", stored)
	}

	loaded, err := GetNotificationChannelForDelivery(channel.ID)
	if err != nil {
		t.Fatalf("GetNotificationChannelForDelivery() error = %v", err)
	}
	if loaded.Secrets["token"] != "secret-token" {
		t.Fatalf("loaded secret = %#v", loaded.Secrets)
	}
	_ = os.Unsetenv("TRADIS_MASTER_KEY")
}

func TestNotificationChannelRejectsCredentialsEmbeddedInVisibleURL(t *testing.T) {
	setupNotificationTestDB(t)
	_, err := CreateNotificationChannel(NotificationChannel{
		Name: "不安全 Webhook", Type: "webhook", Enabled: true,
		Config: map[string]string{"url": "https://token@example.test/hook?access_token=secret"},
	})
	if err == nil {
		t.Fatal("CreateNotificationChannel() accepted visible URL credentials")
	}
}
