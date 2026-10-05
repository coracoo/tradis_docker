package api

import (
	"bytes"
	"dockerpanel/backend/internal/templatecompiler"
	"fmt"
	"hash/crc32"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
	"gopkg.in/yaml.v3"
)

func getAppVars(c *gin.Context) {
	id := c.Param("id")
	app, err := getAppFromCacheOrServer(id)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "获取应用详情失败", err)
		return
	}

	response, err := buildAppStoreManifestVars(app)
	if err != nil {
		respondError(c, http.StatusBadRequest, "解析应用配置失败", err)
		return
	}
	c.JSON(http.StatusOK, response)
}

func buildAppStoreManifestVars(app *App) (AppVarsResponse, error) {
	_, manifest, diagnostics, err := resolveAppStoreManifest(app)
	if err != nil {
		return AppVarsResponse{}, err
	}
	initialValues := make(map[string]string, len(manifest.Inputs))
	variables := make([]AppVariableInfo, 0, len(manifest.Inputs))
	params := make([]AppParam, 0, len(manifest.Inputs))
	for _, input := range manifest.Inputs {
		initialValues[input.ID] = input.DefaultValue
		sources := []string{input.Scope}
		variables = append(variables, AppVariableInfo{
			InputID:      input.ID,
			Name:         input.Key,
			Value:        input.DefaultValue,
			DefaultValue: input.DefaultValue,
			Required:     input.Required,
			Sources:      sources,
		})
		param := AppParam{
			InputID:      input.ID,
			Key:          input.Key,
			Kind:         AppParamKind(input.Kind),
			Value:        input.DefaultValue,
			DefaultValue: input.DefaultValue,
			Required:     input.Required,
			Sources:      []AppParamSource{AppParamSource(input.Scope)},
		}
		for _, binding := range manifest.Bindings {
			if binding.InputID != input.ID {
				continue
			}
			param.Bindings = append(param.Bindings, AppParamBinding{
				ServiceName: binding.Service,
				Target:      binding.TargetKind,
				File:        binding.File,
				Required:    input.Required,
			})
		}
		for _, mapping := range manifest.Mappings {
			if mapping.ID != input.ID {
				continue
			}
			param.Bindings = append(param.Bindings, AppParamBinding{
				ServiceName: mapping.Service,
				Target:      mapping.Kind,
			})
		}
		params = append(params, param)
	}
	schema := variablesFromLegacyProjection(templatecompiler.ProjectLegacySchema(manifest))
	app.Schema = schema
	warnings := make([]string, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		warnings = append(warnings, diagnostic.Code+": "+diagnostic.Message)
	}
	return AppVarsResponse{
		App:           app,
		Dotenv:        app.Dotenv,
		Manifest:      manifest,
		InitialValues: initialValues,
		Diagnostics:   diagnostics,
		Schema:        schema,
		Variables:     variables,
		Params:        params,
		Warnings:      warnings,
	}, nil
}

func variablesFromLegacyProjection(projected []templatecompiler.LegacyVariable) []Variable {
	variables := make([]Variable, 0, len(projected))
	for _, item := range projected {
		variables = append(variables, Variable{
			InputID:     item.InputID,
			Name:        item.Name,
			Label:       item.Label,
			Description: item.Description,
			Type:        item.Type,
			Default:     item.Default,
			Category:    item.Category,
			ServiceName: item.ServiceName,
			ParamType:   item.ParamType,
			EnvFile:     item.EnvFile,
		})
	}
	return variables
}

