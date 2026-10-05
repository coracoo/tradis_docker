package system

import (
	"context"
	"dockerpanel/backend/pkg/aihttp"
	"dockerpanel/backend/pkg/settings"
	"errors"
	"strings"
	"time"
)

var waitForNavigationAIBackfillForDiscovery = waitForNavigationAIBackfill
var runNavigationAIBackfillForDiscovery = RunNavigationAIBackfillContext
var runNavigationAIRequest = func(ctx context.Context, baseURL, apiKey string, payload any) (*aihttp.Result, error) {
	return aihttp.New(aihttp.UtilityPolicy()).ChatCompletions(ctx, baseURL, apiKey, payload)
}
var beforeNavigationAIUpdate = func(context.Context) {}
var scheduleNavigationAIEnrichment = func(navID int, labels map[string]string, image string, s settings.Settings, force bool) {
	go aiEnrichNavigationItemContext(withNavNotifySuppressed(context.Background()), navID, labels, image, s, force)
}

func editionRunNavigationAIBackfill(ctx context.Context, limit int) int {
	if isAutoAIEnrichSuppressed() || navAIEnrichBatchRunning.Load() {
		return 0
	}
	containerDiscoveryHooksMu.RLock()
	waitForBackfill := waitForNavigationAIBackfillForDiscovery
	containerDiscoveryHooksMu.RUnlock()
	if !waitForBackfill(ctx) || ctx.Err() != nil {
		return 0
	}
	containerDiscoveryHooksMu.RLock()
	runBackfill := runNavigationAIBackfillForDiscovery
	containerDiscoveryHooksMu.RUnlock()
	return runBackfill(ctx, limit)
}

func editionSuppressAutoNavigationAIEnrichFor(duration time.Duration) {
	suppressAutoAIEnrichFor(duration)
}

func editionScheduleNavigationAIEnrichment(navID int, labels map[string]string, image string, s settings.Settings, force bool) {
	if !navigationAIConfigurationFor(s).Enabled || isAutoAIEnrichSuppressed() || navAIEnrichBatchRunning.Load() {
		return
	}
	navigationAIHooksMu.RLock()
	schedule := scheduleNavigationAIEnrichment
	navigationAIHooksMu.RUnlock()
	schedule(navID, labels, image, s, force)
}

func editionRunNavigationAIRequest(ctx context.Context, baseURL, apiKey string, payload any) (*navigationAIResult, error) {
	navigationAIHooksMu.RLock()
	runRequest := runNavigationAIRequest
	navigationAIHooksMu.RUnlock()
	result, err := runRequest(ctx, baseURL, apiKey, payload)
	if err != nil || result == nil {
		return nil, err
	}
	return &navigationAIResult{
		Body:     result.Body,
		Duration: result.Duration,
		Attempts: result.Attempts,
		TraceID:  result.TraceID,
	}, nil
}

func editionNavigationAIEndpoint(baseURL string) string {
	endpoint, _ := aihttp.ChatCompletionsURL(baseURL)
	return endpoint
}

func editionNavigationAIErrorDetails(err error) map[string]any {
	var providerErr *aihttp.Error
	if !errors.As(err, &providerErr) {
		return nil
	}
	return map[string]any{
		"kind":       providerErr.Kind,
		"statusCode": providerErr.StatusCode,
		"attempts":   providerErr.Attempts,
		"traceId":    providerErr.TraceID,
	}
}

func editionBeforeNavigationAIUpdate(ctx context.Context) {
	navigationAIHooksMu.RLock()
	beforeUpdate := beforeNavigationAIUpdate
	navigationAIHooksMu.RUnlock()
	beforeUpdate(ctx)
}

func navigationAIConfigurationFor(s settings.Settings) navigationAIConfiguration {
	return navigationAIConfiguration{
		Enabled:             s.AiEnabled,
		APIKey:              settings.GetAIAPIKey(),
		BaseURL:             strings.TrimSpace(s.AiBaseUrl),
		Model:               settings.EffectiveAIUtilityModel(s),
		Temperature:         s.AiTemperature,
		AllowCreateCategory: s.AiAllowCreateCategory,
		NavigationPrompt:    s.AiNavigationPrompt,
	}
}
