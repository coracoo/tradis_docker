package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"dockerpanel/backend/pkg/database"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestDeriveGeneratedComposeProjectNameUsesStableCode(t *testing.T) {
	require.Equal(t, "compose-2cd3107f", deriveGeneratedComposeProjectName("好好的11", nil))
	require.Equal(t, "compose-2cd3107f279d", deriveGeneratedComposeProjectName("好好的11", map[string]struct{}{
		"compose-2cd3107f": {},
	}))
}

func TestReadExplicitComposeProjectNames(t *testing.T) {
	require.Equal(t, "from-env", composeProjectNameFromDotenv([]byte(`
# COMPOSE_PROJECT_NAME=ignored
export COMPOSE_PROJECT_NAME=from-env
OTHER_SECRET=do-not-return
`)))
	require.Equal(t, "from-yaml", composeProjectNameFromYAML([]byte(`
name: from-yaml
services:
  app:
    image: nginx
`)))
	require.Empty(t, composeProjectNameFromDotenv([]byte("COMPOSE_PROJECT_NAME=中文\n")))
	require.Empty(t, composeProjectNameFromYAML([]byte("name: Mixed.Case\nservices: {}\n")))
}

func TestResolveManagedComposeIdentityUsesEvidencePriorityAndPersists(t *testing.T) {
	projectRoot := setupComposeDeployContractTest(t)

	tests := []struct {
		name       string
		directory  string
		dotenv     string
		yamlName   string
		observed   []string
		wantName   string
		wantSource string
	}{
		{name: "docker label", directory: "外部项目", dotenv: "COMPOSE_PROJECT_NAME=env-name\n", yamlName: "yaml-name", observed: []string{"runtime-name"}, wantName: "runtime-name", wantSource: "docker_label"},
		{name: "dotenv", directory: "外部项目2", dotenv: "COMPOSE_PROJECT_NAME=env-name\n", yamlName: "yaml-name", wantName: "env-name", wantSource: "dotenv"},
		{name: "yaml", directory: "外部项目3", yamlName: "yaml-name", wantName: "yaml-name", wantSource: "yaml"},
		{name: "uppercase directory", directory: "MyApp", wantName: "myapp", wantSource: "directory"},
		{name: "generated", directory: "好好的11", wantName: "compose-2cd3107f", wantSource: "generated"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			projectDir := filepath.Join(projectRoot, tt.directory)
			require.NoError(t, os.MkdirAll(projectDir, 0755))
			compose := "services:\n  app:\n    image: nginx\n"
			if tt.yamlName != "" {
				compose = "name: " + tt.yamlName + "\n" + compose
			}
			require.NoError(t, os.WriteFile(filepath.Join(projectDir, "compose.yaml"), []byte(compose), 0644))
			if tt.dotenv != "" {
				require.NoError(t, os.WriteFile(filepath.Join(projectDir, ".env"), []byte(tt.dotenv), 0600))
			}

			got, err := resolveManagedComposeIdentity(projectDir, tt.observed, nil)
			require.NoError(t, err)
			require.Equal(t, tt.wantName, got.ComposeProjectName)
			require.Equal(t, tt.wantSource, got.Source)

			stored, found, err := database.GetComposeProjectIdentity(database.LocalEnvironmentID, tt.directory)
			require.NoError(t, err)
			require.True(t, found)
			require.Equal(t, tt.wantName, stored.ComposeProjectName)
		})
	}
}

func TestResolveManagedComposeIdentityRejectsConflictingRuntimeEvidence(t *testing.T) {
	projectRoot := setupComposeDeployContractTest(t)
	projectDir := filepath.Join(projectRoot, "冲突项目")
	require.NoError(t, os.MkdirAll(projectDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "compose.yaml"), []byte("services:\n  app:\n    image: nginx\n"), 0644))

	_, err := resolveManagedComposeIdentity(projectDir, []string{"first", "second"}, nil)
	require.ErrorIs(t, err, errComposeIdentityConflict)
}

