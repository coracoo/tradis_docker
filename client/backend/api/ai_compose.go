package api

import (
	"dockerpanel/backend/pkg/aihttp"
	"dockerpanel/backend/pkg/settings"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

type aiComposeGenerateRequest struct {
	Prompt          string `json:"prompt"`
	ExistingCompose string `json:"existingCompose"`
	ExistingDotenv  string `json:"existingDotenv"`
}

func generateComposeYAML(c *gin.Context) {
	var req aiComposeGenerateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "Invalid request", err)
		return
	}
	prompt := strings.TrimSpace(req.Prompt)
	if prompt == "" {
		respondError(c, http.StatusBadRequest, "prompt is required", errors.New("empty prompt"))
		return
	}

	s, err := settings.GetSettings()
	if err != nil {
		respondError(c, http.StatusInternalServerError, "Failed to get settings", err)
		return
	}
	if !s.AiEnabled {
		respondError(c, http.StatusBadRequest, "AI is disabled", errors.New("aiEnabled=false"))
		return
	}

	baseUrl := strings.TrimSpace(s.AiBaseUrl)
	model := strings.TrimSpace(s.AiModel)
	if baseUrl == "" || model == "" {
		respondError(c, http.StatusBadRequest, "AI baseUrl/model is required", errors.New("empty baseUrl/model"))
		return
	}
	apiKey := settings.GetAIAPIKey()
	if apiKey == "" {
		respondError(c, http.StatusBadRequest, "AI apiKey is required", errors.New("empty apiKey"))
		return
	}

	if _, err := buildChatCompletionsEndpoint(baseUrl); err != nil {
		respondError(c, http.StatusBadRequest, "AI baseUrl is invalid", err)
		return
	}

	systemPrompt := strings.TrimSpace(s.AiComposePrompt)
	if systemPrompt == "" {
		systemPrompt = "你是一个 Docker Compose 编排助手。"
	}
	systemPrompt = systemPrompt + "\n你必须只输出严格 JSON：{\"composeYaml\":\"\",\"dotenvText\":\"\",\"notes\":[],\"warnings\":[]}。不要输出解释、推理过程、Markdown、代码块或额外字段。composeYaml 必须是可用的 docker compose YAML（包含 services）。dotenvText 为 .env 文本（可为空）。"

	userPayload := map[string]any{
		"requirements":    prompt,
		"existingCompose": strings.TrimSpace(req.ExistingCompose),
		"existingDotenv":  strings.TrimSpace(req.ExistingDotenv),
	}
	userBytes, _ := json.Marshal(userPayload)

	payload := map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": string(userBytes)},
		},
		"temperature": s.AiTemperature,
		"max_tokens":  1800,
	}
	result, err := aihttp.New(aihttp.UtilityPolicy()).ChatCompletions(
		c.Request.Context(), baseUrl, apiKey, payload,
	)
	if err != nil {
		respondError(c, http.StatusBadGateway, "AI request failed", err)
		return
	}

	content, err := extractChatCompletionContent(result.Body)
	if err != nil {
		respondError(c, http.StatusBadGateway, "AI response invalid", err)
		return
	}

	var raw map[string]any
	if err := json.Unmarshal([]byte(content), &raw); err != nil {
		respondError(c, http.StatusBadGateway, "AI response invalid", err)
		return
	}
	for k := range raw {
		if k != "composeYaml" && k != "dotenvText" && k != "notes" && k != "warnings" {
			respondError(c, http.StatusBadGateway, "AI response invalid", errors.New("unexpected field"))
			return
		}
	}

	composeYaml, _ := raw["composeYaml"].(string)
	dotenvText, _ := raw["dotenvText"].(string)
	notes := anyToStringSlice(raw["notes"])
	warnings := anyToStringSlice(raw["warnings"])

	composeYaml = strings.TrimSpace(composeYaml)
	if composeYaml == "" || !strings.Contains(composeYaml, "services:") {
		respondError(c, http.StatusBadGateway, "AI response invalid", errors.New("composeYaml is empty or missing services"))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"composeYaml": strings.TrimRight(composeYaml, "\n") + "\n",
		"dotenvText":  strings.TrimRight(dotenvText, "\n") + "\n",
		"notes":       notes,
		"warnings":    warnings,
		"latencyMs":   result.Duration.Milliseconds(),
	})
}

func extractChatCompletionContent(respBody []byte) (string, error) {
	var decoded struct {
		Choices []struct {
			Message struct {
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &decoded); err != nil {
		return "", err
	}
	if len(decoded.Choices) == 0 {
		return "", errors.New("empty choices")
	}
	var s string
	if err := json.Unmarshal(decoded.Choices[0].Message.Content, &s); err != nil {
		s = strings.TrimSpace(string(decoded.Choices[0].Message.Content))
	}
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "{") || !strings.HasSuffix(s, "}") {
		return "", errors.New("content is not json object")
	}
	return s, nil
}

func anyToStringSlice(v any) []string {
	arr, ok := v.([]any)
	if !ok || len(arr) == 0 {
		return []string{}
	}
	out := make([]string, 0, len(arr))
	for _, it := range arr {
		s, ok := it.(string)
		if !ok {
			continue
		}
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		out = append(out, s)
	}
	return out
}
