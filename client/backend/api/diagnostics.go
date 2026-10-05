package api

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"strings"
	"time"
	"unicode/utf8"

	"dockerpanel/backend/pkg/database"
	"dockerpanel/backend/pkg/docker"
	"dockerpanel/backend/pkg/logging"
	"dockerpanel/backend/pkg/secrets"
	"dockerpanel/backend/pkg/settings"
	"dockerpanel/backend/pkg/system"

	"github.com/gin-gonic/gin"
)

const (
	diagnosticBundleFormatVersion = 1
	diagnosticTaskLimit           = 50
	diagnosticEventLimit          = 100
	diagnosticTextLimit           = 512
)

type diagnosticManifest struct {
	FormatVersion int      `json:"format_version"`
	GeneratedAt   string   `json:"generated_at"`
	ClientVersion string   `json:"client_version"`
	GoVersion     string   `json:"go_version"`
	Warnings      []string `json:"warnings,omitempty"`
}

type diagnosticSystem struct {
	OS                string `json:"os"`
	Arch              string `json:"arch"`
	DockerVersion     string `json:"docker_version,omitempty"`
	DockerAPIVersion  string `json:"docker_api_version,omitempty"`
	DockerOS          string `json:"docker_os,omitempty"`
	DockerArch        string `json:"docker_arch,omitempty"`
	CPUs              int    `json:"cpus,omitempty"`
	MemoryBytes       int64  `json:"memory_bytes,omitempty"`
	Containers        int    `json:"containers,omitempty"`
	ContainersRunning int    `json:"containers_running,omitempty"`
	ContainersPaused  int    `json:"containers_paused,omitempty"`
	ContainersStopped int    `json:"containers_stopped,omitempty"`
	Images            int    `json:"images,omitempty"`
}

