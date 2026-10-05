package api

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"dockerpanel/backend/pkg/database"
	"dockerpanel/backend/pkg/notify"

	"github.com/gin-gonic/gin"
)

type notificationChannelStore struct{}

func (notificationChannelStore) ListEnabledChannels(_ context.Context, category string) ([]notify.Channel, error) {
	channels, err := database.ListNotificationChannelsForDelivery(category)
	if err != nil {
		return nil, err
	}
	result := make([]notify.Channel, 0, len(channels))
	for _, channel := range channels {
		result = append(result, notify.Channel{
			ID: channel.ID, Name: channel.Name, Type: channel.Type, Enabled: channel.Enabled,
			Categories: channel.Categories, Config: channel.Config, Secrets: channel.Secrets,
			SecretSet: channel.SecretSet, CreatedAt: channel.CreatedAt, UpdatedAt: channel.UpdatedAt,
		})
	}
	return result, nil
}

func (notificationChannelStore) CreateDelivery(_ context.Context, delivery notify.Delivery) (notify.Delivery, bool, error) {
	created, inserted, err := database.CreateNotificationDelivery(database.NotificationDelivery{
		EnvironmentID: delivery.EnvironmentID,
		ChannelID:     delivery.ChannelID, NotificationID: delivery.NotificationID, DedupeKey: delivery.DedupeKey,
		EventCategory: delivery.EventCategory, EventType: delivery.EventType, Status: delivery.Status,
		Attempts: delivery.Attempts, ResponseCode: delivery.ResponseCode, LastError: delivery.LastError,
		CreatedAt: delivery.CreatedAt, UpdatedAt: delivery.UpdatedAt,
	})
	return toNotifyDelivery(created), inserted, err
}

func (notificationChannelStore) UpdateDelivery(_ context.Context, delivery notify.Delivery) error {
	return database.UpdateNotificationDelivery(database.NotificationDelivery{
		ID: delivery.ID, EnvironmentID: delivery.EnvironmentID, ChannelID: delivery.ChannelID, NotificationID: delivery.NotificationID,
		DedupeKey: delivery.DedupeKey, EventCategory: delivery.EventCategory, EventType: delivery.EventType,
		Status: delivery.Status, Attempts: delivery.Attempts, ResponseCode: delivery.ResponseCode,
		LastError: delivery.LastError, CreatedAt: delivery.CreatedAt, UpdatedAt: delivery.UpdatedAt,
	})
}

func toNotifyDelivery(delivery database.NotificationDelivery) notify.Delivery {
	return notify.Delivery{
		ID: delivery.ID, EnvironmentID: delivery.EnvironmentID, ChannelID: delivery.ChannelID, NotificationID: delivery.NotificationID,
		DedupeKey: delivery.DedupeKey, EventCategory: delivery.EventCategory, EventType: delivery.EventType,
		Status: delivery.Status, Attempts: delivery.Attempts, ResponseCode: delivery.ResponseCode,
		LastError: delivery.LastError, CreatedAt: delivery.CreatedAt, UpdatedAt: delivery.UpdatedAt,
	}
}

var notificationChannelSender notify.Sender = notify.HTTPSender{}

func newNotificationDispatcher() *notify.Dispatcher {
	return notify.NewDispatcher(notificationChannelStore{}, notificationChannelSender)
}

func RegisterNotificationChannelRoutes(group *gin.RouterGroup) {
	routes := group.Group("/notification-channels")
	routes.GET("", listNotificationChannels)
	routes.POST("", createNotificationChannel)
	routes.GET("/:id", getNotificationChannel)
	routes.PUT("/:id", updateNotificationChannel)
	routes.DELETE("/:id", deleteNotificationChannel)
	routes.POST("/:id/test", testNotificationChannel)
	routes.GET("/:id/deliveries", listNotificationChannelDeliveries)

	database.SetNotificationEmitter(func(notification database.Notification) {
		event := eventFromNotification(notification)
		if err := newNotificationDispatcher().Dispatch(context.Background(), event); err != nil {
			// The notification has already been safely persisted. Delivery failures
			// are recorded per channel and must not affect the originating task.
			return
		}
	})
}

