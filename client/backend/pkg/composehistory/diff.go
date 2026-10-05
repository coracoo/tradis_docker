package composehistory

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type ChangeKind string

const (
	ChangeAdded    ChangeKind = "added"
	ChangeRemoved  ChangeKind = "removed"
	ChangeModified ChangeKind = "modified"
)

type Change struct {
	Path   string     `json:"path"`
	Field  string     `json:"field"`
	Kind   ChangeKind `json:"kind"`
	Before string     `json:"before,omitempty"`
	After  string     `json:"after,omitempty"`
	Count  int        `json:"count,omitempty"`
}

type ChangeGroup struct {
	Key     string   `json:"key"`
	Label   string   `json:"label"`
	Changes []Change `json:"changes"`
}

type ChangeSummary struct {
	Total    int `json:"total"`
	Added    int `json:"added"`
	Modified int `json:"modified"`
	Removed  int `json:"removed"`
}

type Preview struct {
	BaseHash      string        `json:"baseHash"`
	CandidateHash string        `json:"candidateHash"`
	HasChanges    bool          `json:"hasChanges"`
	YAMLChanged   bool          `json:"yamlChanged"`
	EnvChanged    bool          `json:"envChanged"`
	Summary       ChangeSummary `json:"summary"`
	Groups        []ChangeGroup `json:"groups"`
}

var safeServiceFields = map[string]bool{
	"image":        true,
	"build":        true,
	"ports":        true,
	"volumes":      true,
	"devices":      true,
	"privileged":   true,
	"user":         true,
	"cap_add":      true,
	"cap_drop":     true,
	"read_only":    true,
	"restart":      true,
	"network_mode": true,
	"healthcheck":  true,
}

var redactedSafeFields = map[string]bool{
	"healthcheck": true,
}

func BuildPreview(currentYAML, candidateYAML, currentEnv, candidateEnv string) (Preview, error) {
	current, err := parseComposeYAML(currentYAML)
	if err != nil {
		return Preview{}, fmt.Errorf("当前 Compose YAML 无效: %w", err)
	}
	candidate, err := parseComposeYAML(candidateYAML)
	if err != nil {
		return Preview{}, fmt.Errorf("候选 Compose YAML 无效: %w", err)
	}

	baseHash, err := hashNormalized(current)
	if err != nil {
		return Preview{}, err
	}
	candidateHash, err := hashNormalized(candidate)
	if err != nil {
		return Preview{}, err
	}

	groups := map[string]*ChangeGroup{}
	compareServices(groups, mapValue(current["services"]), mapValue(candidate["services"]))
	compareTopLevel(groups, current, candidate)
	compareEnvironment(groups, parseDotEnv(currentEnv), parseDotEnv(candidateEnv))

	preview := Preview{
		BaseHash:      baseHash,
		CandidateHash: candidateHash,
		YAMLChanged:   baseHash != candidateHash,
		EnvChanged:    !reflect.DeepEqual(parseDotEnv(currentEnv), parseDotEnv(candidateEnv)),
		Groups:        sortedGroups(groups),
	}
	for _, group := range preview.Groups {
		for _, change := range group.Changes {
			preview.Summary.Total++
			switch change.Kind {
			case ChangeAdded:
				preview.Summary.Added++
			case ChangeRemoved:
				preview.Summary.Removed++
			case ChangeModified:
				preview.Summary.Modified++
			}
		}
	}
	preview.HasChanges = preview.YAMLChanged || preview.EnvChanged
	return preview, nil
}

func YAMLHash(raw string) (string, error) {
	document, err := parseComposeYAML(raw)
	if err != nil {
		return "", err
	}
	return hashNormalized(document)
}

func parseComposeYAML(raw string) (map[string]any, error) {
	var decoded any
	if err := yaml.Unmarshal([]byte(raw), &decoded); err != nil {
		return nil, fmt.Errorf("YAML 解析失败: %w", err)
	}
	normalized, err := normalizeYAMLValue(decoded)
	if err != nil {
		return nil, err
	}
	document, ok := normalized.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("Compose YAML 根节点必须是映射")
	}
	services, ok := document["services"].(map[string]any)
	if !ok || len(services) == 0 {
		return nil, fmt.Errorf("Compose YAML 根节点必须包含非空 services 映射")
	}
	return document, nil
}

