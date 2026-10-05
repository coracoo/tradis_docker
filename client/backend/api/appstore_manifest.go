package api

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"dockerpanel/backend/internal/templatecompiler"
)

func appSourceBundle(app *App) templatecompiler.SourceBundle {
	if app == nil {
		return templatecompiler.SourceBundle{}
	}
	return templatecompiler.SourceBundle{
		ComposePath: "compose.yaml",
		Compose:     app.Compose,
		Dotenv:      app.Dotenv,
		Files:       append([]templatecompiler.SourceFile(nil), app.SourceFiles...),
	}
}

func resolveAppStoreManifest(app *App) (templatecompiler.SourceBundle, templatecompiler.Manifest, []templatecompiler.Diagnostic, error) {
	if app == nil {
		return templatecompiler.SourceBundle{}, templatecompiler.Manifest{}, nil, fmt.Errorf("app is nil")
	}
	bundle := appSourceBundle(app)

	// Server 维护的分类（InputMetadata 或 legacy schema）优先：即使存量 Manifest
	// 自洽，空展示元数据也会在编译时补齐，部署侧重放使用同一确定性结果。
	metadata := app.InputMetadata
	if metadata == nil {
		metadata = exactLegacyInputMetadata(app.Schema)
	}
	if len(metadata) == 0 && len(app.Schema) > 0 {
		// 旧 Server 的 schema 行没有 inputId，先编译一次拿到稳定 ID，再按
		// name/service/paramType/envFile 身份匹配回填分类。
		if base, err := templatecompiler.Compile(bundle, nil); err == nil {
			metadata = legacyIdentityMetadata(app.Schema, base.Manifest)
		}
	}
	if len(metadata) > 0 {
		result, err := templatecompiler.Compile(bundle, metadata)
		if err != nil {
			return bundle, templatecompiler.Manifest{}, nil, err
		}
		if err := incompleteManifestError(result.Manifest); err != nil {
			return bundle, templatecompiler.Manifest{}, result.Manifest.Diagnostics, err
		}
		return bundle, result.Manifest, warningDiagnostics(result.Manifest.Diagnostics), nil
	}

	if app.Manifest != nil &&
		(app.SourceDigest == "" || app.SourceDigest == app.Manifest.SourceDigest) &&
		(app.ManifestDigest == "" || app.ManifestDigest == app.Manifest.ManifestDigest) &&
		(app.CompilerVersion == "" || app.CompilerVersion == app.Manifest.CompilerVersion) {
		if err := templatecompiler.Verify(bundle, *app.Manifest); err == nil {
			if err := incompleteManifestError(*app.Manifest); err != nil {
				return bundle, templatecompiler.Manifest{}, app.Manifest.Diagnostics, err
			}
			return bundle, *app.Manifest, warningDiagnostics(app.Manifest.Diagnostics), nil
		}
	}

	result, err := templatecompiler.Compile(bundle, metadata)
	if err != nil {
		return bundle, templatecompiler.Manifest{}, nil, err
	}
	if err := incompleteManifestError(result.Manifest); err != nil {
		return bundle, templatecompiler.Manifest{}, result.Manifest.Diagnostics, err
	}
	return bundle, result.Manifest, warningDiagnostics(result.Manifest.Diagnostics), nil
}

func warningDiagnostics(diagnostics []templatecompiler.Diagnostic) []templatecompiler.Diagnostic {
	warnings := make([]templatecompiler.Diagnostic, 0)
	for _, diagnostic := range diagnostics {
		if diagnostic.Severity != "error" {
			warnings = append(warnings, diagnostic)
		}
	}
	return warnings
}

func manifestPresentationMetadata(manifest templatecompiler.Manifest) templatecompiler.PresentationMetadata {
	metadata := make(templatecompiler.PresentationMetadata, len(manifest.Inputs))
	for _, input := range manifest.Inputs {
		presentation := input.Presentation
		presentation.Group = templatecompiler.NormalizePresentationGroup(presentation.Group)
		metadata[input.ID] = presentation
	}
	return metadata
}

func exactLegacyInputMetadata(schema []Variable) templatecompiler.PresentationMetadata {
	metadata := make(templatecompiler.PresentationMetadata)
	for _, variable := range schema {
		id := strings.TrimSpace(variable.InputID)
		if id == "" {
			continue
		}
		metadata[id] = legacyPresentation(variable)
	}
	return metadata
}

