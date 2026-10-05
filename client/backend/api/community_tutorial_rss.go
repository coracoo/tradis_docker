//go:build community

package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"dockerpanel/backend/pkg/settings"
)

const communityTutorialRSSLimit = 4 << 20

type communityRSSItem struct {
	GUID           string `xml:"guid"`
	Title          string `xml:"title"`
	Description    string `xml:"description"`
	ContentEncoded string `xml:"http://purl.org/rss/1.0/modules/content/ encoded"`
	PublishedAt    string `xml:"pubDate"`
}

type communityRSSDocument struct {
	Channel struct {
		Items []communityRSSItem `xml:"item"`
	} `xml:"channel"`
}

func communityTutorialRSSURLAllowed(raw string) bool {
	source, err := url.Parse(strings.TrimSpace(raw))
	return err == nil && (source.Scheme == "http" || source.Scheme == "https") &&
		source.Hostname() != "" && source.User == nil && source.Fragment == ""
}

func configuredCommunityTutorialRSSURL() (string, error) {
	current, err := settings.GetSettings()
	if err != nil {
		return "", err
	}
	source := strings.TrimSpace(current.TutorialRSSURL)
	if source != "" && !communityTutorialRSSURLAllowed(source) {
		return "", errors.New("tutorial RSS source must be an HTTP(S) URL without embedded credentials")
	}
	return source, nil
}

func fetchCommunityTutorialRSS(ctx context.Context, source string) ([]byte, error) {
	if !communityTutorialRSSURLAllowed(source) {
		return nil, errors.New("invalid tutorial RSS URL")
	}
	client := &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 || !communityTutorialRSSURLAllowed(req.URL.String()) ||
				(len(via) > 0 && via[0].URL.Scheme == "https" && req.URL.Scheme != "https") {
				return errors.New("unsafe tutorial RSS redirect")
			}
			return nil
		},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
	if err != nil {
		return nil, err
	}
	response, err := client.Do(req)
	if err != nil {
		// A custom feed may carry an access token in its query string.
		return nil, errors.New("tutorial RSS request failed; check source and connectivity")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("public RSS source status %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, communityTutorialRSSLimit+1))
	if err != nil {
		return nil, err
	}
	if len(data) > communityTutorialRSSLimit {
		return nil, errors.New("public RSS source is too large")
	}
	return data, nil
}

func loadCommunityTutorialRSS(ctx context.Context, source string, force bool) (communityTutorialManifest, map[string]communityTutorialArticle, error) {
	cache := communityTutorialCachePath(source, "feed.xml")
	if !force {
		if data, err := readCommunityTutorialCache(cache, true); err == nil {
			if manifest, articles, err := parseCommunityTutorialRSS(data); err == nil {
				return manifest, articles, nil
			}
		}
	}
	data, fetchErr := fetchCommunityTutorialRSS(ctx, source)
	if fetchErr == nil {
		manifest, articles, parseErr := parseCommunityTutorialRSS(data)
		if parseErr == nil {
			_ = saveCommunityTutorialCache(cache, data)
			return manifest, articles, nil
		}
		fetchErr = parseErr
	}
	if data, err := readCommunityTutorialCache(cache, false); err == nil {
		if manifest, articles, err := parseCommunityTutorialRSS(data); err == nil {
			return manifest, articles, nil
		}
	}
	return communityTutorialManifest{}, nil, fetchErr
}

func parseCommunityTutorialRSS(data []byte) (communityTutorialManifest, map[string]communityTutorialArticle, error) {
	var feed communityRSSDocument
	if err := xml.Unmarshal(data, &feed); err != nil {
		return communityTutorialManifest{}, nil, err
	}
	if len(feed.Channel.Items) == 0 || len(feed.Channel.Items) > 200 {
		return communityTutorialManifest{}, nil, errors.New("public RSS article count is invalid")
	}
	manifest := communityTutorialManifest{FormatVersion: 1, Articles: make([]communityTutorialMeta, 0, len(feed.Channel.Items))}
	articles := make(map[string]communityTutorialArticle, len(feed.Channel.Items))
	for _, item := range feed.Channel.Items {
		title := strings.TrimSpace(item.Title)
		content := strings.TrimSpace(item.ContentEncoded)
		if content == "" {
			content = strings.TrimSpace(item.Description)
		}
		if title == "" || content == "" {
			continue
		}
		identity := strings.TrimSpace(item.GUID)
		if identity == "" {
			identity = title
		}
		sum := sha256.Sum256([]byte(identity))
		slug := "article-" + hex.EncodeToString(sum[:8])
		meta := communityTutorialMeta{Slug: slug, Title: title}
		if published, err := time.Parse(time.RFC1123Z, strings.TrimSpace(item.PublishedAt)); err == nil {
			meta.UpdatedAt = published.Format(time.RFC3339)
		}
		if _, exists := articles[slug]; exists {
			return communityTutorialManifest{}, nil, errors.New("duplicate public RSS article")
		}
		manifest.Articles = append(manifest.Articles, meta)
		articles[slug] = communityTutorialArticle{communityTutorialMeta: meta, Content: content}
	}
	if len(articles) == 0 {
		return communityTutorialManifest{}, nil, errors.New("public RSS has no readable articles")
	}
	manifest.TotalCount = len(manifest.Articles)
	return manifest, articles, nil
}
