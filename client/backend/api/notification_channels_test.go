package api

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dockerpanel/backend/pkg/database"
	"dockerpanel/backend/pkg/notify"

	"github.com/gin-gonic/gin"
)

type failingNotificationSender struct{}

func (failingNotificationSender) Send(context.Context, notify.Channel, notify.Event) (int, error) {
	return 0, errors.New("endpoint is unavailable")
}

func TestNotificationChannelValidatesPushPlusAndWeComSecrets(t *testing.T) {
	router := setupNotificationChannelAPI(t)
	for _, body := range []string{
		`{"name":"PushPlus","type":"pushplus","enabled":true,"config":{}}`,
		`{"name":"企业微信","type":"wecom","enabled":true,"config":{}}`,
	} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/notification-channels", bytes.NewBufferString(body)))
		if response.Code != http.StatusBadRequest {
			t.Fatalf("missing secret must be rejected: status=%d body=%s", response.Code, response.Body.String())
		}
	}
}

func TestNotificationChannelValidatesPushPlusChannelContract(t *testing.T) {
	router := setupNotificationChannelAPI(t)
	valid := []string{
		`{"name":"默认微信","type":"pushplus","enabled":true,"config":{},"secrets":{"token":"pp-token"}}`,
		`{"name":"企业微信应用","type":"pushplus","enabled":true,"config":{"channel":"cp","option":"nas-app"},"secrets":{"token":"pp-token"}}`,
		`{"name":"企业微信群机器人","type":"pushplus","enabled":true,"config":{"channel":"webhook","option":"nas-bot"},"secrets":{"token":"pp-token"}}`,
	}
	for _, body := range valid {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/notification-channels", bytes.NewBufferString(body)))
		if response.Code != http.StatusCreated {
			t.Fatalf("valid pushplus config rejected: status=%d body=%s", response.Code, response.Body.String())
		}
		if bytes.Contains(response.Body.Bytes(), []byte("pp-token")) {
			t.Fatalf("response leaked token: %s", response.Body.String())
		}
	}

	invalid := []string{
		`{"name":"缺应用编码","type":"pushplus","enabled":true,"config":{"channel":"cp"},"secrets":{"token":"pp-token"}}`,
		`{"name":"缺 Webhook 编码","type":"pushplus","enabled":true,"config":{"channel":"webhook"},"secrets":{"token":"pp-token"}}`,
		`{"name":"历史 cpwebhook","type":"pushplus","enabled":true,"config":{"channel":"cpwebhook","option":"x"},"secrets":{"token":"pp-token"}}`,
		`{"name":"未知渠道","type":"pushplus","enabled":true,"config":{"channel":"sms","option":"x"},"secrets":{"token":"pp-token"}}`,
	}
	for _, body := range invalid {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/notification-channels", bytes.NewBufferString(body)))
		if response.Code != http.StatusBadRequest {
			t.Fatalf("invalid pushplus config must be rejected at save: status=%d body=%s", response.Code, response.Body.String())
		}
	}

	// 直连企业微信机器人不经过 PushPlus，不受渠道契约影响。
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/notification-channels",
		bytes.NewBufferString(`{"name":"直连机器人","type":"wecom","enabled":true,"config":{},"secrets":{"key":"robot-key"}}`)))
	if response.Code != http.StatusCreated {
		t.Fatalf("direct wecom must stay valid: status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestNotificationChannelTestDeliveryRejectsInvalidPushPlusConfig(t *testing.T) {
	router := setupNotificationChannelAPI(t)
	channel, err := database.CreateNotificationChannel(database.NotificationChannel{
		Name: "历史 PushPlus", Type: notify.ChannelTypePushPlus, Enabled: true,
		Config:  map[string]string{"channel": "cpwebhook"},
		Secrets: map[string]string{"token": "pp-token"},
	})
	if err != nil {
		t.Fatalf("CreateNotificationChannel() error = %v", err)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/notification-channels/"+channel.ID+"/test", nil))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("test delivery must fail fast with a clear config error: status=%d body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "PushPlus") {
		t.Fatalf("error must name the config problem: %s", response.Body.String())
	}
}

func TestNotificationChannelUpdatePreservesExistingSecret(t *testing.T) {
	router := setupNotificationChannelAPI(t)
	created, err := database.CreateNotificationChannel(database.NotificationChannel{
		Name: "PushPlus", Type: notify.ChannelTypePushPlus, Enabled: true,
		Secrets: map[string]string{"token": "existing-token"},
	})
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]any{
		"name": "PushPlus 新名称", "type": notify.ChannelTypePushPlus, "enabled": true,
		"config": map[string]string{},
	})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/api/notification-channels/"+created.ID, bytes.NewReader(body))
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("secret-preserving update failed: status=%d body=%s", response.Code, response.Body.String())
	}
	loaded, err := database.GetNotificationChannelForDelivery(created.ID)
	if err != nil || loaded.Secrets["token"] != "existing-token" {
		t.Fatalf("secret was not preserved: channel=%#v err=%v", loaded, err)
	}
}

