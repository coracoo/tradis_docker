package system

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"dockerpanel/backend/pkg/database"
	"dockerpanel/backend/pkg/logging"
	"dockerpanel/backend/pkg/settings"
)

var navAIEnrichLocks sync.Map
var navAIEnrichBatchMu = newNavigationEnrichmentMutex()
var navAIEnrichBatchRunning atomic.Bool
var autoAIEnrichSuppressedUntil atomic.Int64

var navigationAIHooksMu sync.RWMutex

type navigationEnrichmentMutex struct {
	token chan struct{}
}

func newNavigationEnrichmentMutex() *navigationEnrichmentMutex {
	token := make(chan struct{}, 1)
	token <- struct{}{}
	return &navigationEnrichmentMutex{token: token}
}

func (m *navigationEnrichmentMutex) Lock(ctx context.Context) bool {
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-ctx.Done():
		return false
	case <-m.token:
		return true
	}
}

func (m *navigationEnrichmentMutex) Unlock() {
	select {
	case m.token <- struct{}{}:
	default:
		panic("navigation enrichment mutex: unlock of unlocked mutex")
	}
}

func suppressAutoAIEnrichFor(d time.Duration) {
	if d <= 0 {
		return
	}
	autoAIEnrichSuppressedUntil.Store(time.Now().Add(d).UnixNano())
}

func isAutoAIEnrichSuppressed() bool {
	return time.Now().UnixNano() < autoAIEnrichSuppressedUntil.Load()
}

func withNavAIEnrichLock(navID int, fn func()) {
	if navID <= 0 || fn == nil {
		return
	}
	muAny, _ := navAIEnrichLocks.LoadOrStore(navID, newNavigationEnrichmentMutex())
	mu := muAny.(*navigationEnrichmentMutex)
	if !mu.Lock(context.Background()) {
		return
	}
	defer mu.Unlock()
	fn()
}

func withNavAIEnrichLockContext(ctx context.Context, navID int, fn func()) {
	if navID <= 0 || fn == nil || ctx.Err() != nil {
		return
	}
	muAny, _ := navAIEnrichLocks.LoadOrStore(navID, newNavigationEnrichmentMutex())
	mu := muAny.(*navigationEnrichmentMutex)
	if !mu.Lock(ctx) {
		return
	}
	defer mu.Unlock()
	if ctx.Err() != nil {
		return
	}
	fn()
}

func waitForNavigationAIBackfill(ctx context.Context) bool {
	select {
	case <-time.After(5 * time.Second):
		return true
	case <-ctx.Done():
		return false
	}
}

func RunNavigationAIBackfill(limit int) int {
	return RunNavigationAIEnrich(limit, false)
}

func RunNavigationAIBackfillContext(ctx context.Context, limit int) int {
	return runNavigationAIEnrichContext(ctx, limit, false)
}

func RunNavigationAIEnrichByID(navID int, force bool) int {
	s, err := settings.GetSettings()
	if err != nil {
		return 0
	}
	if !navigationAIConfigurationFor(s).Enabled || navID <= 0 {
		return 0
	}

	if !navAIEnrichBatchMu.Lock(context.Background()) {
		return 0
	}
	navAIEnrichBatchRunning.Store(true)
	defer func() {
		navAIEnrichBatchRunning.Store(false)
		navAIEnrichBatchMu.Unlock()
	}()

	CleanupOrphanAutoNavigationNow()

	db := database.GetDB()
	var isDeleted int
	var aiGenerated int
	var title sql.NullString
	var category sql.NullString
	var icon sql.NullString
	if err := db.QueryRow("SELECT title, category, icon, is_deleted, ai_generated FROM navigation_items WHERE id = ?", navID).
		Scan(&title, &category, &icon, &isDeleted, &aiGenerated); err != nil {
		return 0
	}
	if isDeleted == 1 {
		return 0
	}

	if !force {
		if aiGenerated == 1 {
			return 0
		}
		needFill := strings.TrimSpace(title.String) == "" ||
			strings.TrimSpace(category.String) == "" || strings.TrimSpace(category.String) == "默认" || strings.TrimSpace(category.String) == "未分类" ||
			strings.TrimSpace(icon.String) == "" || strings.TrimSpace(icon.String) == "mdi-docker"
		if !needFill {
			return 0
		}
	}

	aiEnrichNavigationItem(navID, nil, "", s, force)
	return 1
}