func normalizeYAMLValue(value any) (any, error) {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, child := range typed {
			normalized, err := normalizeYAMLValue(child)
			if err != nil {
				return nil, err
			}
			out[key] = normalized
		}
		return out, nil
	case map[any]any:
		out := make(map[string]any, len(typed))
		for key, child := range typed {
			name, ok := key.(string)
			if !ok {
				return nil, fmt.Errorf("YAML 映射键必须是字符串")
			}
			normalized, err := normalizeYAMLValue(child)
			if err != nil {
				return nil, err
			}
			out[name] = normalized
		}
		return out, nil
	case []any:
		out := make([]any, len(typed))
		for i, child := range typed {
			normalized, err := normalizeYAMLValue(child)
			if err != nil {
				return nil, err
			}
			out[i] = normalized
		}
		return out, nil
	default:
		return typed, nil
	}
}

func hashNormalized(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("规范化 Compose YAML 失败: %w", err)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func compareServices(groups map[string]*ChangeGroup, current, candidate map[string]any) {
	for _, service := range unionKeys(current, candidate) {
		before, beforeOK := current[service]
		after, afterOK := candidate[service]
		groupKey := "services." + service
		switch {
		case !beforeOK:
			addChange(groups, groupKey, service, Change{Path: groupKey, Field: "service", Kind: ChangeAdded})
			continue
		case !afterOK:
			addChange(groups, groupKey, service, Change{Path: groupKey, Field: "service", Kind: ChangeRemoved})
			continue
		}

		beforeMap := mapValue(before)
		afterMap := mapValue(after)
		for _, field := range unionKeys(beforeMap, afterMap) {
			oldValue, oldOK := beforeMap[field]
			newValue, newOK := afterMap[field]
			if oldOK && newOK && reflect.DeepEqual(oldValue, newValue) {
				continue
			}
			if field == "environment" {
				compareServiceEnvironment(groups, groupKey, environmentMap(oldValue), environmentMap(newValue))
				continue
			}
			kind := changedKind(oldOK, newOK)
			change := Change{Path: groupKey + "." + field, Field: field, Kind: kind, Count: changedCount(oldValue, newValue)}
			if safeServiceFields[field] && !redactedSafeFields[field] {
				if oldOK {
					change.Before = formatSafeField(field, oldValue)
				}
				if newOK {
					change.After = formatSafeField(field, newValue)
				}
			}
			addChange(groups, groupKey, service, change)
		}
	}
}

func compareServiceEnvironment(groups map[string]*ChangeGroup, groupKey string, current, candidate map[string]string) {
	for _, name := range unionStringKeys(current, candidate) {
		before, beforeOK := current[name]
		after, afterOK := candidate[name]
		if beforeOK && afterOK && before == after {
			continue
		}
		addChange(groups, groupKey, strings.TrimPrefix(groupKey, "services."), Change{
			Path:  groupKey + ".environment." + name,
			Field: "environment." + name,
			Kind:  changedKind(beforeOK, afterOK),
		})
	}
}

func compareTopLevel(groups map[string]*ChangeGroup, current, candidate map[string]any) {
	for _, field := range unionKeys(current, candidate) {
		if field == "services" {
			continue
		}
		before, beforeOK := current[field]
		after, afterOK := candidate[field]
		if beforeOK && afterOK && reflect.DeepEqual(before, after) {
			continue
		}
		if field == "volumes" || field == "networks" || field == "configs" || field == "secrets" {
			compareTopLevelEntries(groups, field, mapValue(before), mapValue(after))
			continue
		}
		addChange(groups, "compose", "其他配置", Change{
			Path:  field,
			Field: field,
			Kind:  changedKind(beforeOK, afterOK),
			Count: changedCount(before, after),
		})
	}
}

func compareTopLevelEntries(groups map[string]*ChangeGroup, groupKey string, current, candidate map[string]any) {
	for _, name := range unionKeys(current, candidate) {
		before, beforeOK := current[name]
		after, afterOK := candidate[name]
		if beforeOK && afterOK && reflect.DeepEqual(before, after) {
			continue
		}
		addChange(groups, groupKey, topLevelLabel(groupKey), Change{
			Path:  groupKey + "." + name,
			Field: name,
			Kind:  changedKind(beforeOK, afterOK),
			Count: changedCount(before, after),
		})
	}
}

func compareEnvironment(groups map[string]*ChangeGroup, current, candidate map[string]string) {
	for _, name := range unionStringKeys(current, candidate) {
		before, beforeOK := current[name]
		after, afterOK := candidate[name]
		if beforeOK && afterOK && before == after {
			continue
		}
		addChange(groups, "environment", ".env 环境变量", Change{
			Path:  "environment." + name,
			Field: name,
			Kind:  changedKind(beforeOK, afterOK),
		})
	}
}

func parseDotEnv(raw string) map[string]string {
	values := map[string]string{}
	for _, line := range strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		name, value, found := strings.Cut(line, "=")
		name = strings.TrimSpace(name)
		if !found || name == "" {
			continue
		}
		values[name] = value
	}
	return values
}

