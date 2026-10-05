//go:build community

package api

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"dockerpanel/backend/pkg/settings"
	"github.com/gin-gonic/gin"
)

var communityTutorialSlug = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,79}$`)

type communityTutorialMeta struct {
	Slug        string   `json:"slug"`
	Title       string   `json:"title"`
	Summary     string   `json:"summary,omitempty"`
	Category    string   `json:"category,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	CoverImage  string   `json:"cover_image,omitempty"`
	OriginalURL string   `json:"original_url,omitempty"`
	UpdatedAt   string   `json:"updated_at,omitempty"`
}

type communityTutorialManifest struct {
	FormatVersion int                     `json:"format_version"`
	Articles      []communityTutorialMeta `json:"articles"`
	TotalCount    int                     `json:"total_count"`
}

type communityTutorialArticle struct {
	communityTutorialMeta
	Content string `json:"content"`
}

func RegisterCommunityTutorialRoutes(r *gin.RouterGroup) {
	group := r.Group("/tutorials")
	group.GET("/manifest", getCommunityTutorialManifest)
	group.GET("/:slug", getCommunityTutorialArticle)
}

func communityTutorialCachePath(source, name string) string {
	sum := sha256.Sum256([]byte(source))
	return filepath.Join(settings.GetDataDir(), "cache", "community-tutorials", hex.EncodeToString(sum[:]), name)
}

func readCommunityTutorialCache(path string, freshOnly bool) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if freshOnly && time.Since(info.ModTime()) > time.Hour {
		return nil, errors.New("cache expired")
	}
	return os.ReadFile(path)
}

func saveCommunityTutorialCache(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tutorial-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err = tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err = tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

func getCommunityTutorialManifest(c *gin.Context) {
	source, err := configuredCommunityTutorialRSSURL()
	if err != nil {
		respondError(c, http.StatusBadGateway, "教程 RSS 配置无效", err)
		return
	}
	if source == "" {
		c.JSON(http.StatusOK, communityTutorialManifest{FormatVersion: 1, Articles: []communityTutorialMeta{}})
		return
	}
	manifest, _, err := loadCommunityTutorialRSS(c.Request.Context(), source, c.Query("refresh") == "1")
	if err != nil {
		respondError(c, http.StatusBadGateway, "教程 RSS 暂不可用", err)
		return
	}
	c.JSON(http.StatusOK, manifest)
}

func getCommunityTutorialArticle(c *gin.Context) {
	slug := c.Param("slug")
	if !communityTutorialSlug.MatchString(slug) {
		respondError(c, http.StatusBadRequest, "教程标识无效", nil)
		return
	}
	source, err := configuredCommunityTutorialRSSURL()
	if err != nil {
		respondError(c, http.StatusBadGateway, "教程 RSS 配置无效", err)
		return
	}
	if source == "" {
		respondError(c, http.StatusNotFound, "尚未配置教程 RSS", nil)
		return
	}
	_, articles, err := loadCommunityTutorialRSS(c.Request.Context(), source, false)
	if err != nil {
		respondError(c, http.StatusBadGateway, "教程 RSS 暂不可用", err)
		return
	}
	article, found := articles[slug]
	if !found {
		respondError(c, http.StatusNotFound, "教程不存在", nil)
		return
	}
	c.JSON(http.StatusOK, article)
}