func RunNavigationAIEnrichByTitle(title string, limit int, force bool) int {
	s, err := settings.GetSettings()
	if err != nil {
		return 0
	}
	if !navigationAIConfigurationFor(s).Enabled {
		return 0
	}
	if !navAIEnrichBatchMu.Lock(context.Background()) {
		return 0
	}
	navAIEnrichBatchRunning.Store(true)
	defer func() {
		navAIEnrichBatchRunning.Store(false)
		navAIEnrichBatchMu.Unlock()
	}()
	CleanupOrphanAutoNavigationNow()
	title = strings.TrimSpace(title)
	if title == "" {
		return 0
	}
	if limit <= 0 || limit > 200 {
		limit = 20
	}

	db := database.GetDB()
	query := "SELECT id FROM navigation_items WHERE is_deleted = 0 AND title LIKE ?"
	args := []any{"%" + title + "%"}
	if !force {
		query += " AND ai_generated = 0"
		query += " AND (trim(title) = '' OR trim(category) = '' OR category = '默认' OR category = '未分类' OR trim(icon) = '' OR icon = 'mdi-docker')"
	}
	query += " ORDER BY updated_at DESC LIMIT ?"
	args = append(args, limit)
	rows, err := db.Query(query, args...)
	if err != nil {
		return 0
	}
	defer rows.Close()

	ids := make([]int, 0, limit)
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err == nil && id > 0 {
			ids = append(ids, id)
		}
	}
	// 批量识别只发一条汇总通知；逐项明细仍写入 ai_logs。
	stats := &aiEnrichOutcomeStats{}
	suppressedCtx := withNavEnrichStats(withNavNotifySuppressed(context.Background()), stats)
	enrichStarted := time.Now()
	for _, id := range ids {
		aiEnrichNavigationItemContext(suppressedCtx, id, nil, "", s, force)
	}
	if stats.started > 0 {
		appendAiLogContext(context.Background(), "navigation", "info", "ai_enrich_batch_done", map[string]any{
			"started":    stats.started,
			"updated":    stats.updated,
			"failed":     stats.failed,
			"durationMs": time.Since(enrichStarted).Milliseconds(),
		})
	}
	return len(ids)
}

func RunNavigationAIEnrich(limit int, force bool) int {
	return runNavigationAIEnrichContext(context.Background(), limit, force)
}

func RunNavigationAIEnrichContext(ctx context.Context, limit int, force bool) int {
	return runNavigationAIEnrichContext(ctx, limit, force)
}