// buildLegacyAppStoreParams is retained only for old request compatibility tests.
// New AppStore callers must consume buildAppStoreManifestVars.
func buildLegacyAppStoreParams(app *App, refs map[string]composeVarRef, dotenvMap map[string]string, envFilesByService map[string][]envFileRef, secretDefs map[string]composeSecretDef, secretUses []composeSecretUse) ([]AppParam, []string) {
	warnings := make([]string, 0)
	paramsMap := make(map[string]*AppParam)
	ensureEnvParam := func(key string) *AppParam {
		key = strings.TrimSpace(key)
		if key == "" {
			return nil
		}
		if p, ok := paramsMap[key]; ok {
			return p
		}
		p := &AppParam{
			Key:      key,
			Kind:     AppParamKindEnv,
			Required: false,
		}
		paramsMap[key] = p
		return p
	}
	addSource := func(p *AppParam, s AppParamSource) {
		for _, it := range p.Sources {
			if it == s {
				return
			}
		}
		p.Sources = append(p.Sources, s)
	}
	addUsage := func(p *AppParam, u AppParamUsage) {
		for _, it := range p.Usages {
			if it == u {
				return
			}
		}
		p.Usages = append(p.Usages, u)
	}
	addExample := func(p *AppParam, ex string) {
		ex = strings.TrimSpace(ex)
		if ex == "" {
			return
		}
		for _, it := range p.Examples {
			if it == ex {
				return
			}
		}
		if len(p.Examples) >= 3 {
			return
		}
		p.Examples = append(p.Examples, ex)
	}
	addBinding := func(p *AppParam, b AppParamBinding) {
		b.ServiceName = strings.TrimSpace(b.ServiceName)
		b.Target = strings.TrimSpace(b.Target)
		b.File = strings.TrimSpace(b.File)
		if b.ServiceName == "" || b.Target == "" {
			return
		}
		for _, it := range p.Bindings {
			if it.ServiceName == b.ServiceName && it.Target == b.Target && it.File == b.File {
				return
			}
		}
		p.Bindings = append(p.Bindings, b)
	}

	schemaDefaultMap := make(map[string]string)
	if app != nil {
		for _, v := range app.Schema {
			key := strings.TrimSpace(v.Name)
			if key == "" || strings.EqualFold(key, "PATH") {
				continue
			}
			if !isSchemaEnvVariable(v) {
				continue
			}
			if strings.TrimSpace(v.Default) == "" {
				continue
			}
			if _, ok := schemaDefaultMap[key]; !ok {
				schemaDefaultMap[key] = strings.TrimSpace(v.Default)
			}
		}
	}

	ensureTypedParam := func(key, paramType string) *AppParam {
		key = strings.TrimSpace(key)
		if key == "" {
			return nil
		}
		if p, ok := paramsMap[key]; ok {
			return p
		}
		p := &AppParam{
			Key:      key,
			Kind:     AppParamKindEnv,
			Required: false,
		}
		paramsMap[key] = p
		return p
	}

	for k, ref := range refs {
		if strings.EqualFold(k, "PATH") {
			continue
		}
		p := ensureEnvParam(k)
		if p == nil {
			continue
		}
		addUsage(p, AppParamUsageInterpolation)
		addSource(p, AppParamSourceComposeRef)
		for _, ex := range ref.Examples {
			addExample(p, ex)
		}
		if ref.HasDefault && strings.TrimSpace(p.DefaultValue) == "" {
			p.DefaultValue = strings.TrimSpace(ref.DefaultValue)
			addSource(p, AppParamSourceComposeDefault)
		}
	}

	for k := range dotenvMap {
		if strings.EqualFold(k, "PATH") {
			continue
		}
		p := ensureEnvParam(k)
		if p == nil {
			continue
		}
		addSource(p, AppParamSourceDotenv)
	}

	if app != nil {
		for _, v := range app.Schema {
			key := strings.TrimSpace(v.Name)
			if key == "" || strings.EqualFold(key, "PATH") {
				continue
			}

			pt := strings.ToLower(strings.TrimSpace(v.ParamType))
			if pt == "" {
				pt = strings.ToLower(strings.TrimSpace(v.Type))
			}

			var p *AppParam
			switch pt {
			case "port", "path", "volume":
				p = ensureTypedParam(key, pt)
			default:
				if !isLikelyEnvKey(key) {
					continue
				}
				if !isSchemaEnvVariable(v) {
					continue
				}
				p = ensureEnvParam(key)
			}
			if p == nil {
				continue
			}
			addSource(p, AppParamSourceSchema)
			if strings.TrimSpace(p.DefaultValue) == "" && strings.TrimSpace(v.Default) != "" {
				p.DefaultValue = strings.TrimSpace(v.Default)
			}

			svc := strings.TrimSpace(v.ServiceName)
			if svc == "" {
				svc = "Global"
			}
			if svc == "Global" {
				continue
			}

			switch pt {
			case "port":
				addBinding(p, AppParamBinding{ServiceName: svc, Target: "ports"})
			case "path", "volume":
				addBinding(p, AppParamBinding{ServiceName: svc, Target: "volumes"})
			case "env", "environment":
				addUsage(p, AppParamUsageRuntimeEnv)
				files := envFilesByService[svc]
				if len(files) > 0 {
					targetFile := strings.TrimSpace(v.EnvFile)
					if targetFile == "" {
						if len(files) == 1 {
							targetFile = strings.TrimSpace(files[0].Path)
						} else {
							warnings = append(warnings, fmt.Sprintf("服务 %s 存在多个 env_file，但变量 %s 未指定 envFile 绑定", svc, key))
						}
					}
					if targetFile != "" {
						addBinding(p, AppParamBinding{ServiceName: svc, Target: "env_file", File: targetFile})
					}
				} else {
					addBinding(p, AppParamBinding{ServiceName: svc, Target: "environment"})
				}
			}
		}
	}

	for key, p := range paramsMap {
		ref, inCompose := refs[key]
		finalDefault := firstNonEmpty(dotenvMap[key], schemaDefaultMap[key], func() string {
			if inCompose && ref.HasDefault {
				return ref.DefaultValue
			}
			return ""
		}(), "")
		p.DefaultValue = finalDefault
		required := inCompose && !ref.HasDefault && strings.TrimSpace(finalDefault) == ""
		p.Required = required
	}

	for _, it := range secretUses {
		name := strings.TrimSpace(it.Name)
		if name == "" {
			continue
		}
		p, ok := paramsMap[name]
		if ok && p.Kind != AppParamKindSecret {
			continue
		}
		var sp *AppParam
		if !ok {
			sp = &AppParam{Key: name, Kind: AppParamKindSecret, Required: true}
			paramsMap[name] = sp
		} else {
			sp = p
			sp.Kind = AppParamKindSecret
			sp.Required = true
		}
		addUsage(sp, AppParamUsageSecretMount)
		addSource(sp, AppParamSourceComposeSecret)
		def, hasDef := secretDefs[name]
		if hasDef {
			if def.External {
				addBinding(sp, AppParamBinding{ServiceName: it.Service, Target: "secret_external", File: name, Required: true})
			} else {
				addBinding(sp, AppParamBinding{ServiceName: it.Service, Target: "secret_file", File: def.File, Required: true})
			}
		} else {
			addBinding(sp, AppParamBinding{ServiceName: it.Service, Target: "secret", File: "", Required: true})
		}
	}

	params := make([]AppParam, 0, len(paramsMap))
	for _, p := range paramsMap {
		params = append(params, *p)
	}
	sort.SliceStable(params, func(i, j int) bool {
		a := params[i]
		b := params[j]
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		if a.Required != b.Required {
			return a.Required
		}
		return a.Key < b.Key
	})
	return params, warnings
}