type notificationChannelRequest struct {
	Name         string            `json:"name"`
	Type         string            `json:"type"`
	Enabled      *bool             `json:"enabled"`
	Categories   []string          `json:"categories"`
	Config       map[string]string `json:"config"`
	Secrets      map[string]string `json:"secrets"`
	ClearSecrets bool              `json:"clearSecrets"`
}

func createNotificationChannel(c *gin.Context) {
	var request notificationChannelRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		respondError(c, http.StatusBadRequest, "通知通道参数无效", err)
		return
	}
	channel := notificationChannelFromRequest("", request, true)
	if err := validateNotificationChannel(channel); err != nil {
		respondError(c, http.StatusBadRequest, err.Error(), nil)
		return
	}
	created, err := database.CreateNotificationChannel(channel)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "保存通知通道失败", err)
		return
	}
	c.JSON(http.StatusCreated, created)
}

func listNotificationChannels(c *gin.Context) {
	channels, err := database.ListNotificationChannels()
	if err != nil {
		respondError(c, http.StatusInternalServerError, "读取通知通道失败", err)
		return
	}
	c.JSON(http.StatusOK, channels)
}

func getNotificationChannel(c *gin.Context) {
	channel, err := database.GetNotificationChannel(c.Param("id"))
	if err != nil {
		respondError(c, http.StatusNotFound, "通知通道不存在", err)
		return
	}
	c.JSON(http.StatusOK, channel)
}

func updateNotificationChannel(c *gin.Context) {
	current, err := database.GetNotificationChannel(c.Param("id"))
	if err != nil {
		respondError(c, http.StatusNotFound, "通知通道不存在", err)
		return
	}
	var request notificationChannelRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		respondError(c, http.StatusBadRequest, "通知通道参数无效", err)
		return
	}
	channel := notificationChannelFromRequest(current.ID, request, current.Enabled)
	// 仅类型未变时沿用“密钥已配置”标记；切换渠道类型后旧密钥对新类型无意义，
	// 空密钥必须被校验拦截，而不是带着旧标记投递时才失败。
	if strings.EqualFold(strings.TrimSpace(channel.Type), strings.TrimSpace(current.Type)) {
		channel.SecretSet = current.SecretSet
	}
	if err := validateNotificationChannel(channel); err != nil {
		respondError(c, http.StatusBadRequest, err.Error(), nil)
		return
	}
	replaceSecrets := request.Secrets != nil || request.ClearSecrets
	if request.ClearSecrets {
		channel.Secrets = map[string]string{}
	}
	updated, err := database.UpdateNotificationChannel(channel, replaceSecrets)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "更新通知通道失败", err)
		return
	}
	c.JSON(http.StatusOK, updated)
}

func deleteNotificationChannel(c *gin.Context) {
	if err := database.DeleteNotificationChannel(c.Param("id")); err != nil {
		respondError(c, http.StatusNotFound, "删除通知通道失败", err)
		return
	}
	c.Status(http.StatusNoContent)
}

func testNotificationChannel(c *gin.Context) {
	channel, err := database.GetNotificationChannelForDelivery(c.Param("id"))
	if err != nil {
		respondError(c, http.StatusNotFound, "通知通道不存在", err)
		return
	}
	// 测试投递前复用保存时的渠道校验，避免对明显无效的配置发起无意义网络请求。
	if strings.EqualFold(channel.Type, notify.ChannelTypePushPlus) {
		if err := validatePushPlusConfig(channel.Config); err != nil {
			respondError(c, http.StatusBadRequest, err.Error(), nil)
			return
		}
	}
	dispatcher := newNotificationDispatcher()
	dispatcher.MaxAttempts = 1
	event := notify.Event{
		ID:       fmt.Sprintf("notification-test-%d", time.Now().UnixNano()),
		Category: "system",
		Type:     "test",
		Level:    "info",
		Message:  "TRADIS 通知通道测试成功",
		Occurred: time.Now(),
	}
	if err := dispatcher.DispatchChannel(c.Request.Context(), notify.Channel{
		ID: channel.ID, Name: channel.Name, Type: channel.Type, Enabled: true,
		Categories: channel.Categories, Config: channel.Config, Secrets: channel.Secrets,
	}, event); err != nil {
		respondError(c, http.StatusInternalServerError, "发送测试通知失败", err)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"message": "测试通知已提交"})
}

