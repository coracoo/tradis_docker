package deployment

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type composeServiceRuntimeFake struct {
	mu            sync.Mutex
	validatedPath string
	validatedEnv  []string
	runs          [][]string
	runEnvs       [][]string
	verification  VerificationResult
	validateErr   error
	runErr        error
	verifyErr     error
}

func (fake *composeServiceRuntimeFake) ValidateConfig(_ context.Context, _ string, composePath string, env []string) error {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.validatedPath = composePath
	fake.validatedEnv = append([]string(nil), env...)
	return fake.validateErr
}

func (fake *composeServiceRuntimeFake) Run(_ context.Context, _ string, env []string, arguments []string, onLine func(string)) error {
	fake.mu.Lock()
	fake.runs = append(fake.runs, append([]string(nil), arguments...))
	fake.runEnvs = append(fake.runEnvs, append([]string(nil), env...))
	fake.mu.Unlock()
	onLine("done")
	return fake.runErr
}

func (fake *composeServiceRuntimeFake) Verify(context.Context, string) (VerificationResult, error) {
	return fake.verification, fake.verifyErr
}

type composeServiceSinkFake struct {
	running   bool
	logs      []string
	status    string
	result    any
	errText   string
	finishErr error
}

type blockingComposeGuard struct {
	mu            sync.Mutex
	calls         int
	firstStarted  chan struct{}
	secondStarted chan struct{}
	releaseFirst  chan struct{}
}

func (guard *blockingComposeGuard) CheckProject(context.Context, string, string) error {
	guard.mu.Lock()
	guard.calls++
	call := guard.calls
	if call == 1 {
		close(guard.firstStarted)
	}
	if call == 2 {
		close(guard.secondStarted)
	}
	guard.mu.Unlock()
	if call == 1 {
		<-guard.releaseFirst
	}
	return nil
}

func (fake *composeServiceSinkFake) SetRunning() error { fake.running = true; return nil }
func (fake *composeServiceSinkFake) Append(_ string, message string) error {
	fake.logs = append(fake.logs, message)
	return nil
}
func (fake *composeServiceSinkFake) Finish(status string, result any, errText string) error {
	fake.status, fake.result, fake.errText = status, result, errText
	return fake.finishErr
}

func TestComposeServiceReturnsTerminalPersistenceFailure(t *testing.T) {
	for _, autoStart := range []bool{false, true} {
		runtime := &composeServiceRuntimeFake{verification: VerificationResult{Verified: true}}
		service, err := NewComposeService(t.TempDir(), "", runtime, nil)
		require.NoError(t, err)
		candidate := validDeploymentCandidate()
		candidate.Options.AutoStart = autoStart
		persistErr := errors.New("terminal write failed")
		sink := &composeServiceSinkFake{finishErr: persistErr}
		err = service.Execute(context.Background(), "task", candidate, sink)
		require.ErrorIs(t, err, persistErr)
		require.Equal(t, "success", sink.status)
	}
}

func TestComposeServicePreflightRejectsExistingProjectWithoutReplace(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "demo"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "demo", "docker-compose.yml"), []byte("services:\n  old:\n    image: nginx\n"), 0o644))

	service, err := NewComposeService(root, "", &composeServiceRuntimeFake{}, nil)
	require.NoError(t, err)

	_, err = service.Preflight(context.Background(), validDeploymentCandidate())

	var structured *StructuredError
	require.ErrorAs(t, err, &structured)
	require.Equal(t, ErrorCodeProjectExists, structured.Code)
}

func TestComposeServicePreflightReportsExplicitReplacement(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "demo"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "demo", "docker-compose.yml"), []byte("services:\n  old:\n    image: nginx\n"), 0o644))
	runtime := &composeServiceRuntimeFake{}
	service, err := NewComposeService(root, "/host/projects", runtime, nil)
	require.NoError(t, err)
	candidate := validDeploymentCandidate()
	candidate.Options.Replace = true

	result, err := service.Preflight(context.Background(), candidate)

	require.NoError(t, err)
	require.True(t, result.RequiresConfirmation)
	require.Equal(t, "/host/projects/demo", result.HostProjectPath)
	require.NotEmpty(t, runtime.validatedPath)
	require.Len(t, result.Changes, 1)
	require.Equal(t, "replace", result.Changes[0].Type)
}

