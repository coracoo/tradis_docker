//go:build community

package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"dockerpanel/backend/pkg/settings"
	"github.com/gin-gonic/gin"
)

const communityTutorialCDNPath = "/api/community/tutorials"

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

func communityTutorialSource() (string, error) {
	s, err := settings.GetSettings()
	if err != nil {
		return "", err
	}
	base := strings.TrimRight(strings.TrimSpace(s.AppStoreCDNURL), "/")
	u, err := url.Parse(base)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("invalid public CDN URL")
	}
	return base, nil
}

func communityTutorialCachePath(base, name string) string {
	sum := sha256.Sum256([]byte(base))
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

func fetchCommunityTutorialJSON(ctx context.Context, base, path string, limit int64) ([]byte, error) {
	return fetchCommunityCDNJSON(ctx, base, communityTutorialCDNPath+path, limit)
}

func fetchCommunityCDNJSON(ctx context.Context, base, path string, limit int64) ([]byte, error) {
	endpoint := base + path
	baseURL, _ := url.Parse(base)
	client := &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if req.URL.Scheme != baseURL.Scheme || req.URL.Host != baseURL.Host || len(via) >= 3 {
				return errors.New("public tutorial redirect left CDN origin")
			}
			return nil
		},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("public tutorial CDN status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errors.New("public tutorial payload too large")
	}
	return data, nil
}

func loadCommunityTutorialManifest(ctx context.Context, force bool) (communityTutorialManifest, error) {
	base, err := communityTutorialSource()
	if err != nil {
		return communityTutorialManifest{}, err
	}
	if source, err := configuredCommunityTutorialRSSURL(base); err != nil {
		return communityTutorialManifest{}, err
	} else if source != "" {
		manifest, _, err := loadCommunityTutorialRSS(ctx, source, base, force)
		return manifest, err
	}
	cache := communityTutorialCachePath(base, "manifest.json")
	if !force {
		if data, err := readCommunityTutorialCache(cache, true); err == nil {
			if manifest, err := parseCommunityTutorialManifest(data); err == nil {
				return manifest, nil
			}
		}
	}
	data, fetchErr := fetchCommunityTutorialJSON(ctx, base, "/manifest", 1<<20)
	if fetchErr == nil {
		manifest, parseErr := parseCommunityTutorialManifest(data)
		if parseErr == nil {
			_ = saveCommunityTutorialCache(cache, data)
			return manifest, nil
		}
		fetchErr = parseErr
	}
	if data, err := readCommunityTutorialCache(cache, false); err == nil {
		if manifest, err := parseCommunityTutorialManifest(data); err == nil {
			return manifest, nil
		}
	}
	return communityTutorialManifest{}, fetchErr
}

func parseCommunityTutorialManifest(data []byte) (communityTutorialManifest, error) {
	var manifest communityTutorialManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return manifest, err
	}
	if manifest.FormatVersion != 1 || manifest.Articles == nil {
		return manifest, errors.New("unsupported public tutorial catalog")
	}
	seen := make(map[string]bool, len(manifest.Articles))
	for _, item := range manifest.Articles {
		if !communityTutorialSlug.MatchString(item.Slug) || strings.TrimSpace(item.Title) == "" || seen[item.Slug] {
			return manifest, errors.New("invalid public tutorial entry")
		}
		seen[item.Slug] = true
	}
	manifest.TotalCount = len(manifest.Articles)
	return manifest, nil
}

func getCommunityTutorialManifest(c *gin.Context) {
	manifest, err := loadCommunityTutorialManifest(c.Request.Context(), c.Query("refresh") == "1")
	if err != nil {
		respondError(c, http.StatusBadGateway, "公开教程目录暂不可用", err)
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
	base, err := communityTutorialSource()
	if err != nil {
		respondError(c, http.StatusBadGateway, "公开教程源不可用", err)
		return
	}
	if source, err := configuredCommunityTutorialRSSURL(base); err != nil {
		respondError(c, http.StatusBadGateway, "公开教程源不可用", err)
		return
	} else if source != "" {
		_, articles, err := loadCommunityTutorialRSS(c.Request.Context(), source, base, false)
		if err != nil {
			respondError(c, http.StatusBadGateway, "公开教程正文暂不可用", err)
			return
		}
		article, found := articles[slug]
		if !found {
			respondError(c, http.StatusNotFound, "公开教程不存在", nil)
			return
		}
		c.JSON(http.StatusOK, article)
		return
	}
	manifest, err := loadCommunityTutorialManifest(c.Request.Context(), false)
	if err != nil {
		respondError(c, http.StatusBadGateway, "公开教程目录暂不可用", err)
		return
	}
	listed := false
	for _, item := range manifest.Articles {
		if item.Slug == slug {
			listed = true
			break
		}
	}
	if !listed {
		respondError(c, http.StatusNotFound, "公开教程不存在", nil)
		return
	}
	cache := communityTutorialCachePath(base, filepath.Join("articles", slug+".json"))
	if data, err := readCommunityTutorialCache(cache, true); err == nil {
		if article, err := parseCommunityTutorialArticle(data, slug); err == nil {
			c.JSON(http.StatusOK, article)
			return
		}
	}
	data, fetchErr := fetchCommunityTutorialJSON(c.Request.Context(), base, "/articles/"+slug, 2<<20)
	if fetchErr == nil {
		article, parseErr := parseCommunityTutorialArticle(data, slug)
		if parseErr == nil {
			_ = saveCommunityTutorialCache(cache, data)
			c.JSON(http.StatusOK, article)
			return
		}
		fetchErr = parseErr
	}
	if data, err := readCommunityTutorialCache(cache, false); err == nil {
		if article, err := parseCommunityTutorialArticle(data, slug); err == nil {
			c.JSON(http.StatusOK, article)
			return
		}
	}
	respondError(c, http.StatusBadGateway, "公开教程正文暂不可用", fetchErr)
}

func parseCommunityTutorialArticle(data []byte, slug string) (communityTutorialArticle, error) {
	var article communityTutorialArticle
	if err := json.Unmarshal(data, &article); err != nil {
		return article, err
	}
	if article.Slug != slug || strings.TrimSpace(article.Title) == "" || strings.TrimSpace(article.Content) == "" {
		return article, errors.New("invalid public tutorial article")
	}
	return article, nil
}