func listNotificationChannelDeliveries(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	deliveries, err := database.ListNotificationDeliveries(c.Param("id"), limit)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "读取投递记录失败", err)
		return
	}
	c.JSON(http.StatusOK, deliveries)
}

func notificationChannelFromRequest(id string, request notificationChannelRequest, defaultEnabled bool) database.NotificationChannel {
	enabled := defaultEnabled
	if request.Enabled != nil {
		enabled = *request.Enabled
	}
	return database.NotificationChannel{
		ID: id, Name: strings.TrimSpace(request.Name), Type: strings.TrimSpace(request.Type), Enabled: enabled,
		Categories: request.Categories, Config: request.Config, Secrets: request.Secrets,
	}
}

func validateNotificationChannel(channel database.NotificationChannel) error {
	if strings.TrimSpace(channel.Name) == "" {
		return fmt.Errorf("通知通道名称不能为空")
	}
	switch strings.ToLower(strings.TrimSpace(channel.Type)) {
	case notify.ChannelTypeWebhook:
		if strings.TrimSpace(channel.Config["url"]) == "" {
			return fmt.Errorf("Webhook 地址不能为空")
		}
	case notify.ChannelTypeWeCom:
		if strings.TrimSpace(channel.Secrets["key"]) == "" && !channel.SecretSet {
			return fmt.Errorf("企业微信机器人 Key 不能为空")
		}
	case notify.ChannelTypeNtfy:
		if strings.TrimSpace(channel.Config["topic"]) == "" {
			return fmt.Errorf("ntfy Topic 不能为空")
		}
	case notify.ChannelTypeGotify:
		if strings.TrimSpace(channel.Config["server"]) == "" {
			return fmt.Errorf("Gotify 地址不能为空")
		}
	case notify.ChannelTypeBark:
		return nil
	case notify.ChannelTypePushPlus:
		if strings.TrimSpace(channel.Secrets["token"]) == "" && !channel.SecretSet {
			return fmt.Errorf("PushPlus Token 不能为空")
		}
		if err := validatePushPlusConfig(channel.Config); err != nil {
			return err
		}
	default:
		return fmt.Errorf("不支持的通知通道类型")
	}
	return nil
}

// validatePushPlusConfig 按 PushPlus 官方协议校验渠道：空/wechat 为默认微信公众号，
// cp（企业微信应用）和 webhook（含企业微信群机器人）必须填写渠道配置编码；
// 其余取值（含历史 cpwebhook）一律拒绝。
func validatePushPlusConfig(config map[string]string) error {
	channel := strings.ToLower(strings.TrimSpace(config["channel"]))
	switch channel {
	case "", "wechat":
		return nil
	case "cp", "webhook":
		if strings.TrimSpace(config["option"]) == "" {
			return fmt.Errorf("PushPlus 当前渠道必须填写配置编码")
		}
		return nil
	default:
		return fmt.Errorf("PushPlus 推送渠道无效")
	}
}

func eventFromNotification(notification database.Notification) notify.Event {
	// notification.CreatedAt 可能是历史 naive 串，也可能是新的 RFC3339；用兼容解析。
	occurred, ok := database.ParseStoredTime(notification.CreatedAt)
	if !ok || occurred.IsZero() {
		occurred = time.Now()
	}
	return notify.Event{
		ID:             fmt.Sprintf("notification-%d", notification.ID),
		EnvironmentID:  notification.EnvironmentID,
		NotificationID: notification.ID,
		Category:       notification.Category,
		Type:           notification.EventType,
		Level:          notification.Type,
		Message:        notification.Message,
		Occurred:       occurred,
	}
}