func parseAppStoreVars(c *gin.Context) {
	// JSON escaping and field names add overhead beyond the compiler's raw bundle limit.
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, templatecompiler.MaxBundleBytes+(1<<20))
	var req struct {
		Compose       string                                `json:"compose"`
		Dotenv        string                                `json:"dotenv"`
		SourceFiles   []templatecompiler.SourceFile         `json:"source_files"`
		InputMetadata templatecompiler.PresentationMetadata `json:"input_metadata"`
		Schema        []Variable                            `json:"schema"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "无效的请求数据", err)
		return
	}

	app := &App{
		Compose:       req.Compose,
		Dotenv:        req.Dotenv,
		SourceFiles:   req.SourceFiles,
		InputMetadata: req.InputMetadata,
		Schema:        req.Schema,
	}
	response, err := buildAppStoreManifestVars(app)
	if err != nil {
		respondError(c, http.StatusBadRequest, "解析应用配置失败", err)
		return
	}
	c.JSON(http.StatusOK, response)
}

type envFileRef struct {
	Path     string
	Required bool
}

func isLikelyEnvKey(key string) bool {
	if key == "" {
		return false
	}
	b0 := key[0]
	if !((b0 >= 'A' && b0 <= 'Z') || (b0 >= 'a' && b0 <= 'z') || b0 == '_') {
		return false
	}
	for i := 1; i < len(key); i++ {
		b := key[i]
		if (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9') || b == '_' {
			continue
		}
		return false
	}
	return true
}

func isSelfEnvPlaceholder(key, val string) bool {
	k := strings.TrimSpace(key)
	v := strings.TrimSpace(val)
	if k == "" || v == "" {
		return false
	}
	if len(v) >= 2 {
		if (v[0] == '"' && v[len(v)-1] == '"') || (v[0] == '\'' && v[len(v)-1] == '\'') {
			v = strings.TrimSpace(v[1 : len(v)-1])
		}
	}
	if v == "["+k+"]" {
		return true
	}
	prefix := "${" + k
	if !strings.HasPrefix(v, prefix) || !strings.HasSuffix(v, "}") {
		return false
	}
	if len(v) == len(prefix)+1 && v[len(prefix)] == '}' {
		return true
	}
	if len(v) > len(prefix)+1 {
		next := v[len(prefix)]
		if next == ':' || next == '-' || next == '?' || next == '+' {
			return true
		}
	}
	return false
}

func sanitizeDotenvText(dotenvText string, fallbackDotenv string) string {
	src := string(dotenvText)
	fallbackMap := parseDotenvToMap(fallbackDotenv)
	if len(fallbackMap) == 0 {
		return src
	}

	lines := strings.Split(src, "\n")
	for i := 0; i < len(lines); i++ {
		raw := lines[i]
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		prefix := ""
		line := trimmed
		if strings.HasPrefix(line, "export ") {
			prefix = "export "
			line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		}

		idx := strings.Index(line, "=")
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])
		if key == "" {
			continue
		}
		if isSelfEnvPlaceholder(key, val) {
			if fb, ok := fallbackMap[key]; ok {
				lines[i] = prefix + key + "=" + fb
			}
		}
	}
	return strings.Join(lines, "\n")
}

func parseDotenvToMap(content string) map[string]string {
	out := make(map[string]string)
	lines := strings.Split(content, "\n")
	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "export ") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		}
		idx := strings.Index(line, "=")
		if idx < 0 {
			key := strings.TrimSpace(line)
			if key == "" {
				continue
			}
			out[key] = ""
			continue
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])
		if key == "" {
			continue
		}
		if len(val) >= 2 {
			if (val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'') {
				val = val[1 : len(val)-1]
			}
		}
		out[key] = val
	}
	return out
}

type composeVarRef struct {
	HasDefault   bool
	DefaultValue string
	Raw          string
	Examples     []string
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func pushLimitedUnique(list []string, v string, limit int) []string {
	s := strings.TrimSpace(v)
	if s == "" {
		return list
	}
	for _, it := range list {
		if it == s {
			return list
		}
	}
	if len(list) >= limit {
		return list
	}
	return append(list, s)
}

func extractComposeVarRefs(composeContent string) map[string]composeVarRef {
	const maxComposeScanLen = 2_000_000
	const maxVars = 500

	s := composeContent
	if len(s) > maxComposeScanLen {
		s = s[:maxComposeScanLen]
	}
	out := make(map[string]composeVarRef)

	push := func(name string, hasDefault bool, def string, raw string) {
		key := strings.TrimSpace(name)
		if key == "" || !isLikelyEnvKey(key) || strings.EqualFold(key, "PATH") {
			return
		}
		if _, exists := out[key]; !exists && len(out) >= maxVars {
			return
		}
		ref := out[key]
		if ref.Raw == "" {
			ref.Raw = raw
		}
		if hasDefault && !ref.HasDefault {
			ref.HasDefault = true
			ref.DefaultValue = def
		}
		if hasDefault && ref.HasDefault && strings.TrimSpace(ref.DefaultValue) == "" && strings.TrimSpace(def) != "" {
			ref.DefaultValue = def
		}
		ref.Examples = pushLimitedUnique(ref.Examples, raw, 3)
		out[key] = ref
	}

	for i := 0; i < len(s); i++ {
		if s[i] != '$' {
			continue
		}
		if i+1 >= len(s) {
			continue
		}
		next := s[i+1]
		if next == '$' {
			i++
			continue
		}
		if next == '{' {
			end := strings.IndexByte(s[i+2:], '}')
			if end < 0 {
				continue
			}
			end = i + 2 + end
			inner := s[i+2 : end]
			raw := s[i : end+1]
			name, hasDefault, def := parseComposeInterpolationInner(inner)
			push(name, hasDefault, def, raw)
			i = end
			continue
		}
		if (next >= 'A' && next <= 'Z') || (next >= 'a' && next <= 'z') || next == '_' {
			j := i + 1
			for j < len(s) {
				b := s[j]
				if (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9') || b == '_' {
					j++
					continue
				}
				break
			}
			name := s[i+1 : j]
			push(name, false, "", "$"+name)
			i = j - 1
			continue
		}
	}

	return out
}

func parseComposeInterpolationInner(inner string) (name string, hasDefault bool, def string) {
	inner = strings.TrimSpace(inner)
	if inner == "" {
		return "", false, ""
	}
	operators := []string{":-", "-", ":?", "?", ":+", "+"}
	bestIdx := -1
	bestOp := ""
	for _, op := range operators {
		idx := strings.Index(inner, op)
		if idx < 0 {
			continue
		}
		if bestIdx < 0 || idx < bestIdx || (idx == bestIdx && len(op) > len(bestOp)) {
			bestIdx = idx
			bestOp = op
		}
	}
	if bestIdx < 0 {
		return strings.TrimSpace(inner), false, ""
	}
	name = strings.TrimSpace(inner[:bestIdx])
	value := strings.TrimSpace(inner[bestIdx+len(bestOp):])
	switch bestOp {
	case ":-", "-":
		return name, true, value
	default:
		return name, false, ""
	}
}

func isSchemaEnvVariable(v Variable) bool {
	pt := strings.ToLower(strings.TrimSpace(v.ParamType))
	if pt == "env" || pt == "environment" {
		return true
	}
	if pt != "" {
		return false
	}
	t := strings.ToLower(strings.TrimSpace(v.Type))
	if t == "port" || t == "path" {
		return false
	}
	return true
}

func extractComposeInterpolationKeys(composeContent string) map[string]struct{} {
	out := make(map[string]struct{})
	for k := range extractComposeVarRefs(composeContent) {
		out[k] = struct{}{}
	}
	return out
}

func filterDotenvByAllowedKeys(dotenvText string, allowed map[string]struct{}) string {
	if len(allowed) == 0 {
		return ""
	}

	lines := strings.Split(dotenvText, "\n")
	out := make([]string, 0, len(lines))
	for _, raw := range lines {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			out = append(out, raw)
			continue
		}

		line := trimmed
		if strings.HasPrefix(line, "export ") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		}

		idx := strings.Index(line, "=")
		key := ""
		if idx < 0 {
			key = strings.TrimSpace(line)
		} else {
			key = strings.TrimSpace(line[:idx])
		}
		if key == "" {
			continue
		}
		if _, ok := allowed[key]; !ok {
			continue
		}
		out = append(out, raw)
	}
	return strings.Join(out, "\n")
}

func renderDotenvFromMap(env map[string]string) string {
	if len(env) == 0 {
		return ""
	}
	var b strings.Builder
	for k, v := range env {
		if strings.TrimSpace(k) == "" {
			continue
		}
		b.WriteString(k)
		b.WriteString("=")
		b.WriteString(v)
		b.WriteString("\n")
	}
	return b.String()
}

func renderDotenvFromMapStable(env map[string]string) string {
	if len(env) == 0 {
		return ""
	}
	keys := make([]string, 0, len(env))
	for k := range env {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		b.WriteString(k)
		b.WriteString("=")
		b.WriteString(env[k])
		b.WriteString("\n")
	}
	return b.String()
}

func extractEnvFileRefs(composeContent string) ([]envFileRef, error) {
	var data map[string]interface{}
	if err := yaml.Unmarshal([]byte(composeContent), &data); err != nil {
		return nil, err
	}
	services, ok := data["services"].(map[string]interface{})
	if !ok {
		return nil, nil
	}

	merged := make(map[string]bool)
	for _, serviceRaw := range services {
		svc, ok := serviceRaw.(map[string]interface{})
		if !ok {
			continue
		}
		envFileRaw, ok := svc["env_file"]
		if !ok {
			continue
		}
		addRef := func(p string, required bool) {
			p = strings.TrimSpace(p)
			if p == "" {
				return
			}
			if prev, ok := merged[p]; ok {
				merged[p] = prev || required
				return
			}
			merged[p] = required
		}
		switch v := envFileRaw.(type) {
		case string:
			addRef(v, true)
		case []interface{}:
			for _, item := range v {
				switch it := item.(type) {
				case string:
					addRef(it, true)
				case map[string]interface{}:
					pathVal, _ := it["path"]
					reqVal, hasReq := it["required"]
					pathStr := strings.TrimSpace(fmt.Sprintf("%v", pathVal))
					required := true
					if hasReq {
						if b, ok := reqVal.(bool); ok {
							required = b
						} else {
							s := strings.ToLower(strings.TrimSpace(fmt.Sprintf("%v", reqVal)))
							if s == "false" || s == "0" || s == "no" {
								required = false
							}
						}
					}
					addRef(pathStr, required)
				default:
					addRef(fmt.Sprintf("%v", it), true)
				}
			}
		default:
			addRef(fmt.Sprintf("%v", v), true)
		}
	}

	out := make([]envFileRef, 0, len(merged))
	for p, required := range merged {
		out = append(out, envFileRef{Path: p, Required: required})
	}
	return out, nil
}

func extractServiceEnvFileRefs(composeContent string) (map[string][]envFileRef, error) {
	var data map[string]interface{}
	if err := yaml.Unmarshal([]byte(composeContent), &data); err != nil {
		return nil, err
	}
	services, ok := data["services"].(map[string]interface{})
	if !ok {
		return map[string][]envFileRef{}, nil
	}

	out := make(map[string][]envFileRef)
	for svcName, serviceRaw := range services {
		svc, ok := serviceRaw.(map[string]interface{})
		if !ok {
			continue
		}
		envFileRaw, ok := svc["env_file"]
		if !ok {
			continue
		}

		merged := make(map[string]bool)
		addRef := func(p string, required bool) {
			p = strings.TrimSpace(p)
			if p == "" {
				return
			}
			if prev, ok := merged[p]; ok {
				merged[p] = prev || required
				return
			}
			merged[p] = required
		}
		switch v := envFileRaw.(type) {
		case string:
			addRef(v, true)
		case []interface{}:
			for _, item := range v {
				switch it := item.(type) {
				case string:
					addRef(it, true)
				case map[string]interface{}:
					pathVal, _ := it["path"]
					reqVal, hasReq := it["required"]
					pathStr := strings.TrimSpace(fmt.Sprintf("%v", pathVal))
					required := true
					if hasReq {
						if b, ok := reqVal.(bool); ok {
							required = b
						} else {
							s := strings.ToLower(strings.TrimSpace(fmt.Sprintf("%v", reqVal)))
							if s == "false" || s == "0" || s == "no" {
								required = false
							}
						}
					}
					addRef(pathStr, required)
				default:
					addRef(fmt.Sprintf("%v", it), true)
				}
			}
		default:
			addRef(fmt.Sprintf("%v", v), true)
		}

		list := make([]envFileRef, 0, len(merged))
		for p, required := range merged {
			list = append(list, envFileRef{Path: p, Required: required})
		}
		sort.SliceStable(list, func(i, j int) bool { return list[i].Path < list[j].Path })
		name := strings.TrimSpace(svcName)
		if name == "" {
			name = "Global"
		}
		out[name] = list
	}

	return out, nil
}

type composeSecretDef struct {
	Name     string
	File     string
	External bool
}

type composeSecretUse struct {
	Service string
	Name    string
	Target  string
}

func extractComposeSecrets(composeContent string) (map[string]composeSecretDef, []composeSecretUse, error) {
	var data map[string]interface{}
	if err := yaml.Unmarshal([]byte(composeContent), &data); err != nil {
		return nil, nil, err
	}

	defs := make(map[string]composeSecretDef)
	if secretsAny, ok := data["secrets"]; ok {
		if secretsMap, ok := secretsAny.(map[string]interface{}); ok {
			for nameAny, defAny := range secretsMap {
				name := strings.TrimSpace(fmt.Sprintf("%v", nameAny))
				if name == "" {
					continue
				}
				def := composeSecretDef{Name: name}
				if m, ok := defAny.(map[string]interface{}); ok {
					if f, ok := m["file"]; ok {
						def.File = strings.TrimSpace(fmt.Sprintf("%v", f))
					}
					if ex, ok := m["external"]; ok {
						switch v := ex.(type) {
						case bool:
							def.External = v
						default:
							s := strings.ToLower(strings.TrimSpace(fmt.Sprintf("%v", v)))
							def.External = s == "true" || s == "1" || s == "yes"
						}
					}
				}
				defs[name] = def
			}
		}
	}

	uses := make([]composeSecretUse, 0)
	servicesAny, ok := data["services"]
	if !ok {
		return defs, uses, nil
	}
	services, ok := servicesAny.(map[string]interface{})
	if !ok {
		return defs, uses, nil
	}
	for svcNameAny, svcAny := range services {
		svcName := strings.TrimSpace(fmt.Sprintf("%v", svcNameAny))
		if svcName == "" {
			continue
		}
		svc, ok := svcAny.(map[string]interface{})
		if !ok {
			continue
		}
		secAny, ok := svc["secrets"]
		if !ok {
			continue
		}

		addUse := func(name string, target string) {
			name = strings.TrimSpace(name)
			if name == "" {
				return
			}
			uses = append(uses, composeSecretUse{Service: svcName, Name: name, Target: strings.TrimSpace(target)})
		}

		switch v := secAny.(type) {
		case []interface{}:
			for _, it := range v {
				switch item := it.(type) {
				case string:
					addUse(item, "")
				case map[string]interface{}:
					source := strings.TrimSpace(fmt.Sprintf("%v", item["source"]))
					if source == "" {
						source = strings.TrimSpace(fmt.Sprintf("%v", item["secret"]))
					}
					target := strings.TrimSpace(fmt.Sprintf("%v", item["target"]))
					addUse(source, target)
				default:
					addUse(fmt.Sprintf("%v", item), "")
				}
			}
		case string:
			addUse(v, "")
		default:
			addUse(fmt.Sprintf("%v", v), "")
		}
	}

	return defs, uses, nil
}

// validateComposeAssetPaths 校验 Compose 中 env_file/extends.file/secrets.file 的路径安全性。
// 返回人类可读的错误列表；空列表表示通过校验。
func validateComposeAssetPaths(composeContent string) []string {
	var errs []string

	envRefs, err := extractServiceEnvFileRefs(composeContent)
	if err != nil {
		errs = append(errs, fmt.Sprintf("解析 env_file 失败: %v", err))
	} else {
		seen := make(map[string]bool)
		for _, refs := range envRefs {
			for _, ref := range refs {
				orig := strings.TrimSpace(ref.Path)
				if orig == "" || seen[orig] {
					continue
				}
				seen[orig] = true
				clean := filepath.Clean(orig)
				if filepath.IsAbs(clean) || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || clean == ".." {
					errs = append(errs, fmt.Sprintf("env_file 路径不安全: %s", orig))
				}
			}
		}
	}

	secretDefs, _, err := extractComposeSecrets(composeContent)
	if err != nil {
		errs = append(errs, fmt.Sprintf("解析 secrets 失败: %v", err))
	} else {
		for _, def := range secretDefs {
			if def.External {
				continue
			}
			orig := strings.TrimSpace(def.File)
			if orig == "" {
				continue
			}
			clean := filepath.Clean(orig)
			if filepath.IsAbs(clean) || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || clean == ".." {
				errs = append(errs, fmt.Sprintf("secret file 路径不安全: %s", orig))
			}
		}
	}

	errs = append(errs, detectCrossFileExtends(composeContent)...)
	return errs
}

// validateGitImportedComposeAssetPaths validates assets relative to the selected
// Compose directory while keeping every resolved path inside the imported project.
func validateGitImportedComposeAssetPaths(projectDir, composeDir, composeContent string) []string {
	projectDir, err := filepath.Abs(projectDir)
	if err != nil {
		return []string{fmt.Sprintf("解析项目目录失败: %v", err)}
	}
	composeDir, err = filepath.Abs(composeDir)
	if err != nil {
		return []string{fmt.Sprintf("解析 Compose 目录失败: %v", err)}
	}
	if relativeDir, err := filepath.Rel(projectDir, composeDir); err != nil || relativeDir == ".." || strings.HasPrefix(relativeDir, ".."+string(filepath.Separator)) {
		return []string{"Compose 目录不在项目目录内"}
	}

	var errs []string
	envRefs, err := extractServiceEnvFileRefs(composeContent)
	if err != nil {
		errs = append(errs, fmt.Sprintf("解析 env_file 失败: %v", err))
	} else {
		seen := make(map[string]bool)
		for _, refs := range envRefs {
			for _, ref := range refs {
				orig := strings.TrimSpace(ref.Path)
				if orig == "" || seen[orig] {
					continue
				}
				seen[orig] = true
				if !gitImportedComposeAssetPathIsSafe(projectDir, composeDir, orig) {
					errs = append(errs, fmt.Sprintf("env_file 路径不安全: %s", orig))
				}
			}
		}
	}

	secretDefs, _, err := extractComposeSecrets(composeContent)
	if err != nil {
		errs = append(errs, fmt.Sprintf("解析 secrets 失败: %v", err))
	} else {
		for _, def := range secretDefs {
			if def.External {
				continue
			}
			orig := strings.TrimSpace(def.File)
			if orig != "" && !gitImportedComposeAssetPathIsSafe(projectDir, composeDir, orig) {
				errs = append(errs, fmt.Sprintf("secret file 路径不安全: %s", orig))
			}
		}
	}

	errs = append(errs, detectCrossFileExtends(composeContent)...)
	return errs
}

func gitImportedComposeAssetPathIsSafe(projectDir, composeDir, assetPath string) bool {
	cleanPath := filepath.Clean(strings.TrimSpace(assetPath))
	if filepath.IsAbs(cleanPath) {
		return false
	}
	resolvedPath := filepath.Clean(filepath.Join(composeDir, cleanPath))
	relativePath, err := filepath.Rel(projectDir, resolvedPath)
	if err != nil || relativePath == ".." || strings.HasPrefix(relativePath, ".."+string(filepath.Separator)) {
		return false
	}

	currentPath := projectDir
	info, err := os.Lstat(currentPath)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return false
	}
	components := strings.Split(relativePath, string(filepath.Separator))
	for index, component := range components {
		if component == "." || component == "" {
			continue
		}
		currentPath = filepath.Join(currentPath, component)
		info, err = os.Lstat(currentPath)
		if os.IsNotExist(err) {
			return true
		}
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			return false
		}
		if index < len(components)-1 && !info.IsDir() {
			return false
		}
		if index == len(components)-1 && !info.IsDir() && !info.Mode().IsRegular() {
			return false
		}
	}
	return true
}

func detectCrossFileExtends(composeContent string) []string {
	var data map[string]interface{}
	if err := yaml.Unmarshal([]byte(composeContent), &data); err != nil {
		return nil
	}
	servicesAny, ok := data["services"]
	if !ok {
		return nil
	}
	services, ok := servicesAny.(map[string]interface{})
	if !ok {
		return nil
	}
	var errs []string
	for svcNameAny, svcAny := range services {
		svcName := strings.TrimSpace(fmt.Sprintf("%v", svcNameAny))
		if svcName == "" {
			continue
		}
		svc, ok := svcAny.(map[string]interface{})
		if !ok {
			continue
		}
		extAny, ok := svc["extends"]
		if !ok {
			continue
		}
		if m, ok := extAny.(map[string]interface{}); ok {
			fileVal := strings.TrimSpace(fmt.Sprintf("%v", m["file"]))
			if fileVal != "" {
				errs = append(errs, fmt.Sprintf("检测到跨文件 extends：service=%s extends.file=%s（当前仅支持单文件模板）", svcName, fileVal))
			}
		}
	}
	return errs
}

func removeDotenvEnvFileRefs(composeContent string) (string, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(composeContent), &doc); err != nil {
		return "", err
	}
	if len(doc.Content) == 0 {
		return composeContent, nil
	}

	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return composeContent, nil
	}

	findMapValue := func(m *yaml.Node, key string) *yaml.Node {
		if m == nil || m.Kind != yaml.MappingNode {
			return nil
		}
		for i := 0; i+1 < len(m.Content); i += 2 {
			k := m.Content[i]
			v := m.Content[i+1]
			if k != nil && k.Kind == yaml.ScalarNode && k.Value == key {
				return v
			}
		}
		return nil
	}
	deleteMapKey := func(m *yaml.Node, key string) {
		if m == nil || m.Kind != yaml.MappingNode {
			return
		}
		next := make([]*yaml.Node, 0, len(m.Content))
		for i := 0; i+1 < len(m.Content); i += 2 {
			k := m.Content[i]
			v := m.Content[i+1]
			if k != nil && k.Kind == yaml.ScalarNode && k.Value == key {
				continue
			}
			next = append(next, k, v)
		}
		m.Content = next
	}

	services := findMapValue(root, "services")
	if services == nil || services.Kind != yaml.MappingNode {
		return composeContent, nil
	}

	for i := 0; i+1 < len(services.Content); i += 2 {
		svcVal := services.Content[i+1]
		if svcVal == nil || svcVal.Kind != yaml.MappingNode {
			continue
		}
		envFile := findMapValue(svcVal, "env_file")
		if envFile == nil {
			continue
		}
		switch envFile.Kind {
		case yaml.ScalarNode:
			if strings.TrimSpace(envFile.Value) == ".env" {
				deleteMapKey(svcVal, "env_file")
			}
		case yaml.SequenceNode:
			nextItems := make([]*yaml.Node, 0, len(envFile.Content))
			for _, it := range envFile.Content {
				if it == nil {
					continue
				}
				if it.Kind == yaml.ScalarNode {
					if strings.TrimSpace(it.Value) == ".env" {
						continue
					}
					nextItems = append(nextItems, it)
					continue
				}
				if it.Kind == yaml.MappingNode {
					pathNode := findMapValue(it, "path")
					if pathNode != nil && pathNode.Kind == yaml.ScalarNode && strings.TrimSpace(pathNode.Value) == ".env" {
						continue
					}
					nextItems = append(nextItems, it)
					continue
				}
				nextItems = append(nextItems, it)
			}
			if len(nextItems) == 0 {
				deleteMapKey(svcVal, "env_file")
			} else {
				envFile.Content = nextItems
			}
		}
	}

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		_ = enc.Close()
		return "", err
	}
	_ = enc.Close()
	return buf.String(), nil
}

func removePlaceholderEnvVars(composeContent string, knownDotenvKeys map[string]struct{}, keepDotenvKeys map[string]struct{}) (string, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(composeContent), &doc); err != nil {
		return "", err
	}
	if len(doc.Content) == 0 {
		return composeContent, nil
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return composeContent, nil
	}

	findMapValue := func(m *yaml.Node, key string) *yaml.Node {
		if m == nil || m.Kind != yaml.MappingNode {
			return nil
		}
		for i := 0; i+1 < len(m.Content); i += 2 {
			k := m.Content[i]
			v := m.Content[i+1]
			if k != nil && k.Kind == yaml.ScalarNode && k.Value == key {
				return v
			}
		}
		return nil
	}
	deleteMapKey := func(m *yaml.Node, key string) {
		if m == nil || m.Kind != yaml.MappingNode {
			return
		}
		next := make([]*yaml.Node, 0, len(m.Content))
		for i := 0; i+1 < len(m.Content); i += 2 {
			k := m.Content[i]
			v := m.Content[i+1]
			if k != nil && k.Kind == yaml.ScalarNode && k.Value == key {
				continue
			}
			next = append(next, k, v)
		}
		m.Content = next
	}

	extractPlaceholders := func(val string) []string {
		val = strings.TrimSpace(val)
		if len(val) >= 2 {
			if (val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'') {
				val = strings.TrimSpace(val[1 : len(val)-1])
			}
		}
		out := make([]string, 0, 2)
		reBracket := regexp.MustCompile(`\[\s*([A-Za-z_][A-Za-z0-9_]*)\s*\]`)
		for _, m := range reBracket.FindAllStringSubmatch(val, -1) {
			if len(m) >= 2 {
				out = append(out, strings.TrimSpace(m[1]))
			}
		}
		reInterp := regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)`)
		for _, m := range reInterp.FindAllStringSubmatch(val, -1) {
			if len(m) >= 2 {
				out = append(out, strings.TrimSpace(m[1]))
			}
		}
		return out
	}
	shouldRemove := func(val string) bool {
		for _, ph := range extractPlaceholders(val) {
			if ph == "" {
				continue
			}
			if _, known := knownDotenvKeys[ph]; !known {
				continue
			}
			if _, keep := keepDotenvKeys[ph]; keep {
				continue
			}
			return true
		}
		return false
	}

	services := findMapValue(root, "services")
	if services == nil || services.Kind != yaml.MappingNode {
		return composeContent, nil
	}

	for i := 0; i+1 < len(services.Content); i += 2 {
		svcVal := services.Content[i+1]
		if svcVal == nil || svcVal.Kind != yaml.MappingNode {
			continue
		}
		envNode := findMapValue(svcVal, "environment")
		if envNode == nil {
			continue
		}
		switch envNode.Kind {
		case yaml.MappingNode:
			next := make([]*yaml.Node, 0, len(envNode.Content))
			for j := 0; j+1 < len(envNode.Content); j += 2 {
				kNode := envNode.Content[j]
				vNode := envNode.Content[j+1]
				if kNode == nil || vNode == nil || kNode.Kind != yaml.ScalarNode {
					next = append(next, kNode, vNode)
					continue
				}
				if vNode.Kind == yaml.ScalarNode && shouldRemove(vNode.Value) {
					continue
				}
				next = append(next, kNode, vNode)
			}
			if len(next) == 0 {
				deleteMapKey(svcVal, "environment")
			} else {
				envNode.Content = next
			}
		case yaml.SequenceNode:
			nextItems := make([]*yaml.Node, 0, len(envNode.Content))
			for _, it := range envNode.Content {
				if it == nil || it.Kind != yaml.ScalarNode {
					nextItems = append(nextItems, it)
					continue
				}
				s := strings.TrimSpace(it.Value)
				if s == "" || strings.HasPrefix(s, "#") {
					nextItems = append(nextItems, it)
					continue
				}
				parts := strings.SplitN(s, "=", 2)
				if len(parts) != 2 {
					nextItems = append(nextItems, it)
					continue
				}
				val := strings.TrimSpace(parts[1])
				if shouldRemove(val) {
					continue
				}
				nextItems = append(nextItems, it)
			}
			if len(nextItems) == 0 {
				deleteMapKey(svcVal, "environment")
			} else {
				envNode.Content = nextItems
			}
		}
	}

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		_ = enc.Close()
		return "", err
	}
	_ = enc.Close()
	return buf.String(), nil
}