func setupNotificationChannelAPI(t *testing.T) *gin.Engine {
	t.Helper()
	_ = database.Close()
	if err := database.InitDB(filepath.Join(t.TempDir(), "notification-channels.db")); err != nil {
		t.Fatalf("InitDB() error = %v", err)
	}
	t.Setenv("TRADIS_MASTER_KEY", base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32)))
	t.Cleanup(func() {
		database.SetNotificationEmitter(nil)
		_ = database.Close()
	})
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterNotificationChannelRoutes(router.Group("/api"))
	return router
}

func TestNotificationChannelAPICreatesListsAndDoesNotRequireGo(t *testing.T) {
	router := setupNotificationChannelAPI(t)
	body := bytes.NewBufferString(`{"name":"家用 ntfy","type":"ntfy","enabled":true,"categories":["deploy_task"],"config":{"server":"https://ntfy.example","topic":"tradis"},"secrets":{"token":"test-token"}}`)
	created := httptest.NewRecorder()
	router.ServeHTTP(created, httptest.NewRequest(http.MethodPost, "/api/notification-channels", body))
	if created.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", created.Code, created.Body.String())
	}
	if bytes.Contains(created.Body.Bytes(), []byte("test-token")) {
		t.Fatalf("create response leaked secret: %s", created.Body.String())
	}

	listed := httptest.NewRecorder()
	router.ServeHTTP(listed, httptest.NewRequest(http.MethodGet, "/api/notification-channels", nil))
	if listed.Code != http.StatusOK || !bytes.Contains(listed.Body.Bytes(), []byte("家用 ntfy")) || bytes.Contains(listed.Body.Bytes(), []byte("test-token")) {
		t.Fatalf("list status=%d body=%s", listed.Code, listed.Body.String())
	}
}

func TestNotificationDeliveryFailureNeverChangesInAppNotificationResult(t *testing.T) {
	setupNotificationChannelAPI(t)
	previous := notificationChannelSender
	notificationChannelSender = failingNotificationSender{}
	t.Cleanup(func() { notificationChannelSender = previous })
	channel, err := database.CreateNotificationChannel(database.NotificationChannel{
		Name: "失败通道", Type: "webhook", Enabled: true,
		Categories: []string{"system"}, Config: map[string]string{"url": "https://example.invalid/hook"},
	})
	if err != nil {
		t.Fatalf("CreateNotificationChannel() error = %v", err)
	}
	notification := &database.Notification{Type: "error", Category: "system", Message: "业务任务失败"}
	if err := database.SaveNotification(notification); err != nil {
		t.Fatalf("SaveNotification() error = %v", err)
	}
	if notification.ID == 0 {
		t.Fatal("in-app notification was not saved")
	}

	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		deliveries, err := database.ListNotificationDeliveries(channel.ID, 10)
		if err == nil && len(deliveries) > 0 {
			if deliveries[0].Status == notify.DeliveryFailed && deliveries[0].Attempts == 3 {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("notification delivery was not recorded")
}