func TestResolveManagedComposeIdentityDoesNotReplacePersistedIdentity(t *testing.T) {
	projectRoot := setupComposeDeployContractTest(t)
	projectDir := filepath.Join(projectRoot, "Media")
	require.NoError(t, os.MkdirAll(projectDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "compose.yaml"), []byte("services:\n  app:\n    image: nginx\n"), 0644))
	require.NoError(t, database.UpsertComposeProjectIdentity(database.ComposeProjectIdentity{
		EnvironmentID: database.LocalEnvironmentID, RelativePath: "Media", ComposeProjectName: "media", Source: "directory",
	}))

	got, err := resolveManagedComposeIdentity(projectDir, nil, nil)
	require.NoError(t, err)
	require.Equal(t, "media", got.ComposeProjectName)

	_, err = resolveManagedComposeIdentity(projectDir, []string{"other-media"}, nil)
	require.ErrorIs(t, err, errComposeIdentityConflict)
}

func TestResolveComposeOperationTargetUsesExactDirectoryAndPersistedIdentity(t *testing.T) {
	projectRoot := setupComposeDeployContractTest(t)
	projectDir := filepath.Join(projectRoot, "好好的11")
	require.NoError(t, os.MkdirAll(projectDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "compose.yaml"), []byte("services:\n  app:\n    image: nginx\n"), 0644))
	require.NoError(t, database.UpsertComposeProjectIdentity(database.ComposeProjectIdentity{
		EnvironmentID: database.LocalEnvironmentID, RelativePath: "好好的11", ComposeProjectName: "test1", Source: "docker_label",
	}))

	target, err := resolveComposeOperationTarget(context.Background(), "好好的11")
	require.NoError(t, err)
	require.Equal(t, projectDir, target.ProjectDir)
	require.Equal(t, "好好的11", target.DisplayName)
	require.Equal(t, "test1", target.ComposeProjectName)

	byComposeName, err := resolveComposeOperationTarget(context.Background(), "test1")
	require.NoError(t, err)
	require.Equal(t, projectDir, byComposeName.ProjectDir)
}

func TestResolveComposeOperationTargetRejectsPersistedRuntimeConflict(t *testing.T) {
	projectRoot := setupComposeDeployContractTest(t)
	projectDir := filepath.Join(projectRoot, "中文项目")
	require.NoError(t, os.MkdirAll(projectDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "compose.yaml"), []byte("services:\n  app:\n    image: nginx\n"), 0644))
	require.NoError(t, database.UpsertComposeProjectIdentity(database.ComposeProjectIdentity{
		EnvironmentID: database.LocalEnvironmentID, RelativePath: "中文项目", ComposeProjectName: "stored-name", Source: "docker_label",
	}))

	originalObserver := observeComposeProjectNames
	observeComposeProjectNames = func(context.Context, string) ([]string, error) {
		return []string{"runtime-name"}, nil
	}
	t.Cleanup(func() { observeComposeProjectNames = originalObserver })

	_, err := resolveComposeOperationTarget(context.Background(), "中文项目")
	require.ErrorIs(t, err, errComposeIdentityConflict)
}

func TestComposeOperationConflictReturnsStableErrorCode(t *testing.T) {
	projectRoot := setupComposeDeployContractTest(t)
	projectDir := filepath.Join(projectRoot, "中文项目")
	require.NoError(t, os.MkdirAll(projectDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "compose.yaml"), []byte("services:\n  app:\n    image: nginx\n"), 0644))
	require.NoError(t, database.UpsertComposeProjectIdentity(database.ComposeProjectIdentity{
		EnvironmentID: database.LocalEnvironmentID, RelativePath: "中文项目", ComposeProjectName: "stored-name", Source: "docker_label",
	}))

	originalObserver := observeComposeProjectNames
	observeComposeProjectNames = func(context.Context, string) ([]string, error) {
		return []string{"runtime-name"}, nil
	}
	t.Cleanup(func() { observeComposeProjectNames = originalObserver })

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/compose/:name/start/tasks", startProjectTask)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/compose/中文项目/start/tasks", nil))
	require.Equal(t, http.StatusConflict, response.Code, response.Body.String())
	var payload map[string]any
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &payload))
	require.Equal(t, "compose_identity_conflict", payload["code"])
	require.Equal(t, "Compose 项目身份冲突", payload["message"])
}

func TestComposeCommandWithProjectName(t *testing.T) {
	require.Equal(t,
		[]string{"compose", "--project-name", "test1", "up", "-d"},
		composeCommandWithProjectName("test1", []string{"compose", "up", "-d"}),
	)
	require.Equal(t,
		[]string{"version"},
		composeCommandWithProjectName("test1", []string{"version"}),
	)
}