func runNavigationAIEnrichContext(ctx context.Context, limit int, force bool) int {
	if ctx == nil {
		ctx = context.Background()
	}
	if ctx.Err() != nil {
		return 0
	}
	logging.Debug("navigation AI enrichment started", "limit", limit, "force", force)
	s, err := settings.GetSettings()
	if err != nil {
		logging.Warn("navigation AI enrichment could not load settings", "error", err)
		return 0
	}
	if ctx.Err() != nil {
		return 0
	}
	if !navigationAIConfigurationFor(s).Enabled {
		logging.Debug("navigation AI enrichment skipped because AI is disabled")
		return 0
	}
	if ctx.Err() != nil {
		return 0
	}
	logging.Debug("navigation AI enrichment is enabled")

	if !navAIEnrichBatchMu.Lock(ctx) {
		return 0
	}
	navAIEnrichBatchRunning.Store(true)
	defer func() {
		navAIEnrichBatchRunning.Store(false)
		navAIEnrichBatchMu.Unlock()
	}()
	CleanupOrphanAutoNavigationNowContext(ctx)
	if ctx.Err() != nil {
		return 0
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	db := database.GetDB()
	if db == nil || ctx.Err() != nil {
		return 0
	}
	query := "SELECT id FROM navigation_items WHERE is_deleted = 0"
	args := []any{}
	if !force {
		query += " AND ai_generated = 0"
		query += " AND (trim(title) = '' OR trim(category) = '' OR category = '默认' OR category = '未分类' OR trim(icon) = '' OR icon = 'mdi-docker')"
	}
	query += " ORDER BY updated_at DESC LIMIT ?"
	args = append(args, limit)

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		logging.Warn("navigation AI enrichment query failed", "error", err)
		return 0
	}
	defer rows.Close()

	ids := make([]int, 0, limit)
	for rows.Next() {
		if ctx.Err() != nil {
			return 0
		}
		var id int
		if err := rows.Scan(&id); err == nil && id > 0 {
			ids = append(ids, id)
		}
	}

	logging.Debug("navigation AI enrichment selected items", "count", len(ids))

	// 批量识别只发一条汇总通知；逐项明细仍写入 ai_logs（与镜像更新批次汇总一致）。
	stats := &aiEnrichOutcomeStats{}
	suppressedCtx := withNavEnrichStats(withNavNotifySuppressed(ctx), stats)
	enrichStarted := time.Now()
	for _, id := range ids {
		if ctx.Err() != nil {
			return 0
		}
		aiEnrichNavigationItemContext(suppressedCtx, id, nil, "", s, force)
	}
	if stats.started > 0 {
		appendAiLogContext(ctx, "navigation", "info", "ai_enrich_batch_done", map[string]any{
			"started":    stats.started,
			"updated":    stats.updated,
			"failed":     stats.failed,
			"durationMs": time.Since(enrichStarted).Milliseconds(),
		})
	}
	logging.Debug("navigation AI enrichment completed", "count", len(ids))
	return len(ids)
}

func aiEnrichNavigationItem(navID int, labels map[string]string, image string, s settings.Settings, force bool) {
	aiEnrichNavigationItemWithContext(context.Background(), navID, labels, image, s, force)
}

func aiEnrichNavigationItemContext(ctx context.Context, navID int, labels map[string]string, image string, s settings.Settings, force bool) {
	aiEnrichNavigationItemWithContext(ctx, navID, labels, image, s, force)
}