func legacyIdentityMetadata(schema []Variable, base templatecompiler.Manifest) templatecompiler.PresentationMetadata {
	projected := templatecompiler.ProjectLegacySchema(base)
	metadata := make(templatecompiler.PresentationMetadata)
	for _, legacy := range schema {
		matches := make([]templatecompiler.LegacyVariable, 0, 1)
		for _, candidate := range projected {
			if legacy.InputID != "" {
				if legacy.InputID == candidate.InputID {
					matches = []templatecompiler.LegacyVariable{candidate}
					break
				}
				continue
			}
			if legacyIdentityMatches(legacy, candidate) {
				matches = append(matches, candidate)
			}
		}
		if len(matches) != 1 {
			continue
		}
		metadata[matches[0].InputID] = legacyPresentation(legacy)
	}
	return metadata
}

func legacyIdentityMatches(legacy Variable, candidate templatecompiler.LegacyVariable) bool {
	if legacy.Name != candidate.Name {
		return false
	}
	legacyService := strings.TrimSpace(legacy.ServiceName)
	if legacyService == "" {
		legacyService = "Global"
	}
	candidateService := strings.TrimSpace(candidate.ServiceName)
	if candidateService == "" {
		candidateService = "Global"
	}
	if !strings.EqualFold(legacyService, candidateService) {
		return false
	}
	if value := strings.TrimSpace(legacy.ParamType); value != "" && !legacyParamTypeMatches(value, candidate.ParamType) {
		return false
	}
	if value := strings.TrimSpace(legacy.EnvFile); value != "" && value != candidate.EnvFile {
		return false
	}
	return true
}

func legacyParamTypeMatches(legacy, candidate string) bool {
	if strings.EqualFold(legacy, candidate) {
		return true
	}
	switch strings.ToLower(legacy) {
	case "env":
		return strings.EqualFold(candidate, "secret")
	case "hardware":
		return strings.EqualFold(candidate, "device")
	}
	return false
}

func legacyPresentation(variable Variable) templatecompiler.InputPresentation {
	presentation := templatecompiler.InputPresentation{
		Label:       variable.Label,
		Description: variable.Description,
		Group:       variable.Category,
	}
	if strings.EqualFold(strings.TrimSpace(variable.Type), "password") {
		sensitive := true
		presentation.Sensitive = &sensitive
	}
	return presentation
}

// enrichAppDetailManifest 返回补齐展示元数据后的详情副本；失败时原样返回，
// 详情接口不因补齐失败而中断，缓存中的原始数据保持不被污染。
func enrichAppDetailManifest(app *App) *App {
	if app == nil || app.Manifest == nil {
		return app
	}
	_, manifest, _, err := resolveAppStoreManifest(app)
	if err != nil || manifest.ManifestDigest == app.ManifestDigest {
		return app
	}
	enriched := *app
	enriched.Manifest = &manifest
	enriched.ManifestDigest = manifest.ManifestDigest
	enriched.CompilerVersion = manifest.CompilerVersion
	return &enriched
}

func incompleteManifestError(manifest templatecompiler.Manifest) error {
	if manifest.Complete {
		return nil
	}
	codes := make([]string, 0)
	seen := make(map[string]struct{})
	for _, diagnostic := range manifest.Diagnostics {
		if diagnostic.Severity != "error" {
			continue
		}
		if _, exists := seen[diagnostic.Code]; exists {
			continue
		}
		seen[diagnostic.Code] = struct{}{}
		codes = append(codes, diagnostic.Code)
	}
	sort.Strings(codes)
	if len(codes) == 0 {
		codes = append(codes, "template_incomplete")
	}
	return fmt.Errorf("AppStore template is incomplete: %s", strings.Join(codes, ", "))
}

func writeAppDetailCache(app *App) error {
	if app == nil {
		return fmt.Errorf("app is nil")
	}
	name := strings.TrimSpace(app.Name)
	if name == "" || name != filepath.Base(name) || name == "." || name == ".." {
		return fmt.Errorf("invalid AppStore cache name")
	}
	if err := os.MkdirAll(getAppCacheDir(), 0755); err != nil {
		return err
	}
	raw, err := json.Marshal(app)
	if err != nil {
		return err
	}
	target := filepath.Join(getAppCacheDir(), name+".json")
	temporary, err := os.CreateTemp(getAppCacheDir(), "."+name+".json.*.tmp")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0644); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(raw); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryName, target)
}
