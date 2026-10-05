package api

import (
	"database/sql"
	"dockerpanel/backend/pkg/database"
	"dockerpanel/backend/pkg/settings"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type NavigationItem struct {
	ID           int       `json:"id"`
	Title        string    `json:"title"`
	URL          string    `json:"url"` // Deprecated: Use LanUrl or WanUrl
	LanUrl       string    `json:"lan_url"`
	WanUrl       string    `json:"wan_url"`
	IconUrl      string    `json:"icon_url"`
	IconPath     string    `json:"icon_path,omitempty"`
	Category     string    `json:"category"`
	IsAuto       bool      `json:"is_auto"`
	AiGenerated  bool      `json:"ai_generated"`
	IsDeleted    bool      `json:"is_deleted"`
	HiddenReason string    `json:"hidden_reason,omitempty"`
	ContainerID  string    `json:"container_id,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type CreateNavigationRequest struct {
	Title    string `json:"title" binding:"required"`
	URL      string `json:"url"` // Optional now
	LanUrl   string `json:"lan_url"`
	WanUrl   string `json:"wan_url"`
	IconUrl  string `json:"icon_url"`
	Category string `json:"category"`
}

type UpdateNavigationRequest struct {
	Title    string `json:"title"`
	URL      string `json:"url"`
	LanUrl   string `json:"lan_url"`
	WanUrl   string `json:"wan_url"`
	IconUrl  string `json:"icon_url"`
	IconPath string `json:"icon_path"`
	Category string `json:"category"`
}

func RegisterNavigationRoutes(r *gin.RouterGroup) {
	nav := r.Group("/navigation")
	{
		nav.GET("", listNavigationItems)
		nav.POST("", createNavigationItem)
		nav.PUT("/:id", updateNavigationItem)
		nav.DELETE("/:id", deleteNavigationItem)
		nav.POST("/:id/restore", restoreNavigationItem)
		nav.POST("/:id/icon", uploadNavigationIcon)
	}
}

// @Summary 获取导航项列表
// @Description 获取所有导航项，支持按分类分组
// @Tags navigation
// @Accept json
// @Produce json
// @Success 200 {array} NavigationItem
func listNavigationItems(c *gin.Context) {
	db := database.GetDB()

	includeDeleted := c.Query("include_deleted") == "true"
	query := "SELECT id, title, url, lan_url, wan_url, icon, icon_path, category, is_auto, ai_generated, is_deleted, COALESCE(hidden_reason, ''), container_id, created_at, updated_at FROM navigation_items"
	if !includeDeleted {
		query += " WHERE is_deleted = 0"
	}
	query += " ORDER BY category, title"

	rows, err := db.Query(query)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "获取导航项失败", err)
		return
	}
	defer rows.Close()

	var items []NavigationItem
	for rows.Next() {
		var item NavigationItem
		var isAuto int
		var aiGenerated int
		var isDeleted int
		// 处理可能为 NULL 的字段
		var url, lanUrl, wanUrl sql.NullString
		var icon, iconPath sql.NullString
		var containerID sql.NullString

		if err := rows.Scan(&item.ID, &item.Title, &url, &lanUrl, &wanUrl, &icon, &iconPath, &item.Category, &isAuto, &aiGenerated, &isDeleted, &item.HiddenReason, &containerID, &item.CreatedAt, &item.UpdatedAt); err != nil {
			respondError(c, http.StatusInternalServerError, "读取导航项失败", err)
			return
		}
		item.URL = url.String
		item.LanUrl = lanUrl.String
		item.WanUrl = wanUrl.String
		item.IconUrl = browserSafeNavigationIcon(icon.String)
		item.IconPath = iconPath.String
		item.ContainerID = containerID.String
		item.IsAuto = isAuto == 1
		item.AiGenerated = aiGenerated == 1
		item.IsDeleted = isDeleted == 1
		items = append(items, item)
	}

	c.JSON(http.StatusOK, items)
}

// @Summary 创建导航项
// @Description 创建一个新的手动导航项
// @Tags navigation
// @Accept json
// @Produce json
// @Success 201 {object} NavigationItem
func createNavigationItem(c *gin.Context) {
	var req CreateNavigationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "无效的请求参数", err)
		return
	}

	db := database.GetDB()
	result, err := db.Exec(
		"INSERT INTO navigation_items (title, url, lan_url, wan_url, icon, category, is_auto) VALUES (?, ?, ?, ?, ?, ?, 0)",
		req.Title, req.URL, req.LanUrl, req.WanUrl, req.IconUrl, req.Category,
	)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "创建导航项失败", err)
		return
	}

	id, _ := result.LastInsertId()

	// Fetch the created item
	var item NavigationItem
	var isAuto int
	var aiGenerated int
	var isDeleted int
	var url, lanUrl, wanUrl sql.NullString
	var icon, iconPath sql.NullString
	var containerID sql.NullString
	err = db.QueryRow("SELECT id, title, url, lan_url, wan_url, icon, icon_path, category, is_auto, ai_generated, is_deleted, COALESCE(hidden_reason, ''), container_id, created_at, updated_at FROM navigation_items WHERE id = ?", id).
		Scan(&item.ID, &item.Title, &url, &lanUrl, &wanUrl, &icon, &iconPath, &item.Category, &isAuto, &aiGenerated, &isDeleted, &item.HiddenReason, &containerID, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "查询导航项失败", err)
		return
	}
	item.URL = url.String
	item.LanUrl = lanUrl.String
	item.WanUrl = wanUrl.String
	item.IconUrl = browserSafeNavigationIcon(icon.String)
	item.IconPath = iconPath.String
	item.ContainerID = containerID.String
	item.IsAuto = isAuto == 1
	item.AiGenerated = aiGenerated == 1
	item.IsDeleted = isDeleted == 1

	c.JSON(http.StatusCreated, item)
}

// @Summary 更新导航项
// @Description 更新现有的导航项
// @Tags navigation
// @Accept json
// @Produce json
// @Success 200 {object} NavigationItem
func updateNavigationItem(c *gin.Context) {
	id := c.Param("id")
	var req UpdateNavigationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "无效的请求参数", err)
		return
	}

	db := database.GetDB()
	// 注意：这里我们更新所有字段，如果前端传空字符串，也会被更新进去。
	// 根据需求，用户可能想清空某个 URL，所以这是合理的。
	_, err := db.Exec(
		"UPDATE navigation_items SET title = ?, url = ?, lan_url = ?, wan_url = ?, icon = ?, icon_path = ?, category = ?, ai_generated = 0, updated_at = CURRENT_TIMESTAMP WHERE id = ?",
		req.Title, req.URL, req.LanUrl, req.WanUrl, req.IconUrl, req.IconPath, req.Category, id,
	)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "更新导航项失败", err)
		return
	}

	// Fetch the updated item
	var item NavigationItem
	var isAuto int
	var aiGenerated int
	var isDeleted int
	var url, lanUrl, wanUrl sql.NullString
	var icon, iconPath sql.NullString
	var containerID sql.NullString
	err = db.QueryRow("SELECT id, title, url, lan_url, wan_url, icon, icon_path, category, is_auto, ai_generated, is_deleted, COALESCE(hidden_reason, ''), container_id, created_at, updated_at FROM navigation_items WHERE id = ?", id).
		Scan(&item.ID, &item.Title, &url, &lanUrl, &wanUrl, &icon, &iconPath, &item.Category, &isAuto, &aiGenerated, &isDeleted, &item.HiddenReason, &containerID, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "查询导航项失败", err)
		return
	}
	item.URL = url.String
	item.LanUrl = lanUrl.String
	item.WanUrl = wanUrl.String
	item.IconUrl = browserSafeNavigationIcon(icon.String)
	item.IconPath = iconPath.String
	item.ContainerID = containerID.String
	item.IsAuto = isAuto == 1
	item.AiGenerated = aiGenerated == 1
	item.IsDeleted = isDeleted == 1

	c.JSON(http.StatusOK, item)
}

// browserSafeNavigationIcon prevents browsers from loading remote SVG content
// directly. Raster icons and locally cached assets remain unchanged.
func browserSafeNavigationIcon(raw string) string {
	value := strings.TrimSpace(raw)
	if !strings.HasPrefix(value, "http://") && !strings.HasPrefix(value, "https://") {
		return value
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return "container"
	}
	if strings.EqualFold(filepath.Ext(parsed.Path), ".svg") {
		return "container"
	}
	return value
}

// @Summary 删除导航项
// @Description 删除指定的导航项 (软删除)
// @Tags navigation
// @Accept json
// @Produce json
// @Success 200 {object} map[string]string
func deleteNavigationItem(c *gin.Context) {
	id := c.Param("id")
	db := database.GetDB()

	permanent := false
	switch strings.ToLower(strings.TrimSpace(c.Query("permanent"))) {
	case "1", "true", "yes":
		permanent = true
	}

	if permanent {
		result, err := db.Exec("DELETE FROM navigation_items WHERE id = ?", id)
		if err != nil {
			respondError(c, http.StatusInternalServerError, "永久删除导航项失败", err)
			return
		}
		rowsAffected, _ := result.RowsAffected()
		if rowsAffected == 0 {
			respondError(c, http.StatusNotFound, "导航项不存在", nil)
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "导航项已永久删除"})
		return
	}

	result, err := db.Exec("UPDATE navigation_items SET is_deleted = 1, hidden_reason = 'manual', updated_at = CURRENT_TIMESTAMP WHERE id = ?", id)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "删除导航项失败", err)
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		respondError(c, http.StatusNotFound, "导航项不存在", nil)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "导航项已隐藏"})
}

// @Summary 恢复导航项
// @Description 恢复已删除的导航项
// @Tags navigation
// @Accept json
// @Produce json
// @Success 200 {object} map[string]string
func restoreNavigationItem(c *gin.Context) {
	id := c.Param("id")
	db := database.GetDB()

	result, err := db.Exec("UPDATE navigation_items SET is_deleted = 0, hidden_reason = '', updated_at = CURRENT_TIMESTAMP WHERE id = ?", id)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "恢复导航项失败", err)
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		respondError(c, http.StatusNotFound, "导航项不存在", nil)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "导航项已恢复"})
}

// uploadNavigationIcon 处理导航项图标上传并保存至本地
// 根据导航名称生成安全的文件名，保存到数据目录的 icons 子目录，并更新数据库中的 icon 字段
func uploadNavigationIcon(c *gin.Context) {
	id := c.Param("id")
	file, err := c.FormFile("file")
	if err != nil {
		respondError(c, http.StatusBadRequest, "未接收到上传文件", err)
		return
	}

	db := database.GetDB()
	var title string
	err = db.QueryRow("SELECT title FROM navigation_items WHERE id = ?", id).Scan(&title)
	if err != nil {
		respondError(c, http.StatusNotFound, "导航项不存在", err)
		return
	}

	// 生成安全的文件名 (按标题重命名)
	slug := strings.ToLower(strings.TrimSpace(title))
	// 仅保留字母、数字、下划线和中划线
	re := regexp.MustCompile(`[^a-z0-9_-]+`)
	slug = re.ReplaceAllString(slug, "-")
	if slug == "" {
		slug = fmt.Sprintf("nav-%s", id)
	}

	ext := strings.ToLower(filepath.Ext(file.Filename))
	switch ext {
	case ".png", ".jpg", ".jpeg", ".svg", ".gif", ".webp", ".ico", ".avif", ".bmp", ".tif", ".tiff":
	default:
		ext = ".png"
	}

	// 统一存储到 data/pic 目录
	picDir := filepath.Join(settings.GetDataDir(), "pic")
	if err := os.MkdirAll(picDir, 0755); err != nil {
		respondError(c, http.StatusInternalServerError, "创建图片目录失败", err)
		return
	}

	filename := slug + ext
	savePath := filepath.Join(picDir, filename)
	if err := c.SaveUploadedFile(file, savePath); err != nil {
		respondError(c, http.StatusInternalServerError, "保存图标失败", err)
		return
	}

	// 更新数据库 icon 与 icon_path（相对 data 目录路径）
	publicPath := filepath.ToSlash(filepath.Join("/data/pic", filename))
	relativePath := filepath.ToSlash(filepath.Join("pic", filename))
	if _, err := db.Exec("UPDATE navigation_items SET icon = ?, icon_path = ?, ai_generated = 0, updated_at = CURRENT_TIMESTAMP WHERE id = ?", publicPath, relativePath, id); err != nil {
		respondError(c, http.StatusInternalServerError, "更新导航项图标失败", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"icon_url": publicPath})
}
