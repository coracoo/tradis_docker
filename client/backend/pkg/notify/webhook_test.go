package notify

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type recordingHTTPClient struct {
	method  string
	url     string
	headers http.Header
	body    string

	respStatus int
	respBody   string
}

func (c *recordingHTTPClient) Do(request *http.Request) (*http.Response, error) {
	body, _ := io.ReadAll(request.Body)
	c.method = request.Method
	c.url = request.URL.String()
	c.headers = request.Header.Clone()
	c.body = string(body)
	status := c.respStatus
	if status == 0 {
		status = http.StatusNoContent
	}
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(c.respBody))}, nil
}

func TestHTTPSenderFormatsSupportedChannelRequests(t *testing.T) {
	testCases := []struct {
		name       string
		channel    Channel
		wantURL    string
		wantBody   string
		wantHeader string
		wantValue  string
	}{
		{
			name: "webhook", channel: Channel{Type: ChannelTypeWebhook, Config: map[string]string{"url": "https://hooks.example.test/tradis"}, Secrets: map[string]string{"signing_secret": "signing"}},
			wantURL: "https://hooks.example.test/tradis", wantBody: "应用部署完成", wantHeader: "X-Tradis-Signature",
		},
		{
			name: "ntfy", channel: Channel{Type: ChannelTypeNtfy, Config: map[string]string{"server": "https://ntfy.example.test", "topic": "tradis"}, Secrets: map[string]string{"token": "ntfy-token"}},
			wantURL: "https://ntfy.example.test/tradis", wantBody: "应用部署完成", wantHeader: "Authorization", wantValue: "Bearer ntfy-token",
		},
		{
			name: "gotify", channel: Channel{Type: ChannelTypeGotify, Config: map[string]string{"server": "https://gotify.example.test"}, Secrets: map[string]string{"token": "gotify-token"}},
			wantURL: "https://gotify.example.test/message", wantBody: "应用部署完成", wantHeader: "X-Gotify-Key", wantValue: "gotify-token",
		},
		{
			name: "bark", channel: Channel{Type: ChannelTypeBark, Config: map[string]string{"server": "https://bark.example.test"}, Secrets: map[string]string{"device_key": "device-key"}},
			wantURL: "https://bark.example.test/push", wantBody: "应用部署完成", wantHeader: "Content-Type", wantValue: "application/json",
		},
		{
			name: "wecom", channel: Channel{Type: ChannelTypeWeCom, Secrets: map[string]string{"key": "robot-key"}},
			wantURL: "https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=robot-key", wantBody: "应用部署完成", wantHeader: "Content-Type", wantValue: "application/json",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			client := &recordingHTTPClient{}
			code, err := (HTTPSender{Client: client}).Send(context.Background(), testCase.channel, Event{Message: "应用部署完成", Occurred: time.Now()})
			if err != nil || code != http.StatusNoContent {
				t.Fatalf("Send() = %d, %v", code, err)
			}
			if client.method != http.MethodPost || client.url != testCase.wantURL || !strings.Contains(client.body, testCase.wantBody) {
				t.Fatalf("request = method=%s url=%s body=%s", client.method, client.url, client.body)
			}
			if strings.Contains(testCase.name, "wecom") && !strings.Contains(client.body, "\"markdown\"") {
				t.Fatalf("wecom payload must use the robot markdown format: %s", client.body)
			}
			if testCase.wantHeader != "" && client.headers.Get(testCase.wantHeader) == "" {
				t.Fatalf("missing %s header: %#v", testCase.wantHeader, client.headers)
			}
			if testCase.wantValue != "" && client.headers.Get(testCase.wantHeader) != testCase.wantValue {
				t.Fatalf("%s = %q", testCase.wantHeader, client.headers.Get(testCase.wantHeader))
			}
		})
	}
}

func TestWeComSenderTreatsErrCodeAsFailure(t *testing.T) {
	client := &recordingHTTPClient{
		respStatus: http.StatusOK,
		respBody:   `{"errcode":93000,"errmsg":"invalid webhook url"}`,
	}
	_, err := (HTTPSender{Client: client}).Send(context.Background(), Channel{
		Type: ChannelTypeWeCom, Secrets: map[string]string{"key": "robot-key"},
	}, Event{Message: "部署完成", Occurred: time.Now()})
	if err == nil || !strings.Contains(err.Error(), "errcode=93000") {
		t.Fatalf("errcode failure must surface: %v", err)
	}

	ok := &recordingHTTPClient{respStatus: http.StatusOK, respBody: `{"errcode":0,"errmsg":"ok"}`}
	if code, err := (HTTPSender{Client: ok}).Send(context.Background(), Channel{
		Type: ChannelTypeWeCom, Secrets: map[string]string{"key": "robot-key"},
	}, Event{Message: "部署完成", Occurred: time.Now()}); err != nil || code != http.StatusOK {
		t.Fatalf("errcode=0 must succeed: %d %v", code, err)
	}
}
func TestSendPushPlusPostsTokenChannelAndOption(t *testing.T) {
	tests := []struct {
		name       string
		config     map[string]string
		wantFields []string
		absent     []string
	}{
		{name: "default wechat", config: map[string]string{}, absent: []string{`"channel"`, `"option"`}},
		{name: "wecom app", config: map[string]string{"channel": "cp", "option": "nas-app"}, wantFields: []string{`"channel":"cp"`, `"option":"nas-app"`}},
		{name: "wecom robot through pushplus", config: map[string]string{"channel": "webhook", "option": "nas-bot"}, wantFields: []string{`"channel":"webhook"`, `"option":"nas-bot"`}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &recordingHTTPClient{respBody: `{"code":200,"msg":"请求成功"}`}
			code, err := (HTTPSender{Client: client}).Send(context.Background(), Channel{
				Type:    ChannelTypePushPlus,
				Config:  tt.config,
				Secrets: map[string]string{"token": "tok-1"},
			}, Event{Message: "应用部署完成", Occurred: time.Now()})
			if err != nil || code != http.StatusNoContent {
				t.Fatalf("Send() = %d, %v", code, err)
			}
			if client.url != "https://www.pushplus.plus/send" {
				t.Fatalf("url = %s", client.url)
			}
			for _, want := range append([]string{`"token":"tok-1"`, `"content":"应用部署完成"`, `"template":"txt"`}, tt.wantFields...) {
				if !strings.Contains(client.body, want) {
					t.Fatalf("body 缺少 %s: %s", want, client.body)
				}
			}
			for _, absent := range tt.absent {
				if strings.Contains(client.body, absent) {
					t.Fatalf("body 不应包含 %s: %s", absent, client.body)
				}
			}
		})
	}

	if _, err := (HTTPSender{Client: &recordingHTTPClient{}}).Send(context.Background(), Channel{Type: ChannelTypePushPlus}, Event{}); err == nil || !strings.Contains(err.Error(), "Token") {
		t.Fatalf("缺少 Token 必须报错, got %v", err)
	}
}

func TestSendPushPlusTreatsBusinessErrorCodeAsFailure(t *testing.T) {
	// PushPlus 业务失败也返回 HTTP 200，必须按响应体 code 判失败。
	client := &recordingHTTPClient{respStatus: http.StatusOK, respBody: `{"code":500,"msg":"token错误"}`}
	_, err := (HTTPSender{Client: client}).Send(context.Background(), Channel{
		Type:    ChannelTypePushPlus,
		Secrets: map[string]string{"token": "bad-token"},
	}, Event{Message: "测试", Occurred: time.Now()})
	if err == nil || !strings.Contains(err.Error(), "token错误") {
		t.Fatalf("code!=200 必须报错, got %v", err)
	}
}
