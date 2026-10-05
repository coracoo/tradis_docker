package api

import (
	"dockerpanel/backend/pkg/aihttp"
	"dockerpanel/backend/pkg/settings"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
)

func RegisterFreeAIRoutes(r *gin.RouterGroup) {
	group := r.Group("/ai")
	group.GET("/logs", listAILogs)
	group.POST("/navigation/enrich", enrichNavigationOnce)
	group.POST("/navigation/enrich-by-title", enrichNavigationByTitle)
	group.POST("/navigation/enrich-by-id", enrichNavigationByID)
	group.POST("/compose/generate", generateComposeYAML)
	group.POST("/test", testAIConnectivity)
	group.POST("/models", listAIModels)
}

type aiTestRequest struct {
	Enabled     *bool    `json:"enabled"`
	BaseUrl     *string  `json:"baseUrl"`
	ApiKey      *string  `json:"apiKey"`
	Model       *string  `json:"model"`
	Temperature *float64 `json:"temperature"`
}

type aiModelsRequest struct {
	BaseUrl *string `json:"baseUrl"`
	ApiKey  *string `json:"apiKey"`
}

func testAIConnectivity(c *gin.Context) {
	var req aiTestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "Invalid request", err)
		return
	}
	s, err := settings.GetSettings()
	if err != nil {
		respondError(c, http.StatusInternalServerError, "Failed to get settings", err)
		return
	}
	enabled := s.AiEnabled
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	if !enabled {
		respondError(c, http.StatusBadRequest, "AI is disabled", errors.New("aiEnabled=false"))
		return
	}
	baseURL := strings.TrimSpace(s.AiBaseUrl)
	if req.BaseUrl != nil {
		baseURL = strings.TrimSpace(*req.BaseUrl)
	}
	model := strings.TrimSpace(s.AiModel)
	if req.Model != nil {
		model = strings.TrimSpace(*req.Model)
	}
	apiKey := settings.GetAIAPIKey()
	if req.ApiKey != nil {
		apiKey = strings.TrimSpace(*req.ApiKey)
	}
	if baseURL == "" || model == "" || apiKey == "" {
		respondError(c, http.StatusBadRequest, "AI baseUrl/model/apiKey is required", nil)
		return
	}
	if _, err := aihttp.ChatCompletionsURL(baseURL); err != nil {
		respondError(c, http.StatusBadRequest, "AI baseUrl is invalid", err)
		return
	}
	temperature := s.AiTemperature
	if req.Temperature != nil {
		temperature = *req.Temperature
	}
	if temperature < 0 {
		temperature = 0
	}
	if temperature > 2 {
		temperature = 2
	}
	result := runAIDiagnostic(c.Request.Context(), aiDiagnosticConfig{
		BaseURL: baseURL, APIKey: apiKey, Model: model, Temperature: temperature,
	}, aihttp.New(aihttp.DiagnosticPolicy()), aihttp.New(aihttp.UtilityPolicy()))
	c.JSON(http.StatusOK, result)
}

func listAIModels(c *gin.Context) {
	var req aiModelsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "无效的请求", err)
		return
	}
	s, err := settings.GetSettings()
	if err != nil {
		respondError(c, http.StatusInternalServerError, "读取设置失败", err)
		return
	}
	baseURL := strings.TrimSpace(s.AiBaseUrl)
	if req.BaseUrl != nil {
		baseURL = strings.TrimSpace(*req.BaseUrl)
	}
	apiKey := settings.GetAIAPIKey()
	if req.ApiKey != nil {
		apiKey = strings.TrimSpace(*req.ApiKey)
	}
	if baseURL == "" || apiKey == "" {
		respondError(c, http.StatusBadRequest, "AI baseUrl/apiKey 不能为空", nil)
		return
	}
	result, err := aihttp.New(aihttp.DiagnosticPolicy()).Models(c.Request.Context(), baseURL, apiKey)
	if err != nil {
		respondAIModelsError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"models": parseAIModelIDs(result.Body)})
}

func respondAIModelsError(c *gin.Context, err error) {
	var providerErr *aihttp.Error
	if !errors.As(err, &providerErr) {
		respondError(c, http.StatusBadGateway, "AI 服务请求失败", nil)
		return
	}
	message := aiDiagnosticErrorMessage(providerErr.Kind)
	if providerErr.StatusCode == http.StatusNotFound || providerErr.StatusCode == http.StatusMethodNotAllowed {
		message = "AI 服务未提供模型列表接口"
	}
	respondError(c, http.StatusBadGateway, message, fmt.Errorf("kind=%s status=%d", providerErr.Kind, providerErr.StatusCode))
}

func parseAIModelIDs(body []byte) []string {
	var payload struct {
		Data []struct{ ID string `json:"id"` } `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return []string{}
	}
	seen := make(map[string]struct{}, len(payload.Data))
	models := make([]string, 0, len(payload.Data))
	for _, item := range payload.Data {
		id := strings.TrimSpace(item.ID)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		models = append(models, id)
	}
	sort.Strings(models)
	return models
}

func buildChatCompletionsEndpoint(raw string) (string, error) {
	return aihttp.ChatCompletionsURL(raw)
}
