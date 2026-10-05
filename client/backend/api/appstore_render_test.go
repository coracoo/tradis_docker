package api

import (
	"strings"
	"testing"

	"dockerpanel/backend/internal/templatecompiler"
	"dockerpanel/backend/pkg/deployment"
	"github.com/stretchr/testify/require"
)

func TestRenderAppStoreDeploymentV2PreservesOpaqueAndNamedVolume(t *testing.T) {
	app := App{
		Name: "render-demo",
		Compose: `services:
  app:
    image: example/app:latest
    x-extra:
      keep: true
    ports:
      - 8080:80
    volumes:
      - ./data:/data
      - app-data:/cache
volumes:
  app-data:
`,
	}
	result, err := templatecompiler.Compile(appSourceBundle(&app), nil)
	require.NoError(t, err)
	app.Manifest = &result.Manifest
	app.SourceDigest = result.Manifest.SourceDigest
	app.ManifestDigest = result.Manifest.ManifestDigest
	app.CompilerVersion = result.Manifest.CompilerVersion

	rendered, manifest, err := renderAppStoreDeployment(&app, DeployRequest{
		ProjectName:     "render-demo",
		ManifestDigest:  result.Manifest.ManifestDigest,
		ValuesByInputID: map[string]string{"port:app:80/tcp": "50100", "bind:app:/data": "/mnt/apps/render-demo"},
	})
	require.NoError(t, err)
	require.Equal(t, result.Manifest.ManifestDigest, manifest.ManifestDigest)
	require.Contains(t, rendered.Compose, "50100:80")
	require.Contains(t, rendered.Compose, "/mnt/apps/render-demo:/data")
	require.Contains(t, rendered.Compose, "app-data:/cache")
	require.Contains(t, rendered.Compose, "x-extra")
}

func TestRenderAppStoreDeploymentLegacyRejectsAmbiguousPlainName(t *testing.T) {
	app := App{
		Name: "ambiguous",
		Compose: `services:
  api:
    image: example/api:latest
    env_file: [api.env]
  worker:
    image: example/worker:latest
    env_file: [worker.env]
`,
		SourceFiles: []templatecompiler.SourceFile{
			{Path: "api.env", Content: "TOKEN=api\n"},
			{Path: "worker.env", Content: "TOKEN=worker\n"},
		},
	}
	_, _, err := renderAppStoreDeployment(&app, DeployRequest{
		Config: []Variable{{Name: "TOKEN", Default: "changed", ParamType: "env"}},
	})
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "APPSTORE_REQUEST_UPGRADE_REQUIRED"), err.Error())
}