type diagnosticTask struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	Status    string `json:"status"`
	Error     string `json:"error,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

type diagnosticEvent struct {
	Type      string `json:"type"`
	Message   string `json:"message"`
	Timestamp int64  `json:"timestamp,omitempty"`
}

type diagnosticBundle struct {
	Manifest diagnosticManifest
	System   diagnosticSystem
	Settings diagnosticSettings
	Tasks    []diagnosticTask
	Events   []diagnosticEvent
}

func diagnosticExportHandler(
	collector func(context.Context) (diagnosticBundle, error),
	now func() time.Time,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		bundle, err := collector(c.Request.Context())
		if err != nil {
			respondError(c, http.StatusInternalServerError, "生成诊断包失败", err)
			return
		}
		archive, err := buildDiagnosticArchive(bundle)
		if err != nil {
			respondError(c, http.StatusInternalServerError, "生成诊断包失败", err)
			return
		}

		filename := "tradis-diagnostics-" + now().Format("20060102-150405") + ".zip"
		c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
		c.Data(http.StatusOK, "application/zip", archive)
	}
}

func exportDiagnostics(c *gin.Context) {
	diagnosticExportHandler(collectDiagnosticBundle, time.Now)(c)
}

func collectDiagnosticBundle(ctx context.Context) (diagnosticBundle, error) {
	now := time.Now()
	bundle := diagnosticBundle{
		Manifest: diagnosticManifest{
			FormatVersion: diagnosticBundleFormatVersion,
			GeneratedAt:   now.Format(time.RFC3339),
			ClientVersion: resolveLocalClientVersion(),
			GoVersion:     runtime.Version(),
		},
		System: diagnosticSystem{OS: runtime.GOOS, Arch: runtime.GOARCH},
	}

	if cli, err := docker.NewDockerClient(); err != nil {
		bundle.Manifest.Warnings = append(bundle.Manifest.Warnings, diagnosticText("Docker 连接失败: "+err.Error()))
	} else {
		defer cli.Close()
		info, infoErr := cli.Info(ctx)
		if infoErr != nil {
			bundle.Manifest.Warnings = append(bundle.Manifest.Warnings, diagnosticText("Docker 信息读取失败: "+infoErr.Error()))
		} else {
			bundle.System.DockerVersion = strings.TrimSpace(info.ServerVersion)
			bundle.System.DockerOS = strings.TrimSpace(info.OperatingSystem)
			bundle.System.DockerArch = strings.TrimSpace(info.Architecture)
			bundle.System.CPUs = info.NCPU
			bundle.System.MemoryBytes = info.MemTotal
			bundle.System.Containers = info.Containers
			bundle.System.ContainersRunning = info.ContainersRunning
			bundle.System.ContainersPaused = info.ContainersPaused
			bundle.System.ContainersStopped = info.ContainersStopped
			bundle.System.Images = info.Images
		}
		version, versionErr := cli.ServerVersion(ctx)
		if versionErr != nil {
			bundle.Manifest.Warnings = append(bundle.Manifest.Warnings, diagnosticText("Docker 版本读取失败: "+versionErr.Error()))
		} else {
			bundle.System.DockerVersion = strings.TrimSpace(version.Version)
			bundle.System.DockerAPIVersion = strings.TrimSpace(version.APIVersion)
			if bundle.System.DockerArch == "" {
				bundle.System.DockerArch = strings.TrimSpace(version.Arch)
			}
		}
	}

	if current, err := settings.GetSettings(); err != nil {
		bundle.Manifest.Warnings = append(bundle.Manifest.Warnings, diagnosticText("设置读取失败: "+err.Error()))
	} else {
		bundle.Settings = diagnosticSettings{
			AdvancedMode:              current.AdvancedMode,
			AutoPortAllocationEnabled: current.AllowAutoAllocPort,
			VolumeBackupEnabled:       current.VolumeBackupEnabled,
			VolumeBackupRemoteSet:     current.VolumeBackupEnvSet,
			NotificationCategoryCount: len(current.NotificationEnabledCategories),
		}
		populateEditionDiagnosticSettings(&bundle.Settings, current)
	}

	if tasks, err := database.ListTasks(nil, nil, diagnosticTaskLimit); err != nil {
		bundle.Manifest.Warnings = append(bundle.Manifest.Warnings, diagnosticText("任务读取失败: "+err.Error()))
	} else {
		bundle.Tasks = make([]diagnosticTask, 0, len(tasks))
		for _, task := range tasks {
			bundle.Tasks = append(bundle.Tasks, diagnosticTask{
				ID:        diagnosticText(task.ID),
				Type:      diagnosticText(task.Type),
				Status:    diagnosticText(task.Status),
				Error:     diagnosticText(task.Error),
				CreatedAt: diagnosticText(task.CreatedAt),
				UpdatedAt: diagnosticText(task.UpdatedAt),
			})
		}
	}

	if events, err := system.GetRecentLogs(diagnosticEventLimit); err != nil {
		bundle.Manifest.Warnings = append(bundle.Manifest.Warnings, diagnosticText("事件读取失败: "+err.Error()))
	} else {
		bundle.Events = make([]diagnosticEvent, 0, len(events))
		for _, event := range events {
			bundle.Events = append(bundle.Events, diagnosticEvent{
				Type:      diagnosticText(event.Type),
				Message:   diagnosticText(event.Message),
				Timestamp: event.Timestamp,
			})
		}
	}

	return bundle, nil
}

func buildDiagnosticArchive(bundle diagnosticBundle) ([]byte, error) {
	if len(bundle.Tasks) > diagnosticTaskLimit {
		bundle.Tasks = bundle.Tasks[:diagnosticTaskLimit]
	}
	if len(bundle.Events) > diagnosticEventLimit {
		bundle.Events = bundle.Events[:diagnosticEventLimit]
	}
	for index := range bundle.Manifest.Warnings {
		bundle.Manifest.Warnings[index] = diagnosticText(bundle.Manifest.Warnings[index])
	}
	for index := range bundle.Tasks {
		bundle.Tasks[index].ID = diagnosticText(bundle.Tasks[index].ID)
		bundle.Tasks[index].Type = diagnosticText(bundle.Tasks[index].Type)
		bundle.Tasks[index].Status = diagnosticText(bundle.Tasks[index].Status)
		bundle.Tasks[index].Error = diagnosticText(bundle.Tasks[index].Error)
		bundle.Tasks[index].CreatedAt = diagnosticText(bundle.Tasks[index].CreatedAt)
		bundle.Tasks[index].UpdatedAt = diagnosticText(bundle.Tasks[index].UpdatedAt)
	}
	for index := range bundle.Events {
		bundle.Events[index].Type = diagnosticText(bundle.Events[index].Type)
		bundle.Events[index].Message = diagnosticText(bundle.Events[index].Message)
	}

	var output bytes.Buffer
	writer := zip.NewWriter(&output)
	files := []struct {
		name  string
		value any
	}{
		{name: "manifest.json", value: bundle.Manifest},
		{name: "system.json", value: bundle.System},
		{name: "settings.json", value: bundle.Settings},
		{name: "tasks.json", value: bundle.Tasks},
		{name: "events.json", value: bundle.Events},
	}
	for _, file := range files {
		entry, err := writer.Create(file.name)
		if err != nil {
			_ = writer.Close()
			return nil, err
		}
		encoder := json.NewEncoder(entry)
		encoder.SetIndent("", "  ")
		encoder.SetEscapeHTML(false)
		if err := encoder.Encode(file.value); err != nil {
			_ = writer.Close()
			return nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

func diagnosticText(value string) string {
	value = secrets.RedactString(value)
	value = settings.RedactAppStoreURL(value)
	value = logging.RedactText(value)
	value = strings.TrimSpace(value)
	if utf8.RuneCountInString(value) <= diagnosticTextLimit {
		return value
	}
	runes := []rune(value)
	return string(runes[:diagnosticTextLimit]) + "..."
}
