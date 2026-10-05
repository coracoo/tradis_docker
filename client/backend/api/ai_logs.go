package api

import (
	"database/sql"
	"dockerpanel/backend/pkg/database"
	"dockerpanel/backend/pkg/settings"
	"dockerpanel/backend/pkg/system"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type aiLogItem struct {
	ID        int            `json:"id"`
	Scope     sql.NullString `json:"scope"`
	Level     sql.NullString `json:"level"`
	Message   sql.NullString `json:"message"`
	Details   sql.NullString `json:"details"`
	CreatedAt time.Time      `json:"created_at"`
}

func listAILogs(c *gin.Context) {
	limit := 200
	if v := strings.TrimSpace(c.Query("limit")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 1000 {
			limit = n
		}
	}

	db := database.GetDB()
	rows, err := db.Query("SELECT id, scope, level, message, details, created_at FROM ai_logs ORDER BY id DESC LIMIT ?", limit)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "Failed to query ai logs", err)
		return
	}
	defer rows.Close()

	items := make([]aiLogItem, 0, limit)
	for rows.Next() {
		var it aiLogItem
		if err := rows.Scan(&it.ID, &it.Scope, &it.Level, &it.Message, &it.Details, &it.CreatedAt); err != nil {
			respondError(c, http.StatusInternalServerError, "Failed to scan ai logs", err)
			return
		}
		items = append(items, it)
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func checkAIEnabled(c *gin.Context) bool {
	s, err := settings.GetSettings()
	if err != nil || !s.AiEnabled {
		respondError(c, http.StatusServiceUnavailable, "AI 未启用，请先在设置中开启并配置 AI", nil)
		return false
	}
	return true
}

func enrichNavigationOnce(c *gin.Context) {
	type reqBody struct {
		Limit   int  `json:"limit"`
		Force   bool `json:"force"`
		Cleanup bool `json:"cleanup"`
	}
	var req reqBody
	_ = c.ShouldBindJSON(&req)
	limit := req.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	log.Printf("[API] enrichNavigationOnce called: cleanup=%v, force=%v, limit=%d", req.Cleanup, req.Force, limit)

	go func() {
		if req.Cleanup {
			log.Printf("[API] Cleanup requested, rebuilding auto navigation...")
			system.RebuildAutoNavigationAll()
		}
		attempted := system.RunNavigationAIEnrich(limit, req.Force)
		log.Printf("[API] enrichNavigationOnce completed: attempted=%d", attempted)
	}()
	c.JSON(http.StatusAccepted, gin.H{"accepted": true, "force": req.Force, "cleanup": req.Cleanup})
}

func enrichNavigationByTitle(c *gin.Context) {
	if !checkAIEnabled(c) {
		return
	}

	type reqBody struct {
		Title string `json:"title"`
		Limit int    `json:"limit"`
		Force bool   `json:"force"`
	}
	var req reqBody
	_ = c.ShouldBindJSON(&req)
	title := strings.TrimSpace(req.Title)
	if title == "" {
		respondError(c, http.StatusBadRequest, "title is required", nil)
		return
	}
	limit := req.Limit
	if limit <= 0 || limit > 200 {
		limit = 20
	}
	go system.RunNavigationAIEnrichByTitle(title, limit, req.Force)
	c.JSON(http.StatusAccepted, gin.H{"accepted": true, "force": req.Force, "title": title})
}

func enrichNavigationByID(c *gin.Context) {
	if !checkAIEnabled(c) {
		return
	}

	type reqBody struct {
		NavID int  `json:"navId"`
		Force bool `json:"force"`
	}
	var req reqBody
	_ = c.ShouldBindJSON(&req)
	if req.NavID <= 0 {
		respondError(c, http.StatusBadRequest, "navId is required", nil)
		return
	}
	go system.RunNavigationAIEnrichByID(req.NavID, req.Force)
	c.JSON(http.StatusAccepted, gin.H{"accepted": true, "force": req.Force, "navId": req.NavID})
}
