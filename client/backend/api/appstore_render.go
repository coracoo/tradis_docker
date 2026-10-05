package api

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"dockerpanel/backend/internal/templatecompiler"
	"dockerpanel/backend/pkg/database"
	"dockerpanel/backend/pkg/deployment"
)

const appStoreRequestUpgradeRequired = "APPSTORE_REQUEST_UPGRADE_REQUIRED"

func renderAppStoreDeployment(app *App, request DeployRequest) (templatecompiler.RenderedBundle, templatecompiler.Manifest, error) {
	bundle, manifest, _, err := resolveAppStoreManifest(app)
	if err != nil {
		return templatecompiler.RenderedBundle{}, templatecompiler.Manifest{}, err
	}
	baseManifest := manifest
	if request.SourceOverride == nil && strings.TrimSpace(request.BaseManifestDigest) != "" && legacySourceChanged(app, request) {
		return templatecompiler.RenderedBundle{}, templatecompiler.Manifest{}, fmt.Errorf("%s: scoped requests must use sourceOverride for edited source", appStoreRequestUpgradeRequired)
	}
	if request.SourceOverride != nil {
		if strings.TrimSpace(request.ManifestDigest) != "" {
			return templatecompiler.RenderedBundle{}, templatecompiler.Manifest{}, fmt.Errorf("%s: manifestDigest cannot be combined with sourceOverride", appStoreRequestUpgradeRequired)
		}
		baseDigest := strings.TrimSpace(request.BaseManifestDigest)
		overrideDigest := strings.TrimSpace(request.OverrideManifestDigest)
		if baseDigest == "" || overrideDigest == "" {
			return templatecompiler.RenderedBundle{}, templatecompiler.Manifest{}, fmt.Errorf("%s: source override requires base and override manifest digests", appStoreRequestUpgradeRequired)
		}
		if baseDigest != baseManifest.ManifestDigest {
			return templatecompiler.RenderedBundle{}, templatecompiler.Manifest{}, fmt.Errorf("AppStore base manifest digest is stale: request=%s current=%s", baseDigest, baseManifest.ManifestDigest)
		}
		compiled, err := templatecompiler.Compile(*request.SourceOverride, manifestPresentationMetadata(baseManifest))
		if err != nil {
			return templatecompiler.RenderedBundle{}, templatecompiler.Manifest{}, err
		}
		if err := incompleteManifestError(compiled.Manifest); err != nil {
			return templatecompiler.RenderedBundle{}, templatecompiler.Manifest{}, err
		}
		bundle = *request.SourceOverride
		manifest = compiled.Manifest
		if overrideDigest != manifest.ManifestDigest {
			return templatecompiler.RenderedBundle{}, templatecompiler.Manifest{}, fmt.Errorf("AppStore override manifest digest is stale: request=%s current=%s", overrideDigest, manifest.ManifestDigest)
		}
	} else if strings.TrimSpace(request.BaseManifestDigest) == "" && strings.TrimSpace(request.ManifestDigest) == "" && legacySourceChanged(app, request) {
		bundle.Compose = firstNonEmpty(request.Compose, bundle.Compose)
		if request.Dotenv != "" {
			bundle.Dotenv = request.Dotenv
		}
		compiled, err := templatecompiler.Compile(bundle, manifestPresentationMetadata(baseManifest))
		if err != nil {
			return templatecompiler.RenderedBundle{}, templatecompiler.Manifest{}, err
		}
		if err := incompleteManifestError(compiled.Manifest); err != nil {
			return templatecompiler.RenderedBundle{}, templatecompiler.Manifest{}, err
		}
		manifest = compiled.Manifest
	}

	values := make(map[string]string)
	overlays := append([]templatecompiler.MappingOverlay(nil), request.MappingOverlays...)
	digest := strings.TrimSpace(request.BaseManifestDigest)
	if digest == "" && request.SourceOverride == nil {
		digest = strings.TrimSpace(request.ManifestDigest)
	}
	if request.SourceOverride != nil || digest != "" {
		if request.SourceOverride == nil && digest != baseManifest.ManifestDigest {
			return templatecompiler.RenderedBundle{}, templatecompiler.Manifest{}, fmt.Errorf("AppStore manifest digest is stale: request=%s current=%s", digest, baseManifest.ManifestDigest)
		}
		for id, value := range request.ValuesByInputID {
			values[id] = value
		}
	} else {
		legacyValues, legacyOverlays, err := adaptLegacyAppStoreRequest(manifest, request)
		if err != nil {
			return templatecompiler.RenderedBundle{}, templatecompiler.Manifest{}, err
		}
		values = legacyValues
		overlays = append(overlays, legacyOverlays...)
	}
	rendered, err := templatecompiler.Render(bundle, manifest, values, overlays)
	if err != nil {
		return templatecompiler.RenderedBundle{}, templatecompiler.Manifest{}, err
	}
	return rendered, manifest, nil
}

