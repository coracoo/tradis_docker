package notify

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type HTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}

type HTTPSender struct {
	Client HTTPClient
}

func (s HTTPSender) Send(ctx context.Context, channel Channel, event Event) (int, error) {
	client := s.Client
	if client == nil {
		client = &http.Client{Timeout: 12 * time.Second}
	}
	switch channel.Type {
	case ChannelTypeWebhook:
		return s.sendWebhook(ctx, client, channel, event)
	case ChannelTypeWeCom:
		return s.sendWeCom(ctx, client, channel, event)
	case ChannelTypeNtfy:
		return s.sendNtfy(ctx, client, channel, event)
	case ChannelTypeGotify:
		return s.sendGotify(ctx, client, channel, event)
	case ChannelTypeBark:
		return s.sendBark(ctx, client, channel, event)
	case ChannelTypePushPlus:
		return s.sendPushPlus(ctx, client, channel, event)
	default:
		return 0, fmt.Errorf("不支持的通知通道类型: %s", channel.Type)
	}
}

func (s HTTPSender) sendWebhook(ctx context.Context, client HTTPClient, channel Channel, event Event) (int, error) {
	target, err := requiredHTTPURL(channel.Config["url"])
	if err != nil {
		return 0, err
	}
	body, err := json.Marshal(eventPayload(event))
	if err != nil {
		return 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	if authorization := strings.TrimSpace(channel.Secrets["authorization"]); authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	if secret := strings.TrimSpace(channel.Secrets["signing_secret"]); secret != "" {
		mac := hmac.New(sha256.New, []byte(secret))
		_, _ = mac.Write(body)
		req.Header.Set("X-TRADIS-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	}
	return execute(client, req)
}

// sendWeCom delivers to a WeCom (企业微信) group robot. The robot key is a
// secret; the official API domain is used unless a private proxy server is
// configured. Payload uses the robot markdown format with a hard 4096-byte
// content bound.
func (s HTTPSender) sendWeCom(ctx context.Context, client HTTPClient, channel Channel, event Event) (int, error) {
	server := strings.TrimRight(strings.TrimSpace(channel.Config["server"]), "/")
	if server == "" {
		server = "https://qyapi.weixin.qq.com"
	}
	if _, err := requiredHTTPURL(server); err != nil {
		return 0, err
	}
	key := strings.TrimSpace(channel.Secrets["key"])
	if key == "" {
		return 0, fmt.Errorf("企业微信机器人 Key 未配置")
	}
	content := fmt.Sprintf("**TRADIS 通知**\n> 级别：%s\n> 分类：%s\n> %s", event.Level, event.Category, event.Message)
	if runes := []rune(content); len(runes) > 1200 {
		content = string(runes[:1200]) + "…"
	}
	body, _ := json.Marshal(map[string]any{
		"msgtype":  "markdown",
		"markdown": map[string]any{"content": content},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, server+"/cgi-bin/webhook/send?key="+url.QueryEscape(key), bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	// 企业微信机器人在 HTTP 200 中用 errcode 表达失败（无效 Key、限流等），
	// 只看状态码会把业务失败记成成功。
	response, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(response.Body, 4<<10))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return response.StatusCode, fmt.Errorf("远端返回 HTTP %d", response.StatusCode)
	}
	var result struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}
	if jsonErr := json.Unmarshal(raw, &result); jsonErr == nil && result.ErrCode != 0 {
		return response.StatusCode, fmt.Errorf("企业微信返回 errcode=%d: %s", result.ErrCode, result.ErrMsg)
	}
	return response.StatusCode, nil
}

func (s HTTPSender) sendNtfy(ctx context.Context, client HTTPClient, channel Channel, event Event) (int, error) {
	server := strings.TrimRight(strings.TrimSpace(channel.Config["server"]), "/")
	if server == "" {
		server = "https://ntfy.sh"
	}
	if _, err := requiredHTTPURL(server); err != nil {
		return 0, err
	}
	topic := strings.Trim(strings.TrimSpace(channel.Config["topic"]), "/")
	if topic == "" || strings.Contains(topic, "/") {
		return 0, fmt.Errorf("ntfy topic 无效")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, server+"/"+url.PathEscape(topic), strings.NewReader(event.Message))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "text/plain; charset=utf-8")
	req.Header.Set("Title", "TRADIS")
	if token := strings.TrimSpace(channel.Secrets["token"]); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return execute(client, req)
}

func (s HTTPSender) sendGotify(ctx context.Context, client HTTPClient, channel Channel, event Event) (int, error) {
	server, err := requiredHTTPURL(channel.Config["server"])
	if err != nil {
		return 0, err
	}
	token := strings.TrimSpace(channel.Secrets["token"])
	if token == "" {
		return 0, fmt.Errorf("Gotify 应用 Token 未配置")
	}
	body, _ := json.Marshal(map[string]any{"title": "TRADIS", "message": event.Message, "priority": priorityFor(event.Level)})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(server, "/")+"/message", bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Gotify-Key", token)
	return execute(client, req)
}

func (s HTTPSender) sendBark(ctx context.Context, client HTTPClient, channel Channel, event Event) (int, error) {
	server := strings.TrimRight(strings.TrimSpace(channel.Config["server"]), "/")
	if server == "" {
		server = "https://api.day.app"
	}
	if _, err := requiredHTTPURL(server); err != nil {
		return 0, err
	}
	deviceKey := strings.TrimSpace(channel.Secrets["device_key"])
	if deviceKey == "" {
		return 0, fmt.Errorf("Bark 设备密钥未配置")
	}
	body, _ := json.Marshal(map[string]any{"device_key": deviceKey, "title": "TRADIS", "body": event.Message, "group": "tradis"})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, server+"/push", bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	return execute(client, req)
}

func (s HTTPSender) sendPushPlus(ctx context.Context, client HTTPClient, channel Channel, event Event) (int, error) {
	server := strings.TrimRight(strings.TrimSpace(channel.Config["server"]), "/")
	if server == "" {
		server = "https://www.pushplus.plus"
	}
	if _, err := requiredHTTPURL(server); err != nil {
		return 0, err
	}
	token := strings.TrimSpace(channel.Secrets["token"])
	if token == "" {
		return 0, fmt.Errorf("PushPlus Token 未配置")
	}
	payload := map[string]any{
		"token":   token,
		"title":   "TRADIS",
		"content": event.Message,
		// 纯文本模板，保留消息换行；html 模板会把换行折叠掉。
		"template": "txt",
	}
	// 可选推送渠道：wechat（默认）/ cp（企业微信应用）/ webhook（群机器人 Webhook），原样透传。
	if pushChannel := strings.TrimSpace(channel.Config["channel"]); pushChannel != "" {
		payload["channel"] = pushChannel
	}
	// cp/webhook 渠道的渠道配置编码，保存在普通配置中。
	if option := strings.TrimSpace(channel.Config["option"]); option != "" {
		payload["option"] = option
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, server+"/send", bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	// PushPlus 业务失败也返回 HTTP 200（错误在响应体 code 字段），
	// 必须解析响应体判断投递结果，否则 token 错误也会被记为成功。
	response, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return response.StatusCode, fmt.Errorf("远端返回 HTTP %d", response.StatusCode)
	}
	var result struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&result); err != nil {
		return response.StatusCode, fmt.Errorf("PushPlus 响应解析失败: %w", err)
	}
	if result.Code != 200 {
		return response.StatusCode, fmt.Errorf("PushPlus 投递失败: code=%d msg=%s", result.Code, result.Msg)
	}
	return response.StatusCode, nil
}

func eventPayload(event Event) map[string]any {
	return map[string]any{
		"id": event.ID, "notificationId": event.NotificationID, "category": event.Category,
		"type": event.Type, "level": event.Level, "message": event.Message,
		"occurredAt": event.Occurred.UTC().Format(time.RFC3339),
	}
}

func requiredHTTPURL(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", fmt.Errorf("通知地址必须是 HTTP 或 HTTPS URL")
	}
	return parsed.String(), nil
}

func execute(client HTTPClient, req *http.Request) (int, error) {
	response, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return response.StatusCode, fmt.Errorf("远端返回 HTTP %d", response.StatusCode)
	}
	return response.StatusCode, nil
}

func priorityFor(level string) int {
	if strings.EqualFold(level, "error") || strings.EqualFold(level, "danger") {
		return 8
	}
	if strings.EqualFold(level, "warning") {
		return 5
	}
	return 3
}
