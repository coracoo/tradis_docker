package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"dockerpanel/backend/pkg/database"
	"dockerpanel/backend/pkg/deployment"
)

// localComposeServiceRuntime adapts the existing command runner and verifier
// to the transport-neutral deployment service. It intentionally keeps Docker
// command construction in api, where the local container mount rules live.
type localComposeServiceRuntime struct{}

var newLocalComposeServiceRuntime = func() deployment.ComposeRuntime {
	return localComposeServiceRuntime{}
}

func (localComposeServiceRuntime) ValidateConfig(ctx context.Context, projectDir, _ string, env []string) error {
	commandEnv := append([]string{"COMPOSE_PROGRESS=plain", "COMPOSE_NO_COLOR=1"}, env...)
	output, err := runDockerCombinedOutput(ctx, projectDir, commandEnv, []string{"compose", "config", "--quiet"})
	if err == nil {
		return nil
	}
	message := strings.TrimSpace(string(output))
	if message == "" {
		message = err.Error()
	}
	return fmt.Errorf("%s", message)
}

func (localComposeServiceRuntime) Run(ctx context.Context, projectDir string, env []string, arguments []string, onLine func(string)) error {
	commandEnv := append([]string{"COMPOSE_PROGRESS=plain", "COMPOSE_NO_COLOR=1"}, env...)
	return runCommandStreamLinesWithComposeSource(ctx, projectDir, commandEnv, arguments, filepath.Join(projectDir, "docker-compose.yml"), onLine)
}

func (localComposeServiceRuntime) Verify(ctx context.Context, projectName string) (deployment.VerificationResult, error) {
	return composeDeploymentVerifier(ctx, projectName)
}

type localComposeTaskSink struct {
	taskID      string
	taskType    string
	projectName string
	seq         int64
	finished    bool
}

func (sink *localComposeTaskSink) SetRunning() error {
	if err := database.UpsertTask(sink.taskID, sink.taskType, "running"); err != nil {
		return err
	}
	return sink.Append("info", fmt.Sprintf("开始部署项目：%s", sink.projectName))
}

func (sink *localComposeTaskSink) Append(logType, message string) error {
	sink.seq++
	return database.AppendTaskLogWithSeq(sink.taskID, sink.seq, time.Now(), logType, message)
}

func (sink *localComposeTaskSink) Finish(status string, result any, errText string) error {
	if sink.finished {
		return nil
	}
	if err := database.FinishTask(sink.taskID, status, result, errText); err != nil {
		return err
	}
	sink.finished = true
	invalidateComposeProjectListCache()
	st := strings.ToLower(strings.TrimSpace(status))
	if st == "success" || st == "completed" {
		err := database.SaveNotification(&database.Notification{
			Type: "success", Category: "deploy_task",
			Message: fmt.Sprintf("Compose 项目 %s 部署成功", sink.projectName), Read: false,
		})
		if err != nil {
			slog.Warn("deployment notification failed", "task_id", sink.taskID, "error", err)
		}
		return nil
	}
	if st == "canceled" {
		errText = "部署已取消"
	}
	if strings.TrimSpace(errText) == "" {
		errText = "未知错误"
	}
	err := database.SaveNotification(&database.Notification{
		Type: "error", Category: "deploy_task",
		Message: fmt.Sprintf("Compose 项目 %s 部署失败：%s", sink.projectName, errText), Read: false,
	})
	if err != nil {
		slog.Warn("deployment notification failed", "task_id", sink.taskID, "error", err)
	}
	return nil
}

func shouldUseSharedComposeService(taskType string, autoStart bool, options composeOperationOptions) bool {
	return taskType == "compose_deploy" && autoStart && !options.DeferVerification &&
		len(options.ExtraFiles) == 0 && len(options.Profiles) == 0 &&
		strings.TrimSpace(options.ProjectDir) == "" && strings.TrimSpace(options.ComposeProjectName) == ""
}

func runLocalComposeDeploymentWithService(ctx context.Context, taskID, taskType, projectName, compose, dotenvRaw, envRaw string, autoStart bool, options composeOperationOptions, initialSeq int64) bool {
	if !shouldUseSharedComposeService(taskType, autoStart, options) {
		return false
	}
	envMap := make(map[string]string)
	if strings.TrimSpace(envRaw) != "" {
		if err := json.Unmarshal([]byte(envRaw), &envMap); err != nil {
			envMap = make(map[string]string)
		}
	}
	dotenv := strings.ReplaceAll(dotenvRaw, "\r\n", "\n")
	if strings.TrimSpace(dotenv) == "" && len(envMap) > 0 {
		dotenv = renderDotenvFromMap(envMap)
	} else {
		for key, value := range envMap {
			dotenv = upsertDotenvKeyValue(dotenv, key, value)
		}
	}
	dotenv = filterDotenvByAllowedKeys(dotenv, extractComposeInterpolationKeys(compose))

	service, err := deployment.NewComposeService(getProjectsBaseDir(), effectiveHostProjectRoot(), newLocalComposeServiceRuntime(), nil)
	if err != nil {
		_ = database.UpsertTask(taskID, taskType, "running")
		_ = database.FinishTask(taskID, "error", nil, err.Error())
		return true
	}
	sink := &localComposeTaskSink{taskID: taskID, taskType: taskType, projectName: projectName, seq: initialSeq}
	candidate := deployment.DeploymentCandidate{
		Version:       deployment.DeploymentCandidateVersion,
		EnvironmentID: database.LocalEnvironmentID,
		ProjectName:   projectName,
		SourceType:    deployment.DeploymentSourceManualCompose,
		ComposeYAML:   compose,
		Dotenv:        dotenv,
		Options: deployment.DeploymentOptions{
			AutoStart: autoStart,
			Pull:      options.Pull,
			Rebuild:   options.Rebuild,
			Replace:   options.Replace,
		},
	}
	unlock := lockComposeProjectMutation(projectName)
	defer unlock()
	if err := service.Execute(ctx, taskID, candidate, sink); err != nil && !sink.finished {
		var persistence *deployment.TerminalPersistenceError
		if errors.As(err, &persistence) {
			if retryErr := sink.Finish(persistence.Status, persistence.Result, persistence.ErrorText); retryErr != nil {
				slog.Error("deployment outcome persistence failed; Docker execution will not be repeated", "task_id", taskID, "status", persistence.Status, "error", retryErr)
			}
			return true
		}
		status, errorText := "error", err.Error()
		var structured *deployment.StructuredError
		if errors.As(err, &structured) && errors.Unwrap(structured) != nil {
			errorText = errors.Unwrap(structured).Error()
		}
		if ctx != nil && ctx.Err() != nil {
			status, errorText = "canceled", "部署已取消"
		}
		_ = sink.SetRunning()
		_ = sink.Finish(status, nil, errorText)
	}
	return true
}
