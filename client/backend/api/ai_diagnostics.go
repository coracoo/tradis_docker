package api

import (
	"context"
	"dockerpanel/backend/pkg/aihttp"
	"encoding/json"
	"errors"
	"strings"
)

type aiDiagnosticConfig struct {
	BaseURL     string
	APIKey      string
	Model       string
	Temperature float64
}

type aiDiagnosticStage struct {
	OK        bool   `json:"ok"`
	Verified  bool   `json:"verified"`
	LatencyMs int64  `json:"latencyMs,omitempty"`
	Attempts  int    `json:"attempts,omitempty"`
	TraceID   string `json:"traceId,omitempty"`
	Code      string `json:"code,omitempty"`
	Message   string `json:"message"`
}

type aiDiagnosticResult struct {
	OK        bool              `json:"ok"`
	Endpoint  string            `json:"endpoint"`
	Model     string            `json:"model"`
	LatencyMs int64             `json:"latencyMs"`
	Transport aiDiagnosticStage `json:"transport"`
	Inference aiDiagnosticStage `json:"inference"`
}

func runAIDiagnostic(
	ctx context.Context,
	config aiDiagnosticConfig,
	diagnosticClient *aihttp.Client,
	inferenceClient *aihttp.Client,
) aiDiagnosticResult {
	if diagnosticClient == nil {
		diagnosticClient = aihttp.New(aihttp.DiagnosticPolicy())
	}
	if inferenceClient == nil {
		inferenceClient = aihttp.New(aihttp.UtilityPolicy())
	}

	endpoint, _ := aihttp.ChatCompletionsURL(config.BaseURL)
	result := aiDiagnosticResult{
		Endpoint: endpoint,
		Model:    strings.TrimSpace(config.Model),
		Inference: aiDiagnosticStage{
			Code:    "skipped",
			Message: "推理检测未执行",
		},
	}

	modelsResult, err := diagnosticClient.Models(ctx, config.BaseURL, config.APIKey)
	if err != nil {
		var providerErr *aihttp.Error
		if errors.As(err, &providerErr) &&
			(providerErr.StatusCode == 404 || providerErr.StatusCode == 405) {
			result.Transport = aiDiagnosticStage{
				OK:       true,
				Verified: false,
				Attempts: providerErr.Attempts,
				TraceID:  providerErr.TraceID,
				Code:     "models_unsupported",
				Message:  "服务未提供模型列表接口，已继续检测推理能力",
			}
		} else {
			result.Transport = diagnosticStageFromError(err)
			return result
		}
	} else {
		result.Transport = aiDiagnosticStage{
			OK:        true,
			Verified:  true,
			LatencyMs: modelsResult.Duration.Milliseconds(),
			Attempts:  modelsResult.Attempts,
			TraceID:   modelsResult.TraceID,
			Message:   "服务连接与鉴权正常",
		}
	}

	payload := map[string]any{
		"model": result.Model,
		"messages": []map[string]string{
			{"role": "user", "content": "Reply with pong."},
		},
		"temperature": config.Temperature,
		"max_tokens":  8,
		"stream":      false,
	}
	chatResult, err := inferenceClient.ChatCompletions(ctx, config.BaseURL, config.APIKey, payload)
	if err != nil {
		result.Inference = diagnosticStageFromError(err)
		return result
	}
	if !hasChatCompletionChoice(chatResult.Body) {
		result.Inference = aiDiagnosticStage{
			Attempts:  chatResult.Attempts,
			TraceID:   chatResult.TraceID,
			LatencyMs: chatResult.Duration.Milliseconds(),
			Code:      "invalid_response",
			Message:   "AI 服务返回了无法识别的推理结果",
		}
		return result
	}

	result.Inference = aiDiagnosticStage{
		OK:        true,
		Verified:  true,
		LatencyMs: chatResult.Duration.Milliseconds(),
		Attempts:  chatResult.Attempts,
		TraceID:   chatResult.TraceID,
		Message:   "模型推理正常",
	}
	result.LatencyMs = result.Transport.LatencyMs + result.Inference.LatencyMs
	result.OK = true
	return result
}

func hasChatCompletionChoice(body []byte) bool {
	var response struct {
		Choices []json.RawMessage `json:"choices"`
	}
	return json.Unmarshal(body, &response) == nil && len(response.Choices) > 0
}

func diagnosticStageFromError(err error) aiDiagnosticStage {
	stage := aiDiagnosticStage{
		Code:    string(aihttp.ErrorProvider),
		Message: "AI 服务请求失败",
	}
	var providerErr *aihttp.Error
	if !errors.As(err, &providerErr) {
		return stage
	}
	stage.Code = string(providerErr.Kind)
	stage.Attempts = providerErr.Attempts
	stage.TraceID = providerErr.TraceID
	stage.Message = aiDiagnosticErrorMessage(providerErr.Kind)
	return stage
}

func aiDiagnosticErrorMessage(kind aihttp.ErrorKind) string {
	switch kind {
	case aihttp.ErrorInvalidRequest:
		return "AI 请求参数无效"
	case aihttp.ErrorAuthentication:
		return "AI API Key 无效"
	case aihttp.ErrorPermission:
		return "当前账号无权访问该模型或服务"
	case aihttp.ErrorRateLimit:
		return "AI 请求达到限流，请稍后重试"
	case aihttp.ErrorOverloaded:
		return "AI 服务负载较高，请稍后重试"
	case aihttp.ErrorTimeout:
		return "AI 服务响应超时"
	case aihttp.ErrorCanceled:
		return "AI 检测已取消"
	case aihttp.ErrorNetwork:
		return "无法连接 AI 服务"
	default:
		return "AI 服务请求失败"
	}
}
