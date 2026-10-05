package deployment

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// ComposeRuntime isolates the target Docker/Compose implementation. The same
// service can therefore run on the Controller or inside tradis-agent.
type ComposeRuntime interface {
	ValidateConfig(ctx context.Context, projectDir, composePath string, env []string) error
	Run(ctx context.Context, projectDir string, env, arguments []string, onLine func(string)) error
	Verify(ctx context.Context, projectName string) (VerificationResult, error)
}

// ComposeTaskSink is deliberately small: persistence and user notifications
// remain the concern of the Controller or Agent task adapter.
type ComposeTaskSink interface {
	SetRunning() error
	Append(logType, message string) error
	Finish(status string, result any, errText string) error
}

// TerminalPersistenceError preserves the actual outcome for persistence-only retry.
type TerminalPersistenceError struct {
	Status    string
	Result    any
	ErrorText string
	Err       error
}

func (err *TerminalPersistenceError) Error() string {
	return "persist deployment outcome: " + err.Err.Error()
}
func (err *TerminalPersistenceError) Unwrap() error { return err.Err }

// ComposeProjectGuard hosts caller-specific project protection rules, such as
// the Controller/Agent self-project guards.
type ComposeProjectGuard interface {
	CheckProject(ctx context.Context, projectRoot, projectName string) error
}

type ComposePreflightResult struct {
	CandidateHash        string           `json:"candidate_hash"`
	ResolvedProjectName  string           `json:"resolved_project_name"`
	HostProjectPath      string           `json:"host_project_path,omitempty"`
	RequiresConfirmation bool             `json:"requires_confirmation"`
	Changes              []PlanChange     `json:"changes,omitempty"`
	Issues               []PreflightIssue `json:"issues,omitempty"`
}

// ComposeService owns only deterministic candidate execution. It has no Gin,
// database, notification, or source-template dependency.
type ComposeService struct {
	projectRoot     string
	hostProjectRoot string
	runtime         ComposeRuntime
	guard           ComposeProjectGuard
	locks           sync.Map // projectName -> *sync.Mutex
}

func NewComposeService(projectRoot, hostProjectRoot string, runtime ComposeRuntime, guard ComposeProjectGuard) (*ComposeService, error) {
	projectRoot = strings.TrimSpace(projectRoot)
	if projectRoot == "" {
		return nil, errors.New("compose project root is required")
	}
	if runtime == nil {
		return nil, errors.New("compose runtime is required")
	}
	absRoot, err := filepath.Abs(projectRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve compose project root: %w", err)
	}
	return &ComposeService{
		projectRoot:     filepath.Clean(absRoot),
		hostProjectRoot: strings.TrimRight(strings.TrimSpace(hostProjectRoot), string(filepath.Separator)),
		runtime:         runtime,
		guard:           guard,
	}, nil
}

func (service *ComposeService) Preflight(ctx context.Context, candidate DeploymentCandidate) (ComposePreflightResult, error) {
	normalized, err := NormalizeDeploymentCandidate(candidate)
	if err != nil {
		return ComposePreflightResult{}, err
	}
	if service.guard != nil {
		if err := service.guard.CheckProject(ctx, service.projectRoot, normalized.ProjectName); err != nil {
			return ComposePreflightResult{}, NewStructuredError(ErrorCodePreflightBlocked, err, false, nil, map[string]any{"project": normalized.ProjectName})
		}
	}
	projectDir, err := service.projectDir(normalized.ProjectName)
	if err != nil {
		return ComposePreflightResult{}, err
	}

	result := ComposePreflightResult{
		CandidateHash:       normalized.CandidateHash,
		ResolvedProjectName: normalized.ProjectName,
		HostProjectPath:     service.hostProjectPath(normalized.ProjectName),
	}
	existingCompose, exists, err := composeProjectExists(projectDir)
	if err != nil {
		return ComposePreflightResult{}, NewStructuredError(ErrorCodePreflightBlocked, err, false, nil, map[string]any{"project": normalized.ProjectName})
	}
	if exists && existingCompose {
		if !normalized.Options.Replace {
			return ComposePreflightResult{}, NewStructuredError(ErrorCodeProjectExists, errors.New("project compose already exists"), false, []string{"replace_project", "ask_user"}, map[string]any{"project": normalized.ProjectName})
		}
		result.RequiresConfirmation = true
		result.Changes = []PlanChange{{
			Type:       "replace",
			Target:     "project/" + normalized.ProjectName,
			Before:     "existing compose",
			After:      "candidate compose",
			ReasonCode: "project_exists",
		}}
	}
	absoluteBindIssues, err := composeAbsoluteBindIssues(normalized.ComposeYAML)
	if err != nil {
		return ComposePreflightResult{}, NewStructuredError(ErrorCodePreflightBlocked, err, false, []string{"fix_compose_configuration"}, map[string]any{"project": normalized.ProjectName})
	}
	if len(absoluteBindIssues) > 0 {
		result.RequiresConfirmation = true
		result.Issues = append(result.Issues, absoluteBindIssues...)
	}
	if err := service.validateCandidate(ctx, normalized); err != nil {
		return ComposePreflightResult{}, NewStructuredError(ErrorCodePreflightBlocked, err, false, []string{"fix_compose_configuration"}, map[string]any{"project": normalized.ProjectName})
	}
	return result, nil
}