const (
	fixedAssetsDir   = ".tradis"
	fixedEnvFilesDir = ".tradis/env-files"
	fixedSecretsDir  = ".tradis/secrets"
)

func safeWriteFileRelative(baseDir string, relPath string, content []byte, perm os.FileMode) error {
	clean := filepath.Clean(relPath)
	if filepath.IsAbs(clean) || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || clean == ".." {
		return fmt.Errorf("路径不安全: %s", relPath)
	}
	full := filepath.Join(baseDir, clean)
	if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
		return err
	}
	return os.WriteFile(full, content, perm)
}

func appendDotenvMissing(dotenvText string, kv map[string]string) string {
	existing := parseDotenvToMap(dotenvText)
	lines := make([]string, 0, len(kv))
	for k, v := range kv {
		key := strings.TrimSpace(k)
		if key == "" || !isLikelyEnvKey(key) || strings.EqualFold(key, "PATH") {
			continue
		}
		if _, ok := existing[key]; ok {
			continue
		}
		lines = append(lines, fmt.Sprintf("%s=%s", key, v))
	}
	if len(lines) == 0 {
		return dotenvText
	}
	sort.Strings(lines)
	out := strings.TrimRight(dotenvText, "\n")
	if out != "" {
		out += "\n"
	}
	out += strings.Join(lines, "\n") + "\n"
	return out
}