func environmentMap(value any) map[string]string {
	out := map[string]string{}
	switch typed := value.(type) {
	case map[string]any:
		for name, raw := range typed {
			out[name] = fmt.Sprint(raw)
		}
	case []any:
		for _, raw := range typed {
			name, value, found := strings.Cut(fmt.Sprint(raw), "=")
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			if !found {
				value = ""
			}
			out[name] = value
		}
	}
	return out
}

func formatSafeField(field string, value any) string {
	if field == "build" {
		switch typed := value.(type) {
		case string:
			return typed
		case map[string]any:
			parts := make([]string, 0, 2)
			for _, key := range []string{"context", "dockerfile"} {
				if raw, ok := typed[key]; ok {
					parts = append(parts, key+": "+fmt.Sprint(raw))
				}
			}
			return strings.Join(parts, ", ")
		}
	}
	switch typed := value.(type) {
	case []any:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			parts = append(parts, formatScalarOrMap(item))
		}
		return strings.Join(parts, ", ")
	default:
		return formatScalarOrMap(typed)
	}
}

func formatScalarOrMap(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case bool:
		return strconv.FormatBool(typed)
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case map[string]any:
		raw, _ := json.Marshal(typed)
		return string(raw)
	default:
		return fmt.Sprint(typed)
	}
}

func addChange(groups map[string]*ChangeGroup, key, label string, change Change) {
	group := groups[key]
	if group == nil {
		group = &ChangeGroup{Key: key, Label: label, Changes: []Change{}}
		groups[key] = group
	}
	group.Changes = append(group.Changes, change)
}

func sortedGroups(groups map[string]*ChangeGroup) []ChangeGroup {
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]ChangeGroup, 0, len(keys))
	for _, key := range keys {
		group := groups[key]
		sort.SliceStable(group.Changes, func(i, j int) bool {
			if group.Changes[i].Path == group.Changes[j].Path {
				return group.Changes[i].Kind < group.Changes[j].Kind
			}
			return group.Changes[i].Path < group.Changes[j].Path
		})
		out = append(out, *group)
	}
	return out
}

func changedKind(beforeOK, afterOK bool) ChangeKind {
	switch {
	case !beforeOK && afterOK:
		return ChangeAdded
	case beforeOK && !afterOK:
		return ChangeRemoved
	default:
		return ChangeModified
	}
}

func changedCount(before, after any) int {
	beforeCount := collectionLength(before)
	afterCount := collectionLength(after)
	if beforeCount > afterCount {
		return beforeCount - afterCount
	}
	return afterCount - beforeCount
}

func collectionLength(value any) int {
	switch typed := value.(type) {
	case []any:
		return len(typed)
	case map[string]any:
		return len(typed)
	default:
		return 0
	}
}

func mapValue(value any) map[string]any {
	if typed, ok := value.(map[string]any); ok {
		return typed
	}
	return map[string]any{}
}

func unionKeys(first, second map[string]any) []string {
	keys := make(map[string]struct{}, len(first)+len(second))
	for key := range first {
		keys[key] = struct{}{}
	}
	for key := range second {
		keys[key] = struct{}{}
	}
	out := make([]string, 0, len(keys))
	for key := range keys {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

func unionStringKeys(first, second map[string]string) []string {
	keys := make(map[string]struct{}, len(first)+len(second))
	for key := range first {
		keys[key] = struct{}{}
	}
	for key := range second {
		keys[key] = struct{}{}
	}
	out := make([]string, 0, len(keys))
	for key := range keys {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

func topLevelLabel(key string) string {
	switch key {
	case "volumes":
		return "数据卷"
	case "networks":
		return "网络"
	case "configs":
		return "配置"
	case "secrets":
		return "密钥声明"
	default:
		return key
	}
}