// buildRemoteAppStoreCandidate keeps rendering on the Controller, while the
// target Agent remains the only side that materializes project files. The
// rendered .env is carried through the dedicated candidate field; every other
// declared file retains its original relative path and mode.
func buildRemoteAppStoreCandidate(app *App, environmentID string, request DeployRequest) (deployment.DeploymentCandidate, error) {
	if app == nil || strings.TrimSpace(environmentID) == "" || environmentID == database.LocalEnvironmentID {
		return deployment.DeploymentCandidate{}, fmt.Errorf("remote AppStore deployment target is invalid")
	}
	rendered, _, err := renderAppStoreDeployment(app, request)
	if err != nil {
		return deployment.DeploymentCandidate{}, err
	}
	baseProjectName := normalizeProjectName(app.Name)
	projectName := normalizeProjectName(firstNonEmpty(request.ProjectName, baseProjectName))
	if !isAppStoreProjectName(baseProjectName, projectName) {
		return deployment.DeploymentCandidate{}, fmt.Errorf("应用默认项目名无效")
	}
	compose := rendered.Compose
	if projectName != baseProjectName {
		compose, err = removeExplicitContainerNames(compose)
		if err != nil {
			return deployment.DeploymentCandidate{}, err
		}
	}
	dotenv := ""
	files := make([]deployment.DeploymentFile, 0, len(rendered.Files))
	composePath := filepath.Clean(strings.TrimSpace(rendered.ComposePath))
	for _, file := range rendered.Files {
		path := filepath.Clean(strings.TrimSpace(file.Path))
		if path == "." || path == composePath {
			continue
		}
		if path == ".env" {
			dotenv = file.Content
			continue
		}
		files = append(files, deployment.DeploymentFile{Path: file.Path, Content: file.Content, Mode: file.Mode})
	}
	return deployment.NormalizeDeploymentCandidate(deployment.DeploymentCandidate{
		Version:       deployment.DeploymentCandidateVersion,
		EnvironmentID: environmentID,
		ProjectName:   projectName,
		SourceType:    deployment.DeploymentSourceAppStore,
		ComposeYAML:   compose,
		Dotenv:        dotenv,
		Files:         files,
		Options:       deployment.DeploymentOptions{AutoStart: true},
	})
}

func isAppStoreProjectName(baseName, candidate string) bool {
	if candidate == baseName {
		return true
	}
	prefix := baseName + "_"
	if !strings.HasPrefix(candidate, prefix) {
		return false
	}
	suffix, err := strconv.Atoi(strings.TrimPrefix(candidate, prefix))
	return err == nil && suffix > 0
}

func legacySourceChanged(app *App, request DeployRequest) bool {
	if app == nil {
		return false
	}
	return request.Compose != "" && request.Compose != app.Compose || request.Dotenv != "" && request.Dotenv != app.Dotenv
}