func TestRenderAppStoreDeploymentRejectsStaleV2Digest(t *testing.T) {
	app := App{Name: "stale", Compose: "services:\n  app:\n    image: nginx:latest\n"}
	_, _, err := renderAppStoreDeployment(&app, DeployRequest{ManifestDigest: "sha256:stale"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "manifest digest")
}

func TestRenderAppStoreDeploymentValidatesBaseAndOverrideDigestsSeparately(t *testing.T) {
	app := App{
		Name: "advanced-source",
		Compose: `services:
  app:
    image: nginx:${TAG:-latest}
`,
		Dotenv: "TAG=latest\n",
	}
	base, err := templatecompiler.Compile(appSourceBundle(&app), nil)
	require.NoError(t, err)
	app.Manifest = &base.Manifest
	app.SourceDigest = base.Manifest.SourceDigest
	app.ManifestDigest = base.Manifest.ManifestDigest
	app.CompilerVersion = base.Manifest.CompilerVersion

	override := templatecompiler.SourceBundle{
		ComposePath: "compose.yaml",
		Compose: `services:
  app:
    image: nginx:${TAG:-latest}
    ports:
      - 50000:80
`,
		Dotenv: "TAG=latest\n",
	}
	overrideCompiled, err := templatecompiler.Compile(override, manifestPresentationMetadata(base.Manifest))
	require.NoError(t, err)

	rendered, manifest, err := renderAppStoreDeployment(&app, DeployRequest{
		ProjectName:            "advanced-source",
		BaseManifestDigest:     base.Manifest.ManifestDigest,
		OverrideManifestDigest: overrideCompiled.Manifest.ManifestDigest,
		SourceOverride:         &override,
		ValuesByInputID:        map[string]string{"project:TAG": "stable"},
	})
	require.NoError(t, err)
	require.Equal(t, overrideCompiled.Manifest.ManifestDigest, manifest.ManifestDigest)
	require.Contains(t, rendered.Compose, "50000:80")
	require.Contains(t, rendered.Compose, "nginx:${TAG:-latest}")

	_, _, err = renderAppStoreDeployment(&app, DeployRequest{
		BaseManifestDigest:     "sha256:stale-base",
		OverrideManifestDigest: overrideCompiled.Manifest.ManifestDigest,
		SourceOverride:         &override,
	})
	require.ErrorContains(t, err, "base manifest digest")

	_, _, err = renderAppStoreDeployment(&app, DeployRequest{
		BaseManifestDigest:     base.Manifest.ManifestDigest,
		OverrideManifestDigest: "sha256:stale-override",
		SourceOverride:         &override,
	})
	require.ErrorContains(t, err, "override manifest digest")
}

func TestRenderAppStoreDeploymentRejectsLegacyDigestWithSourceOverride(t *testing.T) {
	app := App{Name: "legacy-override", Compose: "services:\n  app:\n    image: nginx:latest\n"}
	base, err := templatecompiler.Compile(appSourceBundle(&app), nil)
	require.NoError(t, err)
	override := appSourceBundle(&app)

	_, _, err = renderAppStoreDeployment(&app, DeployRequest{
		ManifestDigest:         base.Manifest.ManifestDigest,
		BaseManifestDigest:     base.Manifest.ManifestDigest,
		OverrideManifestDigest: base.Manifest.ManifestDigest,
		SourceOverride:         &override,
	})
	require.ErrorContains(t, err, appStoreRequestUpgradeRequired)
}

func TestRenderAppStoreDeploymentRejectsLegacySourceFieldsWithBaseDigest(t *testing.T) {
	app := App{Name: "mixed-contract", Compose: "services:\n  app:\n    image: nginx:latest\n"}
	base, err := templatecompiler.Compile(appSourceBundle(&app), nil)
	require.NoError(t, err)

	_, _, err = renderAppStoreDeployment(&app, DeployRequest{
		BaseManifestDigest: base.Manifest.ManifestDigest,
		Compose:            "services:\n  app:\n    image: nginx:stable\n",
	})
	require.ErrorContains(t, err, appStoreRequestUpgradeRequired)
}

func TestCollectComposePublishedTCPPortsSupportsShortAndLongSyntax(t *testing.T) {
	ports := collectComposePublishedTCPPorts(`services:
  web:
    ports:
      - "127.0.0.1:50001:80"
      - "50002:443/udp"
      - target: 8080
        published: 50003
        protocol: tcp
      - target: 8081
        published: 50004
      - "50010-50011:8100-8101"
      - target: "8200-8201"
        published: "50020-50021"
      - "9090"
`)
	require.Equal(t, []int{50001, 50003, 50004, 50010, 50011, 50020, 50021}, ports)
}

func TestAppStoreRemoteCandidateSeparatesDotenvAndDeclaredFiles(t *testing.T) {
	app := &App{
		ID: 42, Name: "remote-demo",
		Compose: "services:\n  app:\n    image: nginx:${TAG:-latest}\n",
		Dotenv:  "TAG=latest\n",
		SourceFiles: []templatecompiler.SourceFile{
			{Path: "config/app.env", Content: "APP_MODE=prod\n", Mode: 0644},
			{Path: "secret.txt", Content: "secret", Mode: 0600},
		},
	}

	candidate, err := buildRemoteAppStoreCandidate(app, "remote-c1", DeployRequest{})
	require.NoError(t, err)
	require.Equal(t, deployment.DeploymentSourceAppStore, candidate.SourceType)
	require.Equal(t, "remote-c1", candidate.EnvironmentID)
	require.Equal(t, "remote-demo", candidate.ProjectName)
	require.Equal(t, "TAG=latest\n", candidate.Dotenv)
	require.Len(t, candidate.Files, 2)
	require.Equal(t, "config/app.env", candidate.Files[0].Path)
	require.Equal(t, "secret.txt", candidate.Files[1].Path)
	require.NotContains(t, candidate.ComposeYAML, "official")
}