func upsertDotenvValues(dotenvText string, kv map[string]string) string {
	if len(kv) == 0 {
		return dotenvText
	}

	next := make(map[string]string, len(kv))
	for k, v := range kv {
		key := strings.TrimSpace(k)
		if key == "" || !isLikelyEnvKey(key) || strings.EqualFold(key, "PATH") {
			continue
		}
		next[key] = v
	}
	if len(next) == 0 {
		return dotenvText
	}

	seen := make(map[string]struct{}, len(next))
	lines := strings.Split(strings.ReplaceAll(dotenvText, "\r\n", "\n"), "\n")
	for i, raw := range lines {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		prefix := ""
		line := trimmed
		if strings.HasPrefix(line, "export ") {
			prefix = "export "
			line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		}

		idx := strings.Index(line, "=")
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		val, ok := next[key]
		if !ok {
			continue
		}
		lines[i] = fmt.Sprintf("%s%s=%s", prefix, key, val)
		seen[key] = struct{}{}
	}

	missing := make([]string, 0, len(next))
	for key, val := range next {
		if _, ok := seen[key]; ok {
			continue
		}
		missing = append(missing, fmt.Sprintf("%s=%s", key, val))
	}
	sort.Strings(missing)

	out := strings.TrimRight(strings.Join(lines, "\n"), "\n")
	if len(missing) > 0 {
		if out != "" {
			out += "\n"
		}
		out += strings.Join(missing, "\n")
	}
	if out != "" {
		out += "\n"
	}
	return out
}