func TestComposeServicePreflightRequiresConfirmationForAbsoluteBind(t *testing.T) {
	service, err := NewComposeService(t.TempDir(), "/host/projects", &composeServiceRuntimeFake{}, nil)
	require.NoError(t, err)
	candidate := validDeploymentCandidate()
	candidate.ComposeYAML = "services:\n  app:\n    image: nginx\n    volumes:\n      - /etc:/host-etc:ro\n"

	result, err := service.Preflight(context.Background(), candidate)

	require.NoError(t, err)
	require.True(t, result.RequiresConfirmation)
	require.Len(t, result.Issues, 1)
	require.Equal(t, "absolute_bind_requires_confirmation", result.Issues[0].Code)
	require.Equal(t, PreflightWarning, result.Issues[0].Severity)
	require.Equal(t, "/etc", result.Issues[0].Details["path"])
}

func TestComposeServiceExecuteWritesCandidateRunsPlanAndVerifies(t *testing.T) {
	root := t.TempDir()
	runtime := &composeServiceRuntimeFake{verification: VerificationResult{Verified: true}}
	service, err := NewComposeService(root, "/host/projects", runtime, nil)
	require.NoError(t, err)
	candidate := validDeploymentCandidate()
	candidate.Dotenv = "TAG=stable\n"
	candidate.Files = []DeploymentFile{{Path: "config/app.ini", Content: "enabled=true\n", Mode: 0o600}}
	candidate.Options.Pull = true
	candidate.Options.Rebuild = true
	sink := &composeServiceSinkFake{}

	err = service.Execute(context.Background(), "task-1", candidate, sink)

	require.NoError(t, err)
	require.True(t, sink.running)
	require.Equal(t, "success", sink.status)
	require.FileExists(t, filepath.Join(root, "demo", "docker-compose.yml"))
	require.Equal(t, "TAG=stable\n", mustReadFile(t, filepath.Join(root, "demo", ".env")))
	require.Equal(t, "enabled=true\n", mustReadFile(t, filepath.Join(root, "demo", "config", "app.ini")))
	require.Equal(t, [][]string{{"compose", "pull"}, {"compose", "build", "--pull", "--no-cache"}, {"compose", "up", "-d"}}, runtime.runs)
}

func TestComposeServiceInjectsHostProjectRootForValidationAndDeployment(t *testing.T) {
	root := t.TempDir()
	runtime := &composeServiceRuntimeFake{verification: VerificationResult{Verified: true}}
	service, err := NewComposeService(root, "/host/docker", runtime, nil)
	require.NoError(t, err)
	candidate := validDeploymentCandidate()
	candidate.Dotenv = "PROJECT_ROOT=/untrusted/path\nAPP_MODE=production\n"
	sink := &composeServiceSinkFake{}

	require.NoError(t, service.Execute(context.Background(), "task-project-root", candidate, sink))
	require.Contains(t, runtime.validatedEnv, "PROJECT_ROOT=/host/docker")
	require.NotContains(t, runtime.validatedEnv, "PROJECT_ROOT=/untrusted/path")
	require.Len(t, runtime.runEnvs, 1)
	require.Contains(t, runtime.runEnvs[0], "PROJECT_ROOT=/host/docker")
	require.NotContains(t, runtime.runEnvs[0], "PROJECT_ROOT=/untrusted/path")
}

func TestComposeServiceLeavesDotenvInterpretationToCompose(t *testing.T) {
	runtime := &composeServiceRuntimeFake{verification: VerificationResult{Verified: true}}
	service, err := NewComposeService(t.TempDir(), "/host/projects", runtime, nil)
	require.NoError(t, err)
	candidate := validDeploymentCandidate()
	candidate.Dotenv = "TAG=\"stable\" # comment\nLITERAL='${TAG}'\nEXPANDED=${TAG}\n"
	candidate.Options.AutoStart = true
	require.NoError(t, service.Execute(context.Background(), "dotenv", candidate, &composeServiceSinkFake{}))
	require.Equal(t, []string{"PROJECT_ROOT=/host/projects"}, runtime.validatedEnv)
	for _, env := range runtime.runEnvs {
		require.Equal(t, runtime.validatedEnv, env)
	}
}