func composeAbsoluteBindIssues(composeYAML string) ([]PreflightIssue, error) {
	var document map[string]any
	if err := yaml.Unmarshal([]byte(composeYAML), &document); err != nil {
		return nil, err
	}
	services, _ := stringMap(document["services"])
	issues := make([]PreflightIssue, 0)
	for _, serviceName := range sortedMapKeys(services) {
		service, _ := stringMap(services[serviceName])
		for _, source := range composeBindSources(service["volumes"]) {
			if !filepath.IsAbs(source) {
				continue
			}
			issues = append(issues, PreflightIssue{
				Code:        "absolute_bind_requires_confirmation",
				Severity:    PreflightWarning,
				NextActions: []string{"confirm_bind_path"},
				Details: map[string]any{
					"service": serviceName,
					"path":    filepath.Clean(source),
				},
			})
		}
	}
	return issues, nil
}

func (service *ComposeService) Execute(ctx context.Context, taskID string, candidate DeploymentCandidate, sink ComposeTaskSink) (returnErr error) {
	if strings.TrimSpace(taskID) == "" {
		return NewStructuredError(ErrorCodeInvalidInput, errors.New("task id is required"), false, nil, nil)
	}
	if sink == nil {
		return NewStructuredError(ErrorCodeInvalidInput, errors.New("task sink is required"), false, nil, nil)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	normalized, err := NormalizeDeploymentCandidate(candidate)
	if err != nil {
		return err
	}
	unlock := service.lock(normalized.ProjectName)
	defer unlock()
	if err := ctx.Err(); err != nil {
		return NewStructuredError(ErrorCodeCanceled, err, false, nil, nil)
	}
	if _, err := service.Preflight(ctx, normalized); err != nil {
		return err
	}
	if err := sink.SetRunning(); err != nil {
		return err
	}
	appendLog := func(logType, message string) { _ = sink.Append(logType, message) }
	var persistErr error
	defer func() {
		if persistErr != nil {
			returnErr = errors.Join(returnErr, persistErr)
		}
	}()
	finish := func(status string, result any, errText string) {
		if err := sink.Finish(status, result, errText); err != nil {
			persistErr = &TerminalPersistenceError{Status: status, Result: result, ErrorText: errText, Err: err}
		}
	}

	projectDir, err := service.projectDir(normalized.ProjectName)
	if err != nil {
		finish("error", nil, err.Error())
		return err
	}
	if err := service.materialize(projectDir, normalized); err != nil {
		finish("error", nil, err.Error())
		return NewStructuredError(ErrorCodeExecutionFailed, err, false, nil, map[string]any{"project": normalized.ProjectName})
	}
	appendLog("success", "配置已保存")
	if !normalized.Options.AutoStart {
		finish("success", map[string]any{"project": normalized.ProjectName, "autoStart": false}, "")
		return nil
	}

	env := service.composeEnvironment()
	run := func(arguments []string, message string) error {
		appendLog("info", message)
		return service.runtime.Run(ctx, projectDir, env, arguments, func(line string) {
			appendLog("info", line)
		})
	}
	if normalized.Options.Pull {
		if err := run([]string{"compose", "pull"}, "开始拉取镜像..."); err != nil {
			return service.finishExecutionError(finish, err, normalized.ProjectName)
		}
	}
	if normalized.Options.Rebuild {
		if err := run([]string{"compose", "build", "--pull", "--no-cache"}, "开始无缓存重构镜像..."); err != nil {
			return service.finishExecutionError(finish, err, normalized.ProjectName)
		}
	}
	upArguments := []string{"compose", "up", "-d"}
	if normalized.Options.Pull && !normalized.Options.Rebuild {
		upArguments = append(upArguments, "--pull", "always")
	}
	if err := run(upArguments, "正在启动服务..."); err != nil {
		return service.finishExecutionError(finish, err, normalized.ProjectName)
	}
	verification, err := service.runtime.Verify(ctx, normalized.ProjectName)
	if err != nil || !verification.Verified {
		if err == nil {
			err = errors.New("deployment verification failed")
		}
		finish("error", map[string]any{"project": normalized.ProjectName, "verification": verification}, err.Error())
		return NewStructuredError(ErrorCodeVerificationFailed, err, true, []string{"inspect_deployment", "retry"}, map[string]any{"project": normalized.ProjectName})
	}
	appendLog("success", "所有服务已成功启动")
	finish("success", map[string]any{"project": normalized.ProjectName, "verification": verification}, "")
	return nil
}

func (service *ComposeService) finishExecutionError(finish func(string, any, string), err error, projectName string) error {
	if errors.Is(err, context.DeadlineExceeded) {
		finish("error", nil, "部署执行超时")
		return NewStructuredError(ErrorCodeExecutionFailed, err, true, []string{"inspect_deployment", "retry"}, map[string]any{"project": projectName})
	}
	if errors.Is(err, context.Canceled) {
		finish("canceled", nil, "部署已取消")
		return NewStructuredError(ErrorCodeCanceled, err, false, nil, map[string]any{"project": projectName})
	}
	finish("error", nil, err.Error())
	return NewStructuredError(ErrorCodeExecutionFailed, err, true, []string{"inspect_deployment", "retry"}, map[string]any{"project": projectName})
}

func (service *ComposeService) projectDir(projectName string) (string, error) {
	projectDir := filepath.Join(service.projectRoot, projectName)
	relative, err := filepath.Rel(service.projectRoot, projectDir)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return "", candidateInputError("project path escapes root")
	}
	resolved, err := ResolveWithinRoot(service.projectRoot, projectName)
	if err != nil {
		return "", candidateInputError(err.Error())
	}
	return resolved, nil
}

