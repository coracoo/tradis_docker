package settings

import (
	"dockerpanel/backend/pkg/database"
	"dockerpanel/backend/pkg/secrets"
	"os"
	"strconv"
	"strings"
)

const DefaultAIMaxTokens = 8192

func DefaultAINavigationPrompt() string {
	return "你是 TRADIS 的导航识别助手。你必须只输出严格 JSON：{\"title\":\"\",\"category\":\"\",\"icon\":\"\"}，不要输出解释、Markdown、代码块或额外字段。title 必须简短稳定，优先使用 user JSON 中的 normalizedTitle 或 title；如果是 project-service 且两段重复，必须折叠为一段；去掉无意义编号后缀。category 必须是具体中文分类；是否允许创建 categoryCandidates 之外的新分类，以系统追加规则为准；无法判断时输出 未分类。icon 优先使用 faviconIcon，其次使用 http(s) 图标 URL；无法确定时输出 mdi-docker。不要编造不可验证的图标地址。"
}

func DefaultAIComposePrompt() string {
	return "你是一个 Docker Compose 编排助手。你必须根据用户需求生成可部署、可维护的 docker compose 配置。优先使用官方镜像，避免固定 container_name，尽量使用命名卷或项目目录挂载，并在存在端口、权限、数据持久化风险时给出 warnings。"
}

func freeAISettingsDefaults(insert func(string, string) error) error {
	for _, entry := range []struct{ key, value string }{
		{"ai_enabled", "false"},
		{"ai_base_url", "https://api.openai.com/v1"},
		{"ai_api_key", ""},
		{"ai_model", ""},
		{"ai_utility_model", ""},
		{"ai_temperature", "0.7"},
		{"ai_max_tokens", strconv.Itoa(DefaultAIMaxTokens)},
		{"ai_allow_create_category", "true"},
		{"ai_navigation_prompt", DefaultAINavigationPrompt()},
		{"ai_compose_prompt", DefaultAIComposePrompt()},
	} {
		if err := insert(entry.key, entry.value); err != nil {
			return err
		}
	}
	return nil
}

func loadFreeAISettings(s *Settings, getValue func(string) string, parseInt func(string, int) int, parseBool func(string, bool) bool) {
	s.AiEnabled = parseBool(getValue("ai_enabled"), false)
	s.AiBaseUrl = strings.TrimSpace(getValue("ai_base_url"))
	s.AiModel = strings.TrimSpace(getValue("ai_model"))
	s.AiUtilityModel = strings.TrimSpace(getValue("ai_utility_model"))
	s.AiMaxTokens = parseInt(getValue("ai_max_tokens"), DefaultAIMaxTokens)
	s.AiAllowCreateCategory = parseBool(getValue("ai_allow_create_category"), true)
	if s.AiMaxTokens <= 0 {
		s.AiMaxTokens = DefaultAIMaxTokens
	}
	s.AiNavigationPrompt = strings.TrimSpace(getValue("ai_navigation_prompt"))
	if s.AiNavigationPrompt == "" {
		s.AiNavigationPrompt = DefaultAINavigationPrompt()
	}
	s.AiComposePrompt = strings.TrimSpace(getValue("ai_compose_prompt"))
	if s.AiComposePrompt == "" {
		s.AiComposePrompt = DefaultAIComposePrompt()
	}
	aiKey, aiKeySource := GetAIAPIKeyWithSource()
	s.AiApiKeyStored = secrets.IsSealed(strings.TrimSpace(getValue("ai_api_key")))
	s.AiApiKeySet = aiKey != ""
	s.AiApiKeySource = aiKeySource
	s.AiApiKey = ""
	if temperature, err := strconv.ParseFloat(strings.TrimSpace(getValue("ai_temperature")), 64); err == nil {
		s.AiTemperature = temperature
	} else {
		s.AiTemperature = 0.7
	}
}

func updateFreeAISettings(s *Settings, update func(string, string) error) error {
	for _, entry := range []struct{ key, value string }{
		{"ai_enabled", strconv.FormatBool(s.AiEnabled)},
		{"ai_base_url", strings.TrimSpace(s.AiBaseUrl)},
		{"ai_model", strings.TrimSpace(s.AiModel)},
		{"ai_utility_model", strings.TrimSpace(s.AiUtilityModel)},
		{"ai_temperature", strconv.FormatFloat(s.AiTemperature, 'f', -1, 64)},
		{"ai_max_tokens", strconv.Itoa(s.AiMaxTokens)},
		{"ai_allow_create_category", strconv.FormatBool(s.AiAllowCreateCategory)},
		{"ai_navigation_prompt", strings.TrimSpace(s.AiNavigationPrompt)},
		{"ai_compose_prompt", strings.TrimSpace(s.AiComposePrompt)},
	} {
		if err := update(entry.key, entry.value); err != nil {
			return err
		}
	}
	return nil
}

func GetAIAPIKey() string {
	key, _ := GetAIAPIKeyWithSource()
	return key
}

func GetAIAPIKeyWithSource() (string, string) {
	if database.GetDB() != nil {
		if dbKey, _ := GetSensitiveValue("ai_api_key"); strings.TrimSpace(dbKey) != "" {
			return strings.TrimSpace(dbKey), "database"
		}
	}
	for _, key := range []string{"AI_API_KEY", "OPENAI_API_KEY", "DEEPSEEK_API_KEY"} {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value, "env:" + key
		}
	}
	return "", ""
}

func EffectiveAIUtilityModel(s Settings) string {
	if model := strings.TrimSpace(s.AiUtilityModel); model != "" {
		return model
	}
	return strings.TrimSpace(s.AiModel)
}