func aiEnrichNavigationItemWithContext(ctx context.Context, navID int, labels map[string]string, image string, s settings.Settings, force bool) {
	if ctx == nil {
		ctx = context.Background()
	}
	if ctx.Err() != nil {
		return
	}
	run := func() {
		if ctx.Err() != nil {
			return
		}
		configuration := navigationAIConfigurationFor(s)
		apiKey := configuration.APIKey
		if ctx.Err() != nil {
			return
		}
		if apiKey == "" {
			return
		}
		baseUrl := configuration.BaseURL
		model := configuration.Model
		if baseUrl == "" || model == "" {
			return
		}

		db := database.GetDB()
		if db == nil || ctx.Err() != nil {
			return
		}

		var title sql.NullString
		var lanUrl sql.NullString
		var wanUrl sql.NullString
		var icon sql.NullString
		var category sql.NullString
		var containerID sql.NullString
		var isAuto int
		var isDeleted int
		var aiGenerated int

		if err := db.QueryRowContext(ctx, "SELECT title, lan_url, wan_url, icon, category, container_id, is_auto, is_deleted, ai_generated FROM navigation_items WHERE id = ?", navID).
			Scan(&title, &lanUrl, &wanUrl, &icon, &category, &containerID, &isAuto, &isDeleted, &aiGenerated); err != nil {
			return
		}
		if isDeleted == 1 || (aiGenerated == 1 && !force) {
			return
		}

		if !force {
			needFill := strings.TrimSpace(title.String) == "" ||
				strings.TrimSpace(category.String) == "" || strings.TrimSpace(category.String) == "默认" || strings.TrimSpace(category.String) == "未分类" ||
				strings.TrimSpace(icon.String) == "" || strings.TrimSpace(icon.String) == "mdi-docker"
			if !needFill {
				return
			}
		}

		if force {
			appendAiLogContext(ctx, "navigation", "info", "ai_enrich_force_mode", map[string]any{"navId": navID})
		}

		if labels == nil {
			labels = map[string]string{}
		}

		currentTitle := strings.TrimSpace(title.String)
		currentCategory := strings.TrimSpace(category.String)
		currentIcon := strings.TrimSpace(icon.String)
		if currentTitle == "" {
			currentTitle = ""
		}
		if currentCategory == "" {
			currentCategory = ""
		}
		if currentIcon == "" {
			currentIcon = ""
		}

		if currentIcon != "" && strings.HasPrefix(currentIcon, "clay:") {
			currentIcon = normalizeAIIconValue(currentIcon)
		}

		if strings.TrimSpace(currentTitle) == "" && strings.TrimSpace(currentCategory) == "" && strings.TrimSpace(currentIcon) == "" && !force {
			return
		}

		labelPairs := make([]string, 0, len(labels))
		for k, v := range labels {
			k = strings.TrimSpace(k)
			if k == "" {
				continue
			}
			if strings.HasPrefix(strings.ToLower(k), "com.docker.") {
				labelPairs = append(labelPairs, k+"="+strings.TrimSpace(v))
			}
		}
		sort.Strings(labelPairs)
		if len(labelPairs) > 25 {
			labelPairs = labelPairs[:25]
		}

		categoryCandidates := make([]string, 0, 12)
		{
			if ctx.Err() != nil {
				return
			}
			rows, err := db.QueryContext(ctx, "SELECT category, COUNT(*) AS c FROM navigation_items WHERE is_deleted = 0 AND trim(category) != '' AND category != '默认' AND category != '未分类' GROUP BY category ORDER BY c DESC LIMIT 12")
			if err == nil {
				defer rows.Close()
				for rows.Next() {
					var cat string
					var cnt int
					if err := rows.Scan(&cat, &cnt); err != nil {
						continue
					}
					cat = strings.TrimSpace(cat)
					if cat == "" {
						continue
					}
					categoryCandidates = append(categoryCandidates, cat)
				}
			}
		}
		if len(categoryCandidates) == 0 {
			categoryCandidates = []string{"工具", "生产力", "开发", "数据库", "存储", "网络", "监控", "安全", "自动化", "AI工具", "多媒体", "未分类"}
		}

		faviconIcon := resolveFaviconIconContext(ctx, strings.TrimSpace(lanUrl.String), strings.TrimSpace(wanUrl.String))
		normalizedTitle := normalizeNavigationTitle(currentTitle)
		if normalizedTitle == "" {
			normalizedTitle = simplifyImageTitle(image)
		}

		userContent := map[string]any{
			"navId":               navID,
			"containerId":         strings.TrimSpace(containerID.String),
			"title":               currentTitle,
			"normalizedTitle":     normalizedTitle,
			"category":            currentCategory,
			"icon":                currentIcon,
			"image":               strings.TrimSpace(image),
			"imageName":           simplifyImageTitle(image),
			"lanUrl":              strings.TrimSpace(lanUrl.String),
			"wanUrl":              strings.TrimSpace(wanUrl.String),
			"labels":              labelPairs,
			"categoryCandidates":  categoryCandidates,
			"allowCreateCategory": configuration.AllowCreateCategory,
			"faviconIcon":         faviconIcon,
			"isAuto":              isAuto == 1,
			"force":               force,
		}
		userBytes, _ := json.Marshal(userContent)

		systemPrompt := strings.TrimSpace(configuration.NavigationPrompt)
		if systemPrompt == "" {
			systemPrompt = "你是一个导航整理助手。你必须只输出严格 JSON：{\"title\":\"\",\"category\":\"\",\"icon\":\"\"}。title 与 category 必须非空。2.title 注意精简，如 **{name}-{name}-{number}** 的名字必须简化为 {name}；3.category 必须给出具体中文分类，仅当完全无法判断时输出**未分类**；4.icon 必须是 http(s) 图标 URL 或 lucide:app-window。5.不要输出解释、推理过程、Markdown、代码块或额外字段。"
		}
		systemPrompt += "\n补充：user JSON 里可能给了 categoryCandidates（分类候选）与 faviconIcon（可用的图标 URL）。如没有更合适的官方图标，可直接用 faviconIcon 作为 icon。"
		if configuration.AllowCreateCategory {
			systemPrompt += "\n分类规则：允许根据应用用途创建 categoryCandidates 之外的新中文分类；分类名应简短、稳定、可复用，避免为单个应用创建过细分类。"
		} else {
			systemPrompt += "\n分类规则：禁止创建新分类，category 必须严格从 categoryCandidates 中选择。"
		}
		systemPrompt += "\n只输出 JSON，不要包含任何其他文本。"

		payload := map[string]any{
			"model": model,
			"messages": []map[string]string{
				{"role": "system", "content": systemPrompt},
				{"role": "user", "content": string(userBytes)},
			},
			"temperature": configuration.Temperature,
			"max_tokens":  800,
		}
		endpoint := editionNavigationAIEndpoint(baseUrl)
		appendAiLogContext(ctx, "navigation", "info", "ai_enrich_start", map[string]any{
			"navId":       navID,
			"containerId": strings.TrimSpace(containerID.String),
			"endpoint":    endpoint,
			"model":       model,
		})

		aiResult, err := editionRunNavigationAIRequest(ctx, baseUrl, apiKey, payload)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			details := map[string]any{"navId": navID, "error": err.Error()}
			for key, value := range editionNavigationAIErrorDetails(err) {
				details[key] = value
			}
			appendAiLogContext(ctx, "navigation", "error", "ai_enrich_request_failed", details)
			return
		}

		if ctx.Err() != nil {
			return
		}
		respBody := aiResult.Body

		var decoded struct {
			Choices []struct {
				Message struct {
					Content          json.RawMessage `json:"content"`
					ReasoningContent json.RawMessage `json:"reasoning_content"`
				} `json:"message"`
			} `json:"choices"`
		}
		if err := json.Unmarshal(respBody, &decoded); err != nil {
			appendAiLogContext(ctx, "navigation", "error", "ai_enrich_decode_failed", map[string]any{
				"navId":       navID,
				"respSnippet": compactSnippet(respBody, 2048),
			})
			return
		}
		if len(decoded.Choices) == 0 {
			appendAiLogContext(ctx, "navigation", "error", "ai_enrich_empty_choices", map[string]any{
				"navId":       navID,
				"respSnippet": compactSnippet(respBody, 2048),
			})
			return
		}

		content := strings.TrimSpace(extractContentString(decoded.Choices[0].Message.Content))
		content = strings.TrimPrefix(content, "```json")
		content = strings.TrimPrefix(content, "```")
		content = strings.TrimSuffix(content, "```")
		content = strings.TrimSpace(content)
		jsonContent := extractJSONObjectFromText(content)
		if jsonContent == "" {
			appendAiLogContext(ctx, "navigation", "error", "ai_enrich_empty_content", map[string]any{
				"navId":       navID,
				"respSnippet": compactSnippet(respBody, 2048),
			})
			return
		}

		var raw map[string]any
		if err := json.Unmarshal([]byte(jsonContent), &raw); err != nil {
			appendAiLogContext(ctx, "navigation", "error", "ai_enrich_parse_failed", map[string]any{
				"navId":   navID,
				"content": content,
			})
			return
		}

		for k := range raw {
			if k != "title" && k != "category" && k != "icon" {
				delete(raw, k)
			}
		}
		titleStr, _ := raw["title"].(string)
		categoryStr, _ := raw["category"].(string)
		iconStr, _ := raw["icon"].(string)

		outTitle := normalizeNavigationTitle(normalizeDuplicatePairTitle(strings.TrimSpace(titleStr)))
		if outTitle == "" {
			outTitle = normalizedTitle
		}
		outCategory := strings.TrimSpace(categoryStr)
		if strings.EqualFold(outCategory, "default") || outCategory == "默认" {
			outCategory = "未分类"
		}
		if outCategory == "" {
			outCategory = currentCategory
		}
		if outCategory == "" {
			outCategory = "未分类"
		}
		if !configuration.AllowCreateCategory {
			allowed := false
			for _, candidate := range categoryCandidates {
				if outCategory == candidate {
					allowed = true
					break
				}
			}
			if !allowed {
				outCategory = currentCategory
				if outCategory == "" || outCategory == "默认" {
					outCategory = "未分类"
				}
			}
		}
		outIcon := normalizeAIIconValue(strings.TrimSpace(iconStr))
		outIcon = strings.Trim(outIcon, "`")
		if strings.HasPrefix(outIcon, "/icons/ray") {
			outIcon = ""
		}
		if faviconIcon != "" {
			outIcon = faviconIcon
		}
		if outTitle == "" || outCategory == "" {
			appendAiLogContext(ctx, "navigation", "error", "ai_enrich_empty_result", map[string]any{"navId": navID})
			return
		}
		if outIcon == "" {
			outIcon = "mdi-docker"
		}
		if !isAllowedNavigationIconValue(outIcon) {
			outIcon = "mdi-docker"
		}
		outIcon = normalizeAndResolveNavigationIconContext(ctx, outIcon, strings.TrimSpace(lanUrl.String), strings.TrimSpace(wanUrl.String))
		outIcon = cacheNavigationIcon(outIcon, strings.TrimSpace(containerID.String))
		if ctx.Err() != nil {
			return
		}

		updateSQL := ""
		if force {
			updateSQL = "UPDATE navigation_items SET title = COALESCE(NULLIF(?, ''), title), category = COALESCE(NULLIF(?, ''), category), icon = COALESCE(NULLIF(?, ''), icon), ai_generated = 1, updated_at = CURRENT_TIMESTAMP WHERE id = ? AND is_deleted = 0"
		} else {
			updateSQL = "UPDATE navigation_items SET title = CASE WHEN trim(title) = '' THEN COALESCE(NULLIF(?, ''), title) ELSE title END, category = CASE WHEN trim(category) = '' OR category = '默认' OR category = '未分类' THEN COALESCE(NULLIF(?, ''), category) ELSE category END, icon = CASE WHEN trim(icon) = '' OR icon = 'mdi-docker' THEN COALESCE(NULLIF(?, ''), icon) ELSE icon END, ai_generated = 1, updated_at = CURRENT_TIMESTAMP WHERE id = ? AND is_deleted = 0 AND ai_generated = 0"
		}
		editionBeforeNavigationAIUpdate(ctx)
		if ctx.Err() != nil {
			return
		}
		result, _ := db.ExecContext(ctx, updateSQL, outTitle, outCategory, outIcon, navID)
		affected := int64(0)
		if result != nil {
			affected, _ = result.RowsAffected()
		}
		appendAiLogContext(ctx, "navigation", "info", "ai_enrich_done", map[string]any{
			"navId":     navID,
			"latencyMs": aiResult.Duration.Milliseconds(),
			"attempts":  aiResult.Attempts,
			"traceId":   aiResult.TraceID,
			"updated":   affected > 0,
			"title":     outTitle,
			"category":  outCategory,
			"icon":      outIcon,
		})
	}
	withNavAIEnrichLockContext(ctx, navID, run)
}

