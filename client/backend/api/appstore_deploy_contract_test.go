package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"dockerpanel/backend/internal/templatecompiler"
	"dockerpanel/backend/pkg/database"
	"dockerpanel/backend/pkg/deployment"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestReserveAppStoreProjectDirIsAtomicAcrossConcurrentDeployments(t *testing.T) {
	root := t.TempDir()
	const workers = 12
	results := make(chan string, workers)
	errors := make(chan error, workers)
	var wait sync.WaitGroup
	for range workers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			name, _, err := reserveAppStoreProjectDir(root, "demo")
			if err != nil {
				errors <- err
				return
			}
			results <- name
		}()
	}
	wait.Wait()
	close(results)
	close(errors)
	for err := range errors {
		require.NoError(t, err)
	}
	seen := map[string]struct{}{}
	for name := range results {
		if _, exists := seen[name]; exists {
			t.Fatalf("duplicate project reservation: %s", name)
		}
		seen[name] = struct{}{}
	}
	require.Len(t, seen, workers)
}

func TestAppStoreDeployReusesTaskForSameIdempotencyKey(t *testing.T) {
	setupAppStoreDeployContractTest(t, map[string]App{
		"idem-app": {
			ID:      77,
			Name:    "Idempotent App",
			Compose: "services:\n  web:\n    image: nginx:alpine\n",
		},
	})

	request := DeployRequest{IdempotencyKey: "same-deployment-attempt"}
	firstTaskID := startAppStoreDeployContractTask(t, "idem-app", request)
	secondTaskID := startAppStoreDeployContractTask(t, "idem-app", request)

	require.Equal(t, firstTaskID, secondTaskID)
	_ = waitForContractTask(t, firstTaskID)
	entries, err := os.ReadDir(getProjectsBaseDir())
	require.NoError(t, err)
	require.Len(t, entries, 1, "the repeated request must not allocate another project directory")
}

func TestLocalAppStoreIdempotencyRejectsChangedRequest(t *testing.T) {
	setupAppStoreDeployContractTest(t, nil)
	request := DeployRequest{IdempotencyKey: "attempt", Dotenv: "VALUE=one"}
	_, created, err := claimLocalAppStoreTask("42", request)
	require.NoError(t, err)
	require.True(t, created)
	request.Dotenv = "VALUE=two"
	_, _, err = claimLocalAppStoreTask("42", request)
	require.ErrorIs(t, err, database.ErrTaskRequestConflict)
}

func setupAppStoreDeployContractTest(t *testing.T, apps map[string]App) (string, string) {
	t.Helper()
	originalVerifier := composeDeploymentVerifier
	composeDeploymentVerifier = func(context.Context, string) (deployment.VerificationResult, error) {
		return deployment.VerificationResult{Verified: true}, nil
	}
	t.Cleanup(func() { composeDeploymentVerifier = originalVerifier })

	_ = database.Close()
	root := t.TempDir()
	workDir := filepath.Join(root, "backend")
	require.NoError(t, os.MkdirAll(workDir, 0755))
	originalWorkingDir, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(workDir))
	t.Cleanup(func() {
		_ = os.Chdir(originalWorkingDir)
	})

	require.NoError(t, database.InitDB(filepath.Join(root, "data", "data.db")))
	t.Cleanup(func() {
		_ = database.Close()
	})

	projectRoot := filepath.Join(workDir, "project")
	t.Setenv("PROJECT_ROOT", projectRoot)

	cacheDir := filepath.Join(root, "app-cache")
	require.NoError(t, os.MkdirAll(cacheDir, 0755))
	originalGetAppCacheDir := getAppCacheDirFunc
	getAppCacheDirFunc = func() string { return cacheDir }
	t.Cleanup(func() {
		getAppCacheDirFunc = originalGetAppCacheDir
	})

	appDetailCacheMu.Lock()
	originalAppDetailCache := appDetailCache
	appDetailCache = make(map[string]appDetailCacheEntry, len(apps))
	for key, app := range apps {
		appDetailCache[key] = appDetailCacheEntry{app: app, fetchedAt: time.Now()}
	}
	appDetailCacheMu.Unlock()
	t.Cleanup(func() {
		appDetailCacheMu.Lock()
		appDetailCache = originalAppDetailCache
		appDetailCacheMu.Unlock()
	})

	setupAppStoreDeployCountTestCache(t)

	dockerLog := filepath.Join(root, "docker-args.log")
	binDir := filepath.Join(root, "bin")
	require.NoError(t, os.MkdirAll(binDir, 0755))
	fakeDocker := `#!/bin/sh
printf '%s\n' '--- invocation ---' >> "$DOCKER_ARGS_LOG"
printf '%s\n' "$@" >> "$DOCKER_ARGS_LOG"
case " $* " in
  *" config --quiet "*)
    if [ "$DOCKER_CONFIG_FAIL" = "1" ]; then
      printf '%s\n' 'invalid compose configuration' >&2
      exit 1
    fi
    exit 0
    ;;
esac
printf '%s\n' 'Container test Created'
printf '%s\n' 'Container test warning' >&2
`
	require.NoError(t, os.WriteFile(filepath.Join(binDir, "docker"), []byte(fakeDocker), 0755))
	t.Setenv("DOCKER_ARGS_LOG", dockerLog)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	return projectRoot, dockerLog
}

