package api

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"dockerpanel/backend/internal/templatecompiler"
	"github.com/stretchr/testify/require"
)

func manifestTestApp(t *testing.T) App {
	t.Helper()
	app := App{
		ID:   1,
		Name: "manifest-demo",
		Compose: `services:
  app:
    image: example/app:${TAG:-latest}
    env_file:
      - config/app.env
    ports:
      - 8080:80
`,
		Dotenv:      "TAG=stable\n",
		SourceFiles: []templatecompiler.SourceFile{{Path: "config/app.env", Content: "MODE=prod\n"}},
	}
	result, err := templatecompiler.Compile(appSourceBundle(&app), nil)
	require.NoError(t, err)
	app.Manifest = &result.Manifest
	app.SourceDigest = result.Manifest.SourceDigest
	app.ManifestDigest = result.Manifest.ManifestDigest
	app.CompilerVersion = result.Manifest.CompilerVersion
	return app
}

func TestResolveAppStoreManifestUsesVerifiedPublishedManifest(t *testing.T) {
	app := manifestTestApp(t)
	bundle, manifest, diagnostics, err := resolveAppStoreManifest(&app)
	require.NoError(t, err)
	require.Equal(t, app.ManifestDigest, manifest.ManifestDigest)
	require.Equal(t, "config/app.env", bundle.Files[0].Path)
	require.Empty(t, diagnostics)
}

func TestResolveAppStoreManifestRecompilesMismatchedPublishedManifest(t *testing.T) {
	app := manifestTestApp(t)
	app.Manifest.ManifestDigest = "sha256:corrupt"
	bundle, manifest, _, err := resolveAppStoreManifest(&app)
	require.NoError(t, err)
	require.NoError(t, templatecompiler.Verify(bundle, manifest))
	require.NotEqual(t, "sha256:corrupt", manifest.ManifestDigest)
}

func TestResolveAppStoreManifestAppliesLegacyPresentationToVerifiedManifest(t *testing.T) {
	app := manifestTestApp(t)
	app.Schema = []Variable{{
		InputID:     "project:TAG",
		Name:        "TAG",
		Label:       "镜像版本",
		Category:    "advanced",
		ServiceName: "Global",
		ParamType:   "env",
	}}

	bundle, manifest, _, err := resolveAppStoreManifest(&app)
	require.NoError(t, err)
	require.NoError(t, templatecompiler.Verify(bundle, manifest))

	var tagInput *templatecompiler.Input
	for index := range manifest.Inputs {
		if manifest.Inputs[index].ID == "project:TAG" {
			tagInput = &manifest.Inputs[index]
			break
		}
	}
	require.NotNil(t, tagInput)
	require.Equal(t, "镜像版本", tagInput.Presentation.Label)
	require.Equal(t, "advanced", tagInput.Presentation.Group)
	require.NotEqual(t, app.ManifestDigest, manifest.ManifestDigest)
}

func TestResolveAppStoreManifestMatchesLegacySchemaWithoutInputIDs(t *testing.T) {
	app := manifestTestApp(t)
	app.Schema = []Variable{
		{Name: "TAG", Label: "镜像版本", Category: "advanced", ServiceName: "Global", ParamType: "env"},
		{Name: "MODE", Label: "运行模式", Category: "advanced", ServiceName: "Global", ParamType: "env"},
		{Name: "8080", Label: "访问端口", Category: "advanced", ServiceName: "app", ParamType: "port"},
	}

	_, manifest, _, err := resolveAppStoreManifest(&app)
	require.NoError(t, err)

	byID := make(map[string]templatecompiler.Input)
	for _, input := range manifest.Inputs {
		byID[input.ID] = input
	}
	require.Equal(t, "advanced", byID["project:TAG"].Presentation.Group)
	require.Equal(t, "镜像版本", byID["project:TAG"].Presentation.Label)
	require.Equal(t, "advanced", byID["env_file:config/app.env:MODE"].Presentation.Group)
	require.Equal(t, "advanced", byID["port:app:80/tcp"].Presentation.Group)
}

func TestResolveAppStoreManifestRejectsIncompleteSource(t *testing.T) {
	app := App{Name: "broken", Compose: "services: ["}
	_, _, _, err := resolveAppStoreManifest(&app)
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid_compose_yaml")
}

func TestWriteAppDetailCacheAtomicallyReplacesCompleteObject(t *testing.T) {
	root := t.TempDir()
	original := getAppCacheDirFunc
	getAppCacheDirFunc = func() string { return root }
	t.Cleanup(func() { getAppCacheDirFunc = original })

	app := manifestTestApp(t)
	require.NoError(t, writeAppDetailCache(&app))
	path := filepath.Join(root, "manifest-demo.json")
	first, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Contains(t, string(first), `"manifest_digest"`)

	app.Description = "updated"
	require.NoError(t, writeAppDetailCache(&app))
	second, err := os.ReadFile(path)
	require.NoError(t, err)
	require.False(t, bytes.Equal(first, second))
	matches, err := filepath.Glob(filepath.Join(root, ".manifest-demo.json.*.tmp"))
	require.NoError(t, err)
	require.Empty(t, matches)
}
