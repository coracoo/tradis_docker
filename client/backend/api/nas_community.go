//go:build community

package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
)

func RegisterNASRoutes(r *gin.RouterGroup) {
	nas := r.Group("/nas")
	for _, endpoint := range []string{"categories", "list", "tags", "banners", "reviews", "topics"} {
		name := endpoint
		nas.GET("/"+name, func(c *gin.Context) { serveCommunityNAS(c, name, "") })
	}
	nas.GET("/network-tiers", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"tiers": []string{"10G", "5G", "2.5G", "1000M"}})
	})
	nas.GET("/reviews/:id", func(c *gin.Context) { serveCommunityNAS(c, "reviews", c.Param("id")) })
	nas.GET("/topics/:slug", func(c *gin.Context) { serveCommunityNAS(c, "topics", c.Param("slug")) })
}

func serveCommunityNAS(c *gin.Context, collection, id string) {
	base, err := communityTutorialSource()
	if err != nil {
		respondError(c, http.StatusBadGateway, "NAS 内容 CDN 未配置", err)
		return
	}
	path := "/api/nas/" + collection
	if id != "" {
		if len(id) > 128 || strings.ContainsAny(id, "/\\") || id == "." || id == ".." {
			respondError(c, http.StatusBadRequest, "NAS 内容标识无效", nil)
			return
		}
		path += "/" + url.PathEscape(id)
	}
	if collection == "list" && c.Request.URL.RawQuery != "" {
		path += "?" + c.Request.URL.RawQuery
	}
	data, err := fetchCommunityNASJSON(c, base, path)
	if err != nil {
		respondError(c, http.StatusBadGateway, "NAS 内容 CDN 暂不可用", err)
		return
	}
	data, err = sanitizeCommunityNASContent(data, base)
	if err != nil {
		respondError(c, http.StatusBadGateway, "NAS 内容数据无效", err)
		return
	}
	c.Data(http.StatusOK, "application/json; charset=utf-8", data)
}

func fetchCommunityNASJSON(c *gin.Context, base, path string) ([]byte, error) {
	data, err := fetchCommunityCDNJSON(c.Request.Context(), base, path, 4<<20)
	if err != nil {
		return nil, err
	}
	if !json.Valid(data) {
		return nil, errors.New("invalid NAS JSON")
	}
	return data, nil
}

func sanitizeCommunityNASContent(data []byte, cdnBase string) ([]byte, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	cdnURL, _ := url.Parse(cdnBase)
	return json.Marshal(sanitizeCommunityNASValue(value, cdnURL))
}

func sanitizeCommunityNASValue(value any, cdnURL *url.URL) any {
	switch item := value.(type) {
	case []any:
		for i := range item {
			item[i] = sanitizeCommunityNASValue(item[i], cdnURL)
		}
	case map[string]any:
		for key, field := range item {
			switch key {
			case "purchase_url", "jd_search_url", "tb_search_url":
				delete(item, key)
			case "image_url", "ImageURL", "BannerImage", "CoverImage", "cover_image", "banner_image":
				if imageURL := communityNASImageURL(field, cdnURL); imageURL != "" {
					item[key] = imageURL
				} else {
					delete(item, key)
				}
			case "official_url":
				if !communityNASPublicLink(field) {
					delete(item, key)
				}
			case "LinkURL":
				link, ok := field.(string)
				if !ok || !(link == "/nas-store" || strings.HasPrefix(link, "/nas-store/")) {
					delete(item, key)
				}
			default:
				item[key] = sanitizeCommunityNASValue(field, cdnURL)
			}
		}
	}
	return value
}

func communityNASImageURL(value any, cdnURL *url.URL) string {
	raw, ok := value.(string)
	if !ok || cdnURL == nil {
		return ""
	}
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.User != nil {
		return ""
	}
	if !parsed.IsAbs() && strings.HasPrefix(raw, "/") && !strings.HasPrefix(raw, "//") {
		parsed = cdnURL.ResolveReference(parsed)
	}
	if parsed.Scheme != cdnURL.Scheme || !strings.EqualFold(parsed.Host, cdnURL.Host) {
		return ""
	}
	return parsed.String()
}

func communityNASPublicLink(value any) bool {
	raw, ok := value.(string)
	if !ok {
		return false
	}
	link, err := url.Parse(strings.TrimSpace(raw))
	return err == nil && (link.Scheme == "https" || link.Scheme == "http") &&
		link.Host != "" && link.User == nil && !strings.Contains(link.Path, "/api/nas/redirect/")
}