func TestComposeServiceExecuteKeepsPullOnlyUpBehavior(t *testing.T) {
	root := t.TempDir()
	runtime := &composeServiceRuntimeFake{verification: VerificationResult{Verified: true}}
	service, err := NewComposeService(root, "", runtime, nil)
	require.NoError(t, err)
	candidate := validDeploymentCandidate()
	candidate.Options.Pull = true
	sink := &composeServiceSinkFake{}

	err = service.Execute(context.Background(), "task-pull", candidate, sink)

	require.NoError(t, err)
	require.Equal(t, [][]string{{"compose", "pull"}, {"compose", "up", "-d", "--pull", "always"}}, runtime.runs)
}

func TestComposeServiceExecuteDoesNotWriteWhenValidationFails(t *testing.T) {
	root := t.TempDir()
	runtime := &composeServiceRuntimeFake{validateErr: errors.New("invalid config")}
	service, err := NewComposeService(root, "", runtime, nil)
	require.NoError(t, err)
	sink := &composeServiceSinkFake{}

	err = service.Execute(context.Background(), "task-1", validDeploymentCandidate(), sink)

	require.Error(t, err)
	require.Empty(t, sink.status)
	require.NoFileExists(t, filepath.Join(root, "demo", "docker-compose.yml"))
}

func TestComposeServiceExecuteSerializesPreflightAndMaterialize(t *testing.T) {
	root := t.TempDir()
	runtime := &composeServiceRuntimeFake{}
	guard := &blockingComposeGuard{
		firstStarted:  make(chan struct{}),
		secondStarted: make(chan struct{}),
		releaseFirst:  make(chan struct{}),
	}
	service, err := NewComposeService(root, "", runtime, guard)
	require.NoError(t, err)
	candidate := validDeploymentCandidate()
	candidate.Options.AutoStart = false
	firstDone := make(chan error, 1)
	secondDone := make(chan error, 1)
	go func() {
		firstDone <- service.Execute(context.Background(), "first", candidate, &composeServiceSinkFake{})
	}()
	<-guard.firstStarted
	go func() {
		secondDone <- service.Execute(context.Background(), "second", candidate, &composeServiceSinkFake{})
	}()
	select {
	case <-guard.secondStarted:
		t.Fatal("the second deployment entered preflight before the first deployment completed")
	case <-time.After(100 * time.Millisecond):
	}
	close(guard.releaseFirst)
	require.NoError(t, <-firstDone)
	secondErr := <-secondDone
	var structured *StructuredError
	require.ErrorAs(t, secondErr, &structured)
	require.Equal(t, ErrorCodeProjectExists, structured.Code)
}

func TestComposeServiceRejectsProjectSymlinkOutsideRoot(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	require.NoError(t, os.Symlink(outside, filepath.Join(root, "demo")))
	service, err := NewComposeService(root, "", &composeServiceRuntimeFake{}, nil)
	require.NoError(t, err)

	_, err = service.Preflight(context.Background(), validDeploymentCandidate())
	require.Error(t, err)
}

func TestComposeServiceRejectsExtraFileThroughSymlink(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	projectDir := filepath.Join(root, "demo")
	require.NoError(t, os.MkdirAll(projectDir, 0o755))
	require.NoError(t, os.Symlink(outside, filepath.Join(projectDir, "config")))
	service, err := NewComposeService(root, "", &composeServiceRuntimeFake{}, nil)
	require.NoError(t, err)
	candidate := validDeploymentCandidate()
	candidate.Options.AutoStart = false
	candidate.Files = []DeploymentFile{{Path: "config/app.ini", Content: "enabled=true\n", Mode: 0o600}}
	sink := &composeServiceSinkFake{}

	err = service.Execute(context.Background(), "symlink-file", candidate, sink)
	require.Error(t, err)
	require.NotEmpty(t, sink.errText)
	require.NoFileExists(t, filepath.Join(outside, "app.ini"))
}

func mustReadFile(t *testing.T, filePath string) string {
	t.Helper()
	content, err := os.ReadFile(filePath)
	require.NoError(t, err)
	return string(content)
}