func normalizeDuplicatePairTitle(title string) string {
	t := strings.TrimSpace(title)
	if t == "" {
		return ""
	}
	parts := strings.Split(t, "-")
	if len(parts) == 2 {
		a := strings.TrimSpace(parts[0])
		b := strings.TrimSpace(parts[1])
		if a != "" && a == b {
			return a
		}
	}
	return t
}

func simplifyImageTitle(image string) string {
	image = strings.TrimSpace(image)
	if image == "" {
		return ""
	}
	if idx := strings.LastIndex(image, "/"); idx >= 0 {
		image = image[idx+1:]
	}
	if idx := strings.Index(image, "@"); idx >= 0 {
		image = image[:idx]
	}
	if idx := strings.Index(image, ":"); idx >= 0 {
		image = image[:idx]
	}
	return normalizeNavigationTitle(image)
}

func listClayIconValues(limit int) []string {
	dir := ""
	for _, cand := range []string{
		filepath.Join(".", "dist", "icons", "clay"),
		filepath.Join(".", "icons", "clay"),
		filepath.Join("..", "frontend", "public", "icons", "clay"),
	} {
		if st, err := os.Stat(cand); err == nil && st.IsDir() {
			dir = cand
			break
		}
	}
	if dir == "" {
		return nil
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	items := make([]string, 0, len(entries))
	for _, ent := range entries {
		if ent.IsDir() {
			continue
		}
		name := ent.Name()
		ext := strings.ToLower(filepath.Ext(name))
		switch ext {
		case ".png", ".jpg", ".jpeg", ".webp", ".gif", ".svg", ".ico", ".avif", ".bmp", ".tif", ".tiff":
		default:
			continue
		}
		items = append(items, "/icons/clay/"+name)
	}

	sort.Strings(items)
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	return items
}

func normalizeAIIconValue(raw string) string {
	v := strings.TrimSpace(raw)
	if v == "" {
		return ""
	}
	if strings.HasPrefix(v, "clay:") {
		name := strings.TrimSpace(strings.TrimPrefix(v, "clay:"))
		if name == "" {
			return ""
		}
		if strings.Contains(name, ".") {
			return "/icons/clay/" + name
		}
		return "/icons/clay/" + name + ".png"
	}
	return v
}

// navNotifySuppressKey：标记导航识别处于批量/自动场景，抑制逐条通知（ai_logs 明细仍保留）。
type navNotifySuppressKey struct{}

// navEnrichStatsKey：携带批量识别的结果统计，供 appendAiLogContext 在写日志时累计。
type navEnrichStatsKey struct{}

type aiEnrichOutcomeStats struct {
	started int
	updated int
	failed  int
}

func withNavNotifySuppressed(ctx context.Context) context.Context {
	return context.WithValue(ctx, navNotifySuppressKey{}, true)
}

func navNotifySuppressed(ctx context.Context) bool {
	suppressed, _ := ctx.Value(navNotifySuppressKey{}).(bool)
	return suppressed
}

func withNavEnrichStats(ctx context.Context, stats *aiEnrichOutcomeStats) context.Context {
	return context.WithValue(ctx, navEnrichStatsKey{}, stats)
}

func navEnrichStats(ctx context.Context) *aiEnrichOutcomeStats {
	stats, _ := ctx.Value(navEnrichStatsKey{}).(*aiEnrichOutcomeStats)
	return stats
}

func recordEnrichOutcome(ctx context.Context, scope string, level string, message string, details map[string]any) {
	if scope != "navigation" {
		return
	}
	stats := navEnrichStats(ctx)
	if stats == nil {
		return
	}
	switch {
	case message == "ai_enrich_start":
		stats.started++
	case level == "error":
		stats.failed++
	case message == "ai_enrich_done" && getBoolFromAny(details, "updated"):
		stats.updated++
	}
}

func appendAiLog(scope string, level string, message string, details map[string]any) {
	if strings.TrimSpace(message) == "" {
		return
	}
	detailStr := ""
	if details != nil {
		if b, err := json.Marshal(details); err == nil {
			detailStr = string(b)
		}
	}
	_ = database.AppendAILog(scope, level, message, detailStr)
}

func notifyAiLog(scope string, level string, message string, details map[string]any) {
	notifyType, notifyMsg, ok := aiLogToNotification(scope, level, message, details)
	if ok {
		_ = database.SaveNotification(&database.Notification{
			Type:     notifyType,
			Category: "navigation_task",
			Message:  notifyMsg,
			Read:     false,
		})
	}
}

func appendAiLogContext(ctx context.Context, scope string, level string, message string, details map[string]any) {
	if ctx == nil {
		ctx = context.Background()
	}
	if ctx.Err() != nil {
		return
	}
	appendAiLog(scope, level, message, details)
	recordEnrichOutcome(ctx, scope, level, message, details)
	if navNotifySuppressed(ctx) {
		return
	}
	notifyAiLog(scope, level, message, details)
	if ctx.Err() != nil {
		return
	}
}

func aiLogToNotification(scope string, level string, message string, details map[string]any) (string, string, bool) {
	scope = strings.TrimSpace(scope)
	level = strings.TrimSpace(level)
	message = strings.TrimSpace(message)

	if scope != "navigation" {
		return "", "", false
	}

	navID := getIntFromAny(details, "navId")

	// 批量识别汇总：一批只发一条通知（逐项明细保留在 ai_logs）
	if message == "ai_enrich_batch_done" {
		updated := getIntFromAny(details, "updated")
		failed := getIntFromAny(details, "failed")
		msg := "AI 导航识别完成：成功 " + strconv.Itoa(updated) + " 个"
		if failed > 0 {
			msg += "，失败 " + strconv.Itoa(failed) + " 个"
		}
		if failed > 0 {
			return "warning", msg, true
		}
		return "success", msg, true
	}

	if level == "error" {
		msg := "AI 导航识别失败"
		if navID > 0 {
			msg += "（navId=" + strconv.Itoa(navID) + "）"
		}
		msg += "：" + message
		if snippet := getStringFromAny(details, "respSnippet"); snippet != "" {
			msg += " | " + snippet
		}
		if st := getStringFromAny(details, "status"); st != "" {
			msg += " | " + st
		}
		return "error", msg, true
	}

	if message == "ai_enrich_done" && getBoolFromAny(details, "updated") {
		msg := "AI 导航识别完成"
		if navID > 0 {
			msg += "（navId=" + strconv.Itoa(navID) + "）"
		}
		return "success", msg, true
	}

	return "", "", false
}

func getIntFromAny(m map[string]any, key string) int {
	if m == nil {
		return 0
	}
	v, ok := m[key]
	if !ok || v == nil {
		return 0
	}
	switch t := v.(type) {
	case int:
		return t
	case int64:
		return int(t)
	case float64:
		return int(t)
	case string:
		n, _ := strconv.Atoi(strings.TrimSpace(t))
		return n
	default:
		return 0
	}
}

func getBoolFromAny(m map[string]any, key string) bool {
	if m == nil {
		return false
	}
	v, ok := m[key]
	if !ok || v == nil {
		return false
	}
	switch t := v.(type) {
	case bool:
		return t
	case string:
		s := strings.TrimSpace(strings.ToLower(t))
		return s == "true" || s == "1" || s == "yes"
	case float64:
		return t != 0
	default:
		return false
	}
}

func getStringFromAny(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	default:
		b, err := json.Marshal(t)
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(b))
	}
}