func TestResolveComposeRestoreTargetUsesPersistedIdentityAfterDirectoryRemoval(t *testing.T) {
	projectRoot := setupComposeDeployContractTest(t)
	require.NoError(t, database.UpsertComposeProjectIdentity(database.ComposeProjectIdentity{
		EnvironmentID: database.LocalEnvironmentID, RelativePath: "中文备份", ComposeProjectName: "external-app", Source: "docker_label",
	}))

	target, err := resolveComposeRestoreTarget(context.Background(), "中文备份")
	require.NoError(t, err)
	require.Equal(t, filepath.Join(projectRoot, "中文备份"), target.ProjectDir)
	require.Equal(t, "external-app", target.ComposeProjectName)
}

func TestResolveComposeRestoreTargetUsesBackupHintAfterIdentityRemoval(t *testing.T) {
	projectRoot := setupComposeDeployContractTest(t)
	target, err := resolveComposeRestoreTargetWithName(context.Background(), "已删除项目", "external-app")
	require.NoError(t, err)
	require.Equal(t, filepath.Join(projectRoot, "已删除项目"), target.ProjectDir)
	require.Equal(t, "external-app", target.ComposeProjectName)
}

func TestPersistRestoredComposeIdentityKeepsBackupHintForStoppedProject(t *testing.T) {
	setupComposeDeployContractTest(t)
	target, err := resolveComposeRestoreTargetWithName(context.Background(), "已删除项目", "external-app")
	require.NoError(t, err)
	require.NoError(t, persistRestoredComposeIdentity(target))

	stored, found, err := database.GetComposeProjectIdentity(database.LocalEnvironmentID, "已删除项目")
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, "external-app", stored.ComposeProjectName)
	require.Equal(t, "restore_manifest", stored.Source)
}

func TestComposeConfigHandlersUseExactManagedDirectory(t *testing.T) {
	projectRoot := setupComposeDeployContractTest(t)
	projectDir := filepath.Join(projectRoot, "好好的11")
	require.NoError(t, os.MkdirAll(projectDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "compose.yaml"), []byte("services:\n  app:\n    image: nginx\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, ".env"), []byte("OLD=value\n"), 0600))
	require.NoError(t, database.UpsertComposeProjectIdentity(database.ComposeProjectIdentity{
		EnvironmentID: database.LocalEnvironmentID, RelativePath: "好好的11", ComposeProjectName: "test1", Source: "docker_label",
	}))

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/compose/:name/yaml", getProjectYaml)
	router.GET("/compose/:name/env", getProjectEnv)
	router.POST("/compose/:name/env", saveProjectEnv)

	yamlResponse := httptest.NewRecorder()
	router.ServeHTTP(yamlResponse, httptest.NewRequest(http.MethodGet, "/compose/好好的11/yaml", nil))
	require.Equal(t, http.StatusOK, yamlResponse.Code)
	require.Contains(t, yamlResponse.Body.String(), "image: nginx")

	envResponse := httptest.NewRecorder()
	router.ServeHTTP(envResponse, httptest.NewRequest(http.MethodGet, "/compose/好好的11/env", nil))
	require.Equal(t, http.StatusOK, envResponse.Code)
	require.Contains(t, envResponse.Body.String(), "OLD=value")

	body, err := json.Marshal(map[string]string{"content": "NEW=value\n"})
	require.NoError(t, err)
	saveResponse := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/compose/好好的11/env", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(saveResponse, request)
	require.Equal(t, http.StatusOK, saveResponse.Code)
	saved, err := os.ReadFile(filepath.Join(projectDir, ".env"))
	require.NoError(t, err)
	require.Equal(t, "NEW=value\n", string(saved))
}

func TestLoadComposeConfigUsesExactManagedDirectory(t *testing.T) {
	projectRoot := setupComposeDeployContractTest(t)
	projectDir := filepath.Join(projectRoot, "Media Library")
	require.NoError(t, os.MkdirAll(projectDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "compose.yaml"), []byte("services:\n  app:\n    image: nginx\n"), 0644))
	require.NoError(t, database.UpsertComposeProjectIdentity(database.ComposeProjectIdentity{
		EnvironmentID: database.LocalEnvironmentID, RelativePath: "Media Library", ComposeProjectName: "media-library", Source: "docker_label",
	}))

	config, err := loadComposeConfig("Media Library")
	require.NoError(t, err)
	require.Equal(t, projectDir, config.ProjectDir)
	require.Equal(t, "compose.yaml", config.RelativePath)
}
