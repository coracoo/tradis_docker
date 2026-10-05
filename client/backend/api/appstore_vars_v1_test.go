package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"dockerpanel/backend/internal/templatecompiler"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestBuildAppStoreManifestVarsKeepsScopedInputsAndKinds(t *testing.T) {
	app := App{
		Name: "scoped",
		Compose: `services:
  api:
    image: example/api:latest
    env_file: [api.env]
    environment:
      SHARED: ${SHARED}
    ports: [8080:80]
    volumes:
      - ./api-data:/data
    devices:
      - /dev/dri:/dev/dri
    secrets: [external_token]
  worker:
    image: example/worker:latest
    env_file: [worker.env]
secrets:
  external_token:
    external: true
`,
		Dotenv: "SHARED=project\n",
		SourceFiles: []templatecompiler.SourceFile{
			{Path: "api.env", Content: "SHARED=api\n"},
			{Path: "worker.env", Content: "SHARED=worker\n"},
		},
	}

	response, err := buildAppStoreManifestVars(&app)
	require.NoError(t, err)
	require.True(t, response.Manifest.Complete)
	require.Equal(t, "api", response.InitialValues["env_file:api.env:SHARED"])
	require.Equal(t, "worker", response.InitialValues["env_file:worker.env:SHARED"])
	require.Equal(t, "project", response.InitialValues["project:SHARED"])

	kinds := make(map[string]AppParamKind)
	for _, param := range response.Params {
		kinds[param.InputID] = param.Kind
		require.NotEmpty(t, param.InputID)
	}
	require.Equal(t, AppParamKind("port"), kinds["port:api:80/tcp"])
	require.Equal(t, AppParamKind("bind"), kinds["bind:api:/data"])
	require.Equal(t, AppParamKind("device"), kinds["device:api:/dev/dri"])
	_, externalSecretExposed := kinds["secret:external_token"]
	require.False(t, externalSecretExposed)
}

func TestParseAppStoreVarsAcceptsValidMultiFileBundleAboveTwoMiB(t *testing.T) {
	files := []templatecompiler.SourceFile{
		{Path: "assets/one.txt", Content: strings.Repeat("a", 800<<10)},
		{Path: "assets/two.txt", Content: strings.Repeat("b", 800<<10)},
		{Path: "assets/three.txt", Content: strings.Repeat("c", 800<<10)},
	}
	body, err := json.Marshal(map[string]any{
		"compose":      "services:\n  app:\n    image: nginx\n",
		"source_files": files,
	})
	require.NoError(t, err)
	require.Greater(t, len(body), 2<<20)

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/appstore/parse-vars", bytes.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	parseAppStoreVars(ctx)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
}

func TestBuildAppStoreManifestVarsProjectsOneInputWithMultipleBindingsOnce(t *testing.T) {
	app := App{
		Name:   "shared-binding",
		Dotenv: "TAG=stable\n",
		Compose: `services:
  api:
    image: example/api:${TAG}
  worker:
    image: example/worker:${TAG}
`,
	}
	response, err := buildAppStoreManifestVars(&app)
	require.NoError(t, err)
	count := 0
	bindingCount := 0
	for _, param := range response.Params {
		if param.InputID == "project:TAG" {
			count++
			bindingCount = len(param.Bindings)
		}
	}
	require.Equal(t, 1, count)
	require.Equal(t, 2, bindingCount)
}

func TestBuildAppStoreManifestVarsIgnoresStaleLegacySchemaDefault(t *testing.T) {
	app := App{
		Name:   "stale-schema",
		Dotenv: "NATS_PASSWORD=source-value\n",
		Compose: `services:
  api:
    image: example/api:latest
    environment:
      NATS_PASSWORD: ${NATS_PASSWORD}
`,
		Schema: []Variable{{
			InputID:     "project:NATS_PASSWORD",
			Name:        "NATS_PASSWORD",
			Default:     "stale-schema-value",
			ServiceName: "Global",
			ParamType:   "env",
		}},
	}

	response, err := buildAppStoreManifestVars(&app)
	require.NoError(t, err)
	require.Equal(t, "source-value", response.InitialValues["project:NATS_PASSWORD"])

	var projected Variable
	require.Len(t, response.Schema, 1)
	projected = response.Schema[0]
	require.Equal(t, "project:NATS_PASSWORD", projected.InputID)
	require.Equal(t, "source-value", projected.Default)
}