func (service *ComposeService) hostProjectPath(projectName string) string {
	if service.hostProjectRoot == "" {
		return ""
	}
	return filepath.Join(service.hostProjectRoot, projectName)
}

func (service *ComposeService) lock(projectName string) func() {
	value, _ := service.locks.LoadOrStore(projectName, &sync.Mutex{})
	lock := value.(*sync.Mutex)
	lock.Lock()
	return lock.Unlock
}

func (service *ComposeService) validateCandidate(ctx context.Context, candidate DeploymentCandidate) error {
	if err := os.MkdirAll(service.projectRoot, 0o755); err != nil {
		return err
	}
	stageDir, err := os.MkdirTemp(service.projectRoot, ".tradis-preflight-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stageDir)
	if err := writeCandidateFiles(stageDir, candidate); err != nil {
		return err
	}
	return service.runtime.ValidateConfig(ctx, stageDir, filepath.Join(stageDir, "docker-compose.yml"), service.composeEnvironment())
}

// composeEnvironment reserves PROJECT_ROOT for the deployment target. Compose
// expands process variables before .env, so this prevents an imported dotenv
// from silently redirecting ${PROJECT_ROOT} binds away from the configured host
// project root. The same environment is used for preflight and execution.
func (service *ComposeService) composeEnvironment() []string {
	hostRoot := strings.TrimSpace(service.hostProjectRoot)
	if hostRoot == "" {
		return nil
	}
	return []string{"PROJECT_ROOT=" + hostRoot}
}

func (service *ComposeService) materialize(projectDir string, candidate DeploymentCandidate) error {
	if err := os.MkdirAll(service.projectRoot, 0o755); err != nil {
		return err
	}
	if _, exists, err := composeProjectExists(projectDir); err != nil {
		return err
	} else if !exists {
		stageDir, err := os.MkdirTemp(service.projectRoot, ".tradis-deploy-")
		if err != nil {
			return err
		}
		defer os.RemoveAll(stageDir)
		if err := writeCandidateFiles(stageDir, candidate); err != nil {
			return err
		}
		return os.Rename(stageDir, projectDir)
	}
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		return err
	}
	return writeCandidateFiles(projectDir, candidate)
}

func composeProjectExists(projectDir string) (hasCompose bool, exists bool, err error) {
	info, err := os.Stat(projectDir)
	if os.IsNotExist(err) {
		return false, false, nil
	}
	if err != nil {
		return false, false, err
	}
	if !info.IsDir() {
		return false, true, errors.New("project path is not a directory")
	}
	for _, name := range []string{"docker-compose.yml", "docker-compose.yaml", "compose.yml", "compose.yaml"} {
		fileInfo, statErr := os.Lstat(filepath.Join(projectDir, name))
		if os.IsNotExist(statErr) {
			continue
		}
		if statErr != nil {
			return false, true, statErr
		}
		if !fileInfo.Mode().IsRegular() {
			return false, true, fmt.Errorf("compose file is not a regular file: %s", name)
		}
		return true, true, nil
	}
	return false, true, nil
}

func writeCandidateFiles(projectDir string, candidate DeploymentCandidate) error {
	entries := append([]DeploymentFile{{Path: "docker-compose.yml", Content: candidate.ComposeYAML, Mode: 0o644}, {Path: ".env", Content: candidate.Dotenv, Mode: 0o600}}, candidate.Files...)
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	for _, entry := range entries {
		path, err := safeCandidateDestination(projectDir, entry.Path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := writeCandidateFileAtomic(path, []byte(entry.Content), os.FileMode(entry.Mode)); err != nil {
			return err
		}
	}
	return nil
}

func safeCandidateDestination(projectDir, filePath string) (string, error) {
	destination, err := ResolveWithinRoot(projectDir, filepath.FromSlash(filePath))
	if err != nil {
		return "", candidateInputError("extra file escapes project: " + err.Error())
	}
	return destination, nil
}

func writeCandidateFileAtomic(path string, content []byte, mode os.FileMode) error {
	temporary, err := os.CreateTemp(filepath.Dir(path), ".tradis-write-")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(mode); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(content); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryName, path)
}