func buildFixedEnvFilePathMap(envFiles []envFileRef) (map[string]string, error) {
	m := make(map[string]string)
	for _, ref := range envFiles {
		orig := strings.TrimSpace(ref.Path)
		if orig == "" {
			continue
		}
		clean := filepath.Clean(orig)
		if clean == ".env" || strings.HasSuffix(clean, string(filepath.Separator)+".env") {
			m[clean] = ".env"
			continue
		}
		if filepath.IsAbs(clean) || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || clean == ".." {
			return nil, fmt.Errorf("env_file 路径不安全: %s", orig)
		}
		base := filepath.Base(clean)
		if base == "" || base == "." || base == string(filepath.Separator) {
			return nil, fmt.Errorf("env_file 路径无效: %s", orig)
		}
		sum := crc32.ChecksumIEEE([]byte(clean))
		target := fmt.Sprintf("%s/%s-%08x.env", fixedEnvFilesDir, base, sum)
		m[clean] = target
	}
	return m, nil
}

func buildFixedSecretFilePathMap(defs map[string]composeSecretDef) map[string]string {
	m := make(map[string]string)
	for name, def := range defs {
		if def.External {
			continue
		}
		if strings.TrimSpace(def.File) == "" {
			continue
		}
		n := strings.TrimSpace(name)
		if n == "" {
			continue
		}
		m[n] = fmt.Sprintf("%s/%s", fixedSecretsDir, n)
	}
	return m
}