func adaptLegacyAppStoreRequest(manifest templatecompiler.Manifest, request DeployRequest) (map[string]string, []templatecompiler.MappingOverlay, error) {
	values := make(map[string]string)
	for key, value := range parseDotenvToMap(request.Dotenv) {
		if err := assignLegacyInputValue(manifest, values, key, value, "Global", "env", ""); err != nil {
			return nil, nil, err
		}
	}
	for key, value := range request.Env {
		if err := assignLegacyInputValue(manifest, values, key, value, "", "env", ""); err != nil {
			return nil, nil, err
		}
	}
	for _, variable := range request.Config {
		if variable.InputID != "" {
			if !manifestHasInput(manifest, variable.InputID) {
				return nil, nil, fmt.Errorf("%s: unknown input id %s", appStoreRequestUpgradeRequired, variable.InputID)
			}
			values[variable.InputID] = variable.Default
			continue
		}
		if err := assignLegacyInputValue(manifest, values, variable.Name, variable.Default, variable.ServiceName, variable.ParamType, variable.EnvFile); err != nil {
			return nil, nil, err
		}
	}
	for name, value := range request.Secrets {
		id := "secret:" + strings.TrimSpace(name)
		if !manifestHasInput(manifest, id) {
			return nil, nil, fmt.Errorf("%s: unknown secret %s", appStoreRequestUpgradeRequired, name)
		}
		values[id] = value
	}
	return values, nil, nil
}

func assignLegacyInputValue(manifest templatecompiler.Manifest, values map[string]string, key, value, service, kind, envFile string) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return nil
	}
	normalizedKind := normalizeLegacyInputKind(kind)
	matches := make([]templatecompiler.Input, 0, 1)
	for _, input := range manifest.Inputs {
		if input.Key != key {
			continue
		}
		if normalizedKind != "" && input.Kind != normalizedKind {
			continue
		}
		if envFile != "" && input.File != strings.TrimSpace(envFile) {
			continue
		}
		if service != "" && !strings.EqualFold(strings.TrimSpace(service), "Global") && !manifestInputUsesService(manifest, input.ID, strings.TrimSpace(service)) {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(service), "Global") && input.Scope != "project" {
			continue
		}
		matches = append(matches, input)
	}
	if len(matches) != 1 {
		return fmt.Errorf("%s: input %q resolves to %d manifest fields", appStoreRequestUpgradeRequired, key, len(matches))
	}
	values[matches[0].ID] = value
	return nil
}

func normalizeLegacyInputKind(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "", "other":
		return ""
	case "environment":
		return "env"
	case "path", "volume":
		return "bind"
	case "hardware":
		return "device"
	default:
		return strings.ToLower(strings.TrimSpace(kind))
	}
}

func manifestInputUsesService(manifest templatecompiler.Manifest, inputID, service string) bool {
	for _, binding := range manifest.Bindings {
		if binding.InputID == inputID && binding.Service == service {
			return true
		}
	}
	for _, mapping := range manifest.Mappings {
		if mapping.ID == inputID && mapping.Service == service {
			return true
		}
	}
	return false
}

func manifestHasInput(manifest templatecompiler.Manifest, id string) bool {
	for _, input := range manifest.Inputs {
		if input.ID == id {
			return true
		}
	}
	return false
}

func writeRenderedAppStoreBundle(composeDir, composeFile string, bundle templatecompiler.RenderedBundle) (string, error) {
	if err := os.WriteFile(composeFile, []byte(bundle.Compose), 0644); err != nil {
		return "", err
	}
	dotenv := ""
	for _, file := range bundle.Files {
		path := strings.TrimSpace(file.Path)
		if path == "" || filepath.Clean(path) == filepath.Clean(bundle.ComposePath) {
			continue
		}
		mode := os.FileMode(file.Mode)
		if mode == 0 {
			mode = 0644
		}
		if mode != 0600 && mode != 0644 {
			return "", fmt.Errorf("unsupported rendered file mode %o for %s", mode.Perm(), path)
		}
		if err := safeWriteFileRelative(composeDir, path, []byte(file.Content), mode); err != nil {
			return "", err
		}
		if filepath.Clean(path) == ".env" {
			dotenv = file.Content
		}
	}
	return dotenv, nil
}