func startAppStoreDeployContractTask(t *testing.T, appID string, request DeployRequest) string {
	t.Helper()
	body, err := json.Marshal(request)
	require.NoError(t, err)

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: appID}}
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/appstore/deploy/"+appID, bytes.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")

	deployApp(ctx)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())

	var response struct {
		TaskID string `json:"taskId"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.NotEmpty(t, response.TaskID)
	return response.TaskID
}

func waitForContractTask(t *testing.T, taskID string) database.TaskRecord {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		task, err := database.GetTask(taskID)
		if err == nil {
			switch strings.ToLower(strings.TrimSpace(task.Status)) {
			case "success", "completed", "error", "failed":
				// The terminal row is written before deferred notification and
				// cancellation cleanup complete. Wait for unregister so the next
				// test cannot close the shared database under the goroutine.
				if _, running := composeTaskCancels.Load(taskID); !running {
					return task
				}
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("task %s did not finish before timeout", taskID)
	return database.TaskRecord{}
}

func waitForContractNotification(t *testing.T, messagePart string) database.Notification {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		notifications, err := database.GetNotifications(20)
		if err == nil {
			for _, notification := range notifications {
				if strings.Contains(notification.Message, messagePart) {
					return notification
				}
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("notification containing %q was not saved", messagePart)
	return database.Notification{}
}

func TestAppStoreDeployContractWritesFilesRunsComposeAndNotifies(t *testing.T) {
	template := App{
		ID:     42,
		Name:   "official-demo",
		Source: appStoreSourceOfficial,
		Compose: `services:
  web:
    image: nginx:${TAG:-latest}
    container_name: official-demo
    environment:
      APP_MODE: ${APP_MODE:-prod}
    ports:
      - "8080:80"
    secrets:
      - api_token
    volumes:
      - app-data:/data
      - ./config:/etc/app:ro
volumes:
  app-data:
secrets:
  api_token:
    file: ./api-token.txt
`,
	}
	projectRoot, dockerLog := setupAppStoreDeployContractTest(t, map[string]App{
		"42": template,
	})

	taskID := startAppStoreDeployContractTask(t, "42", DeployRequest{
		ProjectName: "custom-project",
		Compose:     template.Compose,
		Dotenv:      "TAG=old\n",
		Env: map[string]string{
			"TAG":      "stable",
			"APP_MODE": "debug",
		},
		Config: []Variable{
			{Name: "TAG", Default: "stable", ServiceName: "Global", ParamType: "env"},
			{Name: "APP_MODE", Default: "debug", ServiceName: "web", ParamType: "env"},
		},
		Secrets: map[string]string{"api_token": "contract-secret"},
	})

	task := waitForContractTask(t, taskID)
	require.Equal(t, "appstore_deploy", task.Type)
	require.Equal(t, "success", task.Status, task.Error)
	require.JSONEq(t, `{"app_id":42,"project":"official-demo","verification":{"verified":true,"attempts":0}}`, task.ResultJSON)
	logs, err := database.GetTaskLogsAfter(taskID, 0, 100)
	require.NoError(t, err)
	var messages []string
	for _, log := range logs {
		messages = append(messages, log.Message)
	}
	require.Contains(t, strings.Join(messages, "\n"), "Container test Created")
	require.Contains(t, strings.Join(messages, "\n"), "Container test warning")

	projectDir := filepath.Join(projectRoot, "official-demo")
	composeBytes, err := os.ReadFile(filepath.Join(projectDir, "docker-compose.yml"))
	require.NoError(t, err)
	composeText := string(composeBytes)
	require.Contains(t, composeText, "nginx:${TAG:-latest}")
	require.Contains(t, composeText, "app-data:/data")
	require.Contains(t, composeText, "./config:/etc/app:ro")
	require.Contains(t, composeText, `8080:80`)
	require.Contains(t, composeText, "container_name: official-demo")
	require.NotContains(t, composeText, "contract-secret")

	dotenvBytes, err := os.ReadFile(filepath.Join(projectDir, ".env"))
	require.NoError(t, err)
	require.Contains(t, string(dotenvBytes), "TAG=stable")
	require.Contains(t, string(dotenvBytes), "APP_MODE=debug")

	secretPath := filepath.Join(projectDir, "api-token.txt")
	secretBytes, err := os.ReadFile(secretPath)
	require.NoError(t, err)
	require.Equal(t, "contract-secret", string(secretBytes))
	secretInfo, err := os.Stat(secretPath)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0600), secretInfo.Mode().Perm())

	dockerArgs, err := os.ReadFile(dockerLog)
	require.NoError(t, err)
	argsText := string(dockerArgs)
	require.Equal(t, 2, strings.Count(argsText, "--- invocation ---"))
	for _, expected := range []string{
		"compose",
		"--project-directory\n" + projectDir,
		"--file\n" + filepath.Join(projectRoot, ".tradis-runtime", "official-demo", "docker-compose.yml"),
		"--env-file\n" + filepath.Join(projectDir, ".env"),
		"config\n--quiet",
		"up\n-d",
	} {
		require.Contains(t, argsText, expected)
	}

	notification := waitForContractNotification(t, "应用 official-demo 部署成功")
	require.Equal(t, "success", notification.Type)
	require.Equal(t, "deploy_task", notification.Category)
	requireNoCommercialAgentRunSideEffects(t)
}

func TestAppStoreDeployContractValidatesComposeBeforeStartingContainers(t *testing.T) {
	template := App{
		ID:      43,
		Name:    "invalid-demo",
		Source:  appStoreSourceOfficial,
		Compose: "services:\n  web:\n    image: nginx\n",
	}
	projectRoot, dockerLog := setupAppStoreDeployContractTest(t, map[string]App{"43": template})
	t.Setenv("DOCKER_CONFIG_FAIL", "1")

	taskID := startAppStoreDeployContractTask(t, "43", DeployRequest{ProjectName: "invalid-project"})
	task := waitForContractTask(t, taskID)
	require.Equal(t, "error", task.Status)
	require.Contains(t, task.Error, "invalid compose configuration")
	_, err := os.Stat(filepath.Join(projectRoot, "invalid-demo"))
	require.ErrorIs(t, err, os.ErrNotExist)

	dockerArgs, err := os.ReadFile(dockerLog)
	require.NoError(t, err)
	argsText := string(dockerArgs)
	require.Contains(t, argsText, "config\n--quiet")
	require.NotContains(t, argsText, "up\n-d")
}

func TestAppStoreDeployContractAllocatesSuffixWithoutOverwritingExistingProject(t *testing.T) {
	template := App{
		ID:      42,
		Name:    "official-demo",
		Source:  appStoreSourceOfficial,
		Compose: "services:\n  app:\n    image: nginx:latest\n    container_name: official-demo\n",
	}
	projectRoot, dockerLog := setupAppStoreDeployContractTest(t, map[string]App{"42": template})
	projectDir := filepath.Join(projectRoot, "official-demo")
	require.NoError(t, os.MkdirAll(projectDir, 0755))
	original := []byte("user-owned compose")
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "docker-compose.yml"), original, 0644))

	taskID := startAppStoreDeployContractTask(t, "42", DeployRequest{ProjectName: "user-supplied-name"})
	task := waitForContractTask(t, taskID)
	require.Equal(t, "success", task.Status, task.Error)
	require.JSONEq(t, `{"app_id":42,"project":"official-demo_1","verification":{"verified":true,"attempts":0}}`, task.ResultJSON)

	actual, err := os.ReadFile(filepath.Join(projectDir, "docker-compose.yml"))
	require.NoError(t, err)
	require.Equal(t, original, actual)
	deployed, err := os.ReadFile(filepath.Join(projectRoot, "official-demo_1", "docker-compose.yml"))
	require.NoError(t, err)
	require.Contains(t, string(deployed), "nginx:latest")
	require.NotContains(t, string(deployed), "container_name")
	_, err = os.Stat(dockerLog)
	require.NoError(t, err)

	notification := waitForContractNotification(t, "应用 official-demo 部署成功")
	require.Equal(t, "success", notification.Type)
	require.Equal(t, "deploy_task", notification.Category)
}

func TestAppStoreDeployV2KeepsOfficialCacheAndWritesCompleteBundle(t *testing.T) {
	template := App{
		ID:     51,
		Name:   "manifest-demo",
		Source: appStoreSourceOfficial,
		Compose: `services:
  web:
    image: nginx:${TAG:-latest}
    env_file: config/web.env
    ports:
      - "8080:80"
    volumes:
      - ./data:/data
      - shared-data:/cache
    x-tradis-test:
      keep: true
volumes:
  shared-data: {}
`,
		Dotenv: "TAG=latest\n",
		SourceFiles: []templatecompiler.SourceFile{{
			Path: "config/web.env", Content: "APP_MODE=prod\n", Mode: 0644,
		}},
	}
	compiled, err := templatecompiler.Compile(appSourceBundle(&template), nil)
	require.NoError(t, err)
	template.Manifest = &compiled.Manifest
	template.SourceDigest = compiled.Manifest.SourceDigest
	template.ManifestDigest = compiled.Manifest.ManifestDigest
	template.CompilerVersion = compiled.Manifest.CompilerVersion

	projectRoot, _ := setupAppStoreDeployContractTest(t, map[string]App{"51": template})
	require.NoError(t, writeAppDetailCache(&template))
	cachePath := filepath.Join(getAppCacheDir(), template.Name+".json")
	cacheBefore, err := os.ReadFile(cachePath)
	require.NoError(t, err)

	taskID := startAppStoreDeployContractTask(t, "51", DeployRequest{
		ProjectName:    "manifest-project",
		ManifestDigest: compiled.Manifest.ManifestDigest,
		ValuesByInputID: map[string]string{
			"project:TAG":                      "stable",
			"port:web:80/tcp":                  "50080",
			"bind:web:/data":                   "/mnt/manifest-data",
			"env_file:config/web.env:APP_MODE": "debug",
		},
	})
	task := waitForContractTask(t, taskID)
	require.Equal(t, "success", task.Status, task.Error)

	projectDir := filepath.Join(projectRoot, "manifest-demo")
	composeBytes, err := os.ReadFile(filepath.Join(projectDir, "docker-compose.yml"))
	require.NoError(t, err)
	composeText := string(composeBytes)
	require.Contains(t, composeText, "50080:80")
	require.Contains(t, composeText, "/mnt/manifest-data:/data")
	require.Contains(t, composeText, "shared-data:/cache")
	require.Contains(t, composeText, "x-tradis-test")
	envFile, err := os.ReadFile(filepath.Join(projectDir, "config", "web.env"))
	require.NoError(t, err)
	require.Contains(t, string(envFile), "APP_MODE=debug")

	cacheAfter, err := os.ReadFile(cachePath)
	require.NoError(t, err)
	require.Equal(t, cacheBefore, cacheAfter)
}
