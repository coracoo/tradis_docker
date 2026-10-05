package cloudflare

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
)

// Repository sources share the CDN contract and the caller's existing content cache.
func normalizeTemplateRepository(raw string) (string, bool, error) {
	raw = strings.TrimRight(strings.TrimSpace(raw), "/")
	u, err := url.Parse(raw)
	if err != nil {
		return raw, false, err
	}
	if u.Hostname() != "github.com" && u.Hostname() != "raw.githubusercontent.com" {
		return raw, false, nil
	}
	invalid := func() (string, bool, error) { return raw, true, fmt.Errorf("invalid GitHub template repository URL") }
	if u.Scheme != "https" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Port() != "" || strings.Contains(u.EscapedPath(), "%") {
		return invalid()
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." || strings.Contains(part, "\\") {
			return invalid()
		}
	}
	if u.Hostname() == "raw.githubusercontent.com" {
		if len(parts) < 3 {
			return invalid()
		}
		return raw, true, nil
	}
	if len(parts) < 2 {
		return invalid()
	}
	parts[1] = strings.TrimSuffix(parts[1], ".git")
	if parts[1] == "" {
		return invalid()
	}
	if len(parts) == 2 {
		parts = append(parts, "main")
	} else {
		if len(parts) < 4 || parts[2] != "tree" {
			return invalid()
		}
		parts = append(parts[:2], parts[3:]...)
	}
	return "https://raw.githubusercontent.com/" + strings.Join(parts, "/"), true, nil
}

func validTemplateFile(file string) bool {
	return strings.HasPrefix(file, "templates/") && strings.HasSuffix(file, ".json") &&
		path.Clean(file) == file && !strings.ContainsAny(file, "%\\?#:")
}

func (c *CDNClient) fetchRepositoryFile(file string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, c.baseURL+"/"+file, nil)
	if err != nil {
		return nil, err
	}
	// Keep redirects inside the configured static source, including in test transports.
	client := *c.httpClient
	client.CheckRedirect = func(next *http.Request, via []*http.Request) error {
		if len(via) >= 5 || next.URL.Scheme != req.URL.Scheme || next.URL.Host != req.URL.Host || next.URL.User != nil {
			return fmt.Errorf("template repository redirect rejected")
		}
		return nil
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("template repository request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("template repository HTTP %d", resp.StatusCode)
	}
	const maxSize = 16 << 20
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxSize+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxSize {
		return nil, fmt.Errorf("template repository file exceeds size limit")
	}
	return data, nil
}

func (c *CDNClient) repositoryAsset(value string) string {
	if value == "" {
		return ""
	}
	u, err := url.Parse(value)
	if err != nil {
		return ""
	}
	if u.IsAbs() {
		if (u.Scheme == "https" || u.Scheme == "http") && u.User == nil {
			return value
		}
		return ""
	}
	if u.Host != "" || strings.Contains(u.Path, "\\") {
		return ""
	}
	base, _ := url.Parse(c.baseURL + "/")
	u.Path = strings.TrimPrefix(u.Path, "/")
	u.RawPath = ""
	resolved := base.ResolveReference(u)
	if !strings.HasPrefix(resolved.Path, base.Path) {
		return ""
	}
	return resolved.String()
}

func (c *CDNClient) fetchRepositoryIndex() ([]TemplateManifest, error) {
	data, err := c.fetchRepositoryFile("index.json")
	if err != nil {
		return nil, err
	}
	var index struct {
		Templates []TemplateManifest `json:"templates"`
	}
	if err := json.Unmarshal(data, &index); err != nil {
		return nil, err
	}
	if index.Templates == nil {
		return nil, fmt.Errorf("template repository index has no templates array")
	}
	ids, names := make(map[uint]bool), make(map[string]bool)
	for i := range index.Templates {
		item := &index.Templates[i]
		if item.ID == 0 || strings.TrimSpace(item.Name) == "" || ids[item.ID] || names[item.Name] || !validTemplateFile(item.File) {
			return nil, fmt.Errorf("template repository index contains invalid or duplicate entries")
		}
		ids[item.ID], names[item.Name] = true, true
		item.Logo = c.repositoryAsset(item.Logo)
	}
	return index.Templates, nil
}

func (c *CDNClient) fetchRepositoryTemplate(key string) (*TemplateDetail, error) {
	index, err := c.fetchRepositoryIndex()
	if err != nil {
		return nil, err
	}
	var selected *TemplateManifest
	for i := range index {
		if strconv.FormatUint(uint64(index[i].ID), 10) == key || index[i].Name == key {
			if selected != nil {
				return nil, fmt.Errorf("ambiguous template identity")
			}
			selected = &index[i]
		}
	}
	if selected == nil {
		return nil, fmt.Errorf("template not found in repository index")
	}
	data, err := c.fetchRepositoryFile(selected.File)
	if err != nil {
		return nil, err
	}
	var detail TemplateDetail
	if err := json.Unmarshal(data, &detail); err != nil {
		return nil, err
	}
	var state struct {
		Enabled *bool `json:"enabled"`
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	if state.Enabled != nil && !*state.Enabled {
		return nil, fmt.Errorf("template is disabled")
	}
	if (detail.ID != 0 && detail.ID != selected.ID) || (detail.Name != "" && detail.Name != selected.Name) {
		return nil, fmt.Errorf("template identity does not match repository index")
	}
	if strings.TrimSpace(detail.Compose) == "" && len(detail.SourceFiles) == 0 {
		return nil, fmt.Errorf("template has no deployment source")
	}
	detail.ID, detail.Enabled = selected.ID, true
	if detail.SortOrder == 0 {
		detail.SortOrder = selected.SortOrder
	}
	for _, field := range []struct {
		value    *string
		fallback string
	}{
		{&detail.Name, selected.Name}, {&detail.Category, selected.Category},
		{&detail.Description, selected.Description}, {&detail.Version, selected.Version},
		{&detail.Website, selected.Website}, {&detail.Logo, selected.Logo},
	} {
		if *field.value == "" {
			*field.value = field.fallback
		}
	}
	detail.Logo = c.repositoryAsset(detail.Logo)
	if detail.Screenshots == nil {
		detail.Screenshots = []string{}
	}
	if detail.Schema == nil {
		detail.Schema = []Variable{}
	}
	for i := range detail.Screenshots {
		detail.Screenshots[i] = c.repositoryAsset(detail.Screenshots[i])
	}
	return &detail, nil
}

func (c *CDNClient) fetchRepositoryRaw(requestPath string) ([]byte, error) {
	u, err := url.Parse(requestPath)
	if err != nil || u.IsAbs() || u.Host != "" {
		return nil, fmt.Errorf("invalid template resource path")
	}
	switch u.Path {
	case "/api/templates":
		list, err := c.fetchRepositoryIndex()
		if err != nil {
			return nil, err
		}
		return json.Marshal(list)
	case "/api/templates/meta":
		return c.fetchRepositoryFile("meta.json")
	default:
		return nil, fmt.Errorf("unsupported template repository resource")
	}
}