func compactSnippet(b []byte, limit int) string {
	if limit <= 0 {
		limit = 2048
	}
	if len(b) > limit {
		b = b[:limit]
	}
	s := strings.TrimSpace(string(b))
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}
	if len(s) > 512 {
		s = s[:512]
	}
	return s
}

func extractContentString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	if string(raw) == "null" {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &parts); err == nil {
		buf := strings.Builder{}
		for _, p := range parts {
			if strings.TrimSpace(p.Text) == "" {
				continue
			}
			if buf.Len() > 0 {
				buf.WriteString("\n")
			}
			buf.WriteString(p.Text)
		}
		return buf.String()
	}
	return ""
}

func extractJSONObjectFromText(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}

	lastValid := ""
	for start := 0; start < len(s); start++ {
		if s[start] != '{' {
			continue
		}
		depth := 0
		inString := false
		escaped := false
		for end := start; end < len(s); end++ {
			ch := s[end]
			if inString {
				if escaped {
					escaped = false
					continue
				}
				if ch == '\\' {
					escaped = true
					continue
				}
				if ch == '"' {
					inString = false
				}
				continue
			}
			switch ch {
			case '"':
				inString = true
			case '{':
				depth++
			case '}':
				depth--
				if depth == 0 {
					candidate := strings.TrimSpace(s[start : end+1])
					var object map[string]any
					if json.Unmarshal([]byte(candidate), &object) == nil {
						lastValid = candidate
					}
					end = len(s)
				}
			}
		}
	}
	return lastValid
}