func rewriteComposeFixedAssetPaths(composeContent string, envFilePathMap map[string]string, secretFilePathMap map[string]string) (string, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(composeContent), &doc); err != nil {
		return "", err
	}
	if len(doc.Content) == 0 {
		return composeContent, nil
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return composeContent, nil
	}

	findMapValue := func(m *yaml.Node, key string) *yaml.Node {
		if m == nil || m.Kind != yaml.MappingNode {
			return nil
		}
		for i := 0; i+1 < len(m.Content); i += 2 {
			k := m.Content[i]
			v := m.Content[i+1]
			if k != nil && k.Kind == yaml.ScalarNode && k.Value == key {
				return v
			}
		}
		return nil
	}

	services := findMapValue(root, "services")
	if services != nil && services.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(services.Content); i += 2 {
			svcVal := services.Content[i+1]
			if svcVal == nil || svcVal.Kind != yaml.MappingNode {
				continue
			}
			envFile := findMapValue(svcVal, "env_file")
			if envFile == nil {
				continue
			}
			switch envFile.Kind {
			case yaml.ScalarNode:
				orig := strings.TrimSpace(envFile.Value)
				clean := filepath.Clean(orig)
				if mapped, ok := envFilePathMap[clean]; ok && mapped != "" {
					envFile.Value = mapped
				}
			case yaml.SequenceNode:
				for _, it := range envFile.Content {
					if it == nil {
						continue
					}
					if it.Kind == yaml.ScalarNode {
						orig := strings.TrimSpace(it.Value)
						clean := filepath.Clean(orig)
						if mapped, ok := envFilePathMap[clean]; ok && mapped != "" {
							it.Value = mapped
						}
						continue
					}
					if it.Kind == yaml.MappingNode {
						pathNode := findMapValue(it, "path")
						if pathNode != nil && pathNode.Kind == yaml.ScalarNode {
							orig := strings.TrimSpace(pathNode.Value)
							clean := filepath.Clean(orig)
							if mapped, ok := envFilePathMap[clean]; ok && mapped != "" {
								pathNode.Value = mapped
							}
						}
					}
				}
			}
		}
	}

	secrets := findMapValue(root, "secrets")
	if secrets != nil && secrets.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(secrets.Content); i += 2 {
			nameNode := secrets.Content[i]
			defNode := secrets.Content[i+1]
			if nameNode == nil || nameNode.Kind != yaml.ScalarNode || defNode == nil {
				continue
			}
			name := strings.TrimSpace(nameNode.Value)
			target := strings.TrimSpace(secretFilePathMap[name])
			if target == "" {
				continue
			}
			if defNode.Kind == yaml.MappingNode {
				fileNode := findMapValue(defNode, "file")
				if fileNode != nil && fileNode.Kind == yaml.ScalarNode {
					fileNode.Value = target
				}
			}
		}
	}

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		_ = enc.Close()
		return "", err
	}
	_ = enc.Close()
	return buf.String(), nil
}
