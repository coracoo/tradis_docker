package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"dockerpanel/backend/pkg/database"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/docker/docker/errdefs"
)

type composeUpdateService struct {
	Image      string    `json:"image"`
	Platform   string    `json:"platform"`
	PullPolicy string    `json:"pull_policy"`
	Build      *struct{} `json:"build"`
	DependsOn  map[string]struct {
		Required *bool `json:"required"`
	} `json:"depends_on"`
	Scale  *int `json:"scale"`
	Deploy struct {
		Replicas *int `json:"replicas"`
	} `json:"deploy"`
}

func (service composeUpdateService) desiredReplicas() int {
	if service.Scale != nil {
		return *service.Scale
	}
	if service.Deploy.Replicas != nil {
		return *service.Deploy.Replicas
	}
	return 1
}

type composeServiceUpdate struct {
	Service string `json:"service"`
	Image   string `json:"image,omitempty"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

type composeUpdateSummary struct {
	TaskID     string                 `json:"taskId"`
	FinishedAt string                 `json:"finishedAt"`
	Status     string                 `json:"status"`
	Services   []composeServiceUpdate `json:"services"`
}

func attachComposeUpdateSummaries(projects []*ComposeProject, containers []types.Container) error {
	tasks, err := database.LatestComposeUpdatesInEnvironment(database.LocalEnvironmentID)
	if err != nil {
		return err
	}
	for _, task := range tasks {
		var result struct {
			Project            string                 `json:"project"`
			ComposeProjectName string                 `json:"composeProjectName"`
			ProjectDir         string                 `json:"projectDir"`
			ServiceUpdates     []composeServiceUpdate `json:"serviceUpdates"`
			PullFailures       []composePullFailure   `json:"pullFailures"`
		}
		if json.Unmarshal([]byte(task.ResultJSON), &result) != nil {
			continue
		}
		name := result.ComposeProjectName
		if name == "" {
			name = result.Project
		}
		var candidates []*ComposeProject
		for _, project := range projects {
			if project.ComposeProjectName != name && !(project.ComposeProjectName == "" && project.Name == name) {
				continue
			}
			if result.ProjectDir != "" && filepath.Clean(project.Path) != filepath.Clean(result.ProjectDir) {
				continue
			}
			candidates = append(candidates, project)
		}
		if len(candidates) != 1 || candidates[0].LastUpdate != nil {
			continue
		}
		services := result.ServiceUpdates
		if len(services) == 0 {
			// Older tasks persisted no per-service result. Only a container created
			// during that task proves application; a version change remains unknown.
			started, startErr := time.Parse(time.RFC3339, task.CreatedAt)
			finished, finishErr := time.Parse(time.RFC3339, task.UpdatedAt)
			observed := map[string]bool{}
			byService := map[string]composeServiceUpdate{}
			for _, container := range containers {
				if container.Labels["com.docker.compose.project"] != name || strings.EqualFold(container.Labels["com.docker.compose.oneoff"], "true") {
					continue
				}
				service := container.Labels["com.docker.compose.service"]
				observed[service] = true
				if startErr == nil && finishErr == nil && container.Created >= started.Unix() && container.Created <= finished.Unix() && service != "" {
					byService[service] = composeServiceUpdate{Service: service, Image: container.Image, Status: "applied", Message: "本次已创建容器；历史任务未记录更新前镜像，无法确认版本变化"}
				}
			}
			for _, failure := range result.PullFailures {
				if failure.Service == "" {
					continue
				}
				status := "blocked"
				if observed[failure.Service] {
					status = "pull_failed"
				}
				byService[failure.Service] = composeServiceUpdate{Service: failure.Service, Image: failure.Image, Status: status, Message: composePullFailureWarning(failure)}
			}
			for _, update := range byService {
				services = append(services, update)
			}
			sort.Slice(services, func(i, j int) bool { return services[i].Service < services[j].Service })
		}
		candidates[0].LastUpdate = &composeUpdateSummary{TaskID: task.ID, FinishedAt: task.UpdatedAt, Status: task.Status, Services: services}
	}
	return nil
}

// Public shared code: local Compose updates in Full and Community use the same CLI.
type composeUpdatePullResult struct {
	Failures      []composePullFailure
	PulledImages  []string
	BuildServices []string
	Services      map[string]composeUpdateService
}

// pullComposeUpdateImages keeps each image/platform pull's exit status observable.
// Compose renders interpolation, overrides and active profiles; rendered values are
// never logged. A failed pull cannot interrupt another image's update.
func pullComposeUpdateImages(ctx context.Context, projectDir, projectName string, onLine func(string)) (composeUpdatePullResult, error) {
	result := composeUpdatePullResult{}
	configArgs := composeCommandWithProjectName(projectName, []string{"compose", "config", "--format", "json"})
	cmd, cleanup, err := newDockerCommand(ctx, projectDir, nil, configArgs)
	if err != nil {
		return result, fmt.Errorf("读取 Compose 更新配置失败: %w", err)
	}
	defer cleanup()
	output, err := cmd.Output() // Keep stderr warnings separate from JSON and secrets out of errors.
	if err != nil {
		return result, fmt.Errorf("读取 Compose 更新配置失败: %w", err)
	}
	var config struct {
		Services map[string]composeUpdateService `json:"services"`
	}
	if err := json.Unmarshal(output, &config); err != nil {
		return result, errors.New("无法解析 Compose 更新配置，请检查 Compose CLI 版本和项目配置")
	}
	result.Services = config.Services
	services := make([]string, 0, len(config.Services))
	for name := range config.Services {
		services = append(services, name)
	}
	sort.Strings(services)
	seen := map[string]bool{}
	failedImages := map[string]bool{}
	for _, name := range services {
		service := config.Services[name]
		image := strings.TrimSpace(service.Image)
		if service.PullPolicy == "build" {
			result.BuildServices = append(result.BuildServices, name)
			continue
		}
		if image == "" || service.PullPolicy == "never" {
			continue
		}
		key := image + "\x00" + service.Platform
		if seen[key] {
			continue
		}
		seen[key] = true
		if err := ctx.Err(); err != nil {
			return result, err
		}
		var failures []composePullFailure
		failureSeen := map[string]bool{}
		lastLine := ""
		pullArgs, _ := composeUpdateCommands()
		pullArgs = append(pullArgs, "--policy", "always", name)
		err := runComposeStreamLines(ctx, projectDir, composeCommandWithProjectName(projectName, pullArgs), func(line string) {
			line = sanitizeRuntimeLogLine(strings.TrimSpace(line))
			if line == "" {
				return
			}
			lastLine = line
			onLine(line)
			appendComposePullFailure(&failures, failureSeen, line)
		})
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		if err != nil {
			var exitErr *exec.ExitError
			if !errors.As(err, &exitErr) {
				return result, err // Setup/launch errors are not registry pull failures.
			}
			if len(failures) == 0 {
				message := lastLine
				if message == "" {
					message = err.Error()
				}
				failures = append(failures, composePullFailure{Message: message, Hint: composePullFailureHint(message)})
			}
		}
		if len(failures) > 0 {
			for i := range failures {
				failures[i].Service = name
				failures[i].Image = image
			}
			result.Failures = append(result.Failures, failures...)
			for _, variant := range normalizeImageVariants(image) {
				failedImages[variant] = true
			}
		} else {
			result.PulledImages = append(result.PulledImages, image)
		}
	}
	// A different platform may have failed for the same tag: retain its pending update.
	images := result.PulledImages[:0]
	for _, image := range result.PulledImages {
		failed := false
		for _, variant := range normalizeImageVariants(image) {
			failed = failed || failedImages[variant]
		}
		if !failed {
			images = append(images, image)
		}
	}
	result.PulledImages = images
	return result, nil
}

// applyComposeUpdateServices keeps dependency conditions inside each component
// under Compose's control. Missing images and their dependents are retained;
// a failed component cannot prevent an independent component from updating.
func applyComposeUpdateServices(ctx context.Context, projectDir, projectName string, wasRunning bool, pull composeUpdatePullResult, onLine func(string)) ([]composeServiceUpdate, []string, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, nil, err
	}
	defer cli.Close()
	before, err := listComposeProjectContainers(ctx, cli, projectName)
	if err != nil {
		return nil, nil, err
	}
	names := make([]string, 0, len(pull.Services))
	for name := range pull.Services {
		names = append(names, name)
	}
	sort.Strings(names)
	updates := make(map[string]*composeServiceUpdate, len(names))
	targetIDs := map[string]string{}
	pullFailed := map[string]bool{}
	for _, failure := range pull.Failures {
		for _, image := range normalizeImageVariants(failure.Image) {
			pullFailed[image] = true
		}
	}
	var problems []string
	for _, name := range names {
		service := pull.Services[name]
		updates[name] = &composeServiceUpdate{Service: name, Image: service.Image}
		image := service.Image
		if image == "" {
			image = projectName + "-" + name
		}
		if _, seen := targetIDs[image]; !seen {
			inspected, _, inspectErr := cli.ImageInspectWithRaw(ctx, image)
			if inspectErr != nil && !errdefs.IsNotFound(inspectErr) {
				return nil, nil, inspectErr
			}
			targetIDs[image] = inspected.ID
		}
		if targetIDs[image] == "" && service.Image != "" && service.Build == nil && service.desiredReplicas() != 0 {
			updates[name].Status = "blocked"
			updates[name].Message = "本地缺少镜像 " + service.Image + "，未应用更新"
		}
	}
	// Required dependencies must remain selected so Compose enforces their
	// healthy/completed conditions. Missing optional dependencies can be skipped.
	for changed := true; changed; {
		changed = false
		for _, name := range names {
			if updates[name].Status != "" {
				continue
			}
			dependencies := make([]string, 0, len(pull.Services[name].DependsOn))
			for dependency := range pull.Services[name].DependsOn {
				dependencies = append(dependencies, dependency)
			}
			sort.Strings(dependencies)
			for _, dependency := range dependencies {
				required := pull.Services[name].DependsOn[dependency].Required
				if required != nil && !*required {
					continue
				}
				if blocked := updates[dependency]; blocked != nil && blocked.Status == "blocked" {
					updates[name].Status = "blocked"
					updates[name].Message = "依赖服务 " + dependency + " 无法应用更新，已保留当前容器"
					changed = true
					break
				}
			}
		}
	}
	partial := false
	neighbors := map[string][]string{}
	for _, name := range names {
		if updates[name].Status == "blocked" {
			partial = true
			problems = append(problems, name+": "+updates[name].Message)
			continue
		}
		for dependency := range pull.Services[name].DependsOn {
			if updates[dependency] != nil && updates[dependency].Status == "" {
				neighbors[name] = append(neighbors[name], dependency)
				neighbors[dependency] = append(neighbors[dependency], name)
			}
		}
	}
	visited := map[string]bool{}
	applyErrors := map[string]string{}
	attempted := map[string]bool{}
	var interrupted error
	for _, name := range names {
		if updates[name].Status == "blocked" || visited[name] {
			continue
		}
		component := []string{}
		queue := []string{name}
		for len(queue) > 0 {
			next := queue[0]
			queue = queue[1:]
			if visited[next] {
				continue
			}
			visited[next] = true
			component = append(component, next)
			queue = append(queue, neighbors[next]...)
		}
		sort.Strings(component)
		if err := ctx.Err(); err != nil {
			interrupted = err
			break
		}
		_, args, _ := composeUpdateApplyCommand(wasRunning, true)
		if partial {
			// A partial update retains resources outside the selected services.
			filtered := args[:0]
			for _, arg := range args {
				if arg != "--remove-orphans" {
					filtered = append(filtered, arg)
				}
			}
			args = filtered
			if !wasRunning {
				args = []string{"compose", "up", "--no-start", "--pull", "never"}
			}
			// The component explicitly includes all its available required deps;
			// --no-deps only prevents missing optional deps from being reintroduced.
			args = append(args, "--no-deps")
		}
		args = append(args, component...)
		for _, service := range component {
			attempted[service] = true
		}
		lastLine := ""
		streamLine := func(line string) {
			lastLine = sanitizeRuntimeLogLine(line)
			onLine(lastLine)
		}
		var buildServices []string
		for _, service := range component {
			for _, buildService := range pull.BuildServices {
				if service == buildService {
					buildServices = append(buildServices, service)
				}
			}
		}
		var applyErr error
		if len(buildServices) > 0 {
			onLine("按项目配置构建镜像: " + strings.Join(buildServices, ", "))
			applyErr = runComposeStreamLines(ctx, projectDir, composeCommandWithProjectName(projectName, append([]string{"compose", "build"}, buildServices...)), streamLine)
		}
		if applyErr == nil {
			onLine("应用服务更新: " + strings.Join(component, ", "))
			lastLine = ""
			applyErr = runComposeStreamLines(ctx, projectDir, composeCommandWithProjectName(projectName, args), streamLine)
		}
		if ctx.Err() != nil {
			applyErr = ctx.Err()
			lastLine = "任务已取消或超时"
			interrupted = applyErr
		}
		if applyErr != nil {
			if lastLine == "" {
				lastLine = applyErr.Error()
			}
			problems = append(problems, strings.Join(component, ", ")+": "+lastLine)
			for _, service := range component {
				applyErrors[service] = lastLine
			}
			var exitErr *exec.ExitError
			if !errors.As(applyErr, &exitErr) {
				interrupted = applyErr
				break
			}
		}
	}
	// Cancellation stops mutations immediately. A bounded read still records
	// containers that were already updated before the interruption.
	observationCtx := ctx
	if interrupted != nil {
		var cancel context.CancelFunc
		observationCtx, cancel = context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
	}
	after, err := listComposeProjectContainers(observationCtx, cli, projectName)
	if err != nil {
		result := make([]composeServiceUpdate, 0, len(names))
		for _, name := range names {
			update := updates[name]
			if update.Status == "" {
				update.Status = "apply_failed"
				update.Message = "无法读取更新后容器，应用结果未确认"
			}
			result = append(result, *update)
		}
		return result, nil, errors.Join(interrupted, fmt.Errorf("读取更新后的容器失败: %w", err))
	}
	result := make([]composeServiceUpdate, 0, len(names))
	for _, name := range names {
		update := updates[name]
		service := pull.Services[name]
		if interrupted != nil && !attempted[name] && update.Status == "" {
			update.Status = "blocked"
			update.Message = "任务已中断，未应用此服务"
		}
		if update.Status == "blocked" {
			result = append(result, *update)
			continue
		}
		image := service.Image
		if image == "" {
			image = projectName + "-" + name
		}
		expected := targetIDs[image]
		// A build-only or buildable service may have created its image during apply.
		if service.Build != nil {
			inspected, _, inspectErr := cli.ImageInspectWithRaw(observationCtx, image)
			if inspectErr == nil {
				expected = inspected.ID
			}
		}
		matched, changed := 0, false
		mismatched := false
		for _, container := range after {
			if container.Labels["com.docker.compose.service"] != name || strings.EqualFold(container.Labels["com.docker.compose.oneoff"], "true") {
				continue
			}
			matched++
			if expected == "" || container.ImageID != expected {
				mismatched = true
			}
			previous := ""
			for _, old := range before {
				if old.Labels["com.docker.compose.service"] == name && !strings.EqualFold(old.Labels["com.docker.compose.oneoff"], "true") && old.Labels["com.docker.compose.container-number"] == container.Labels["com.docker.compose.container-number"] {
					previous = old.ImageID
					break
				}
			}
			changed = changed || previous != container.ImageID
		}
		desired := service.desiredReplicas()
		if desired == 0 && matched == 0 && applyErrors[name] == "" {
			update.Status = "unchanged"
			update.Message = "服务配置为零副本，未创建容器"
		} else if matched != desired || mismatched || (applyErrors[name] != "" && !changed) {
			update.Status = "apply_failed"
			update.Message = applyErrors[name]
			if update.Message == "" {
				update.Message = "更新后未找到使用目标镜像的容器"
				if matched != desired {
					update.Message = fmt.Sprintf("更新后服务副本不完整：期望 %d 个，实际 %d 个", desired, matched)
				}
				problems = append(problems, name+": "+update.Message)
			}
		} else if applyErrors[name] != "" {
			update.Status = "updated"
			update.Message = "容器镜像已更新，但所属服务组未完整完成: " + applyErrors[name]
		} else {
			failed := false
			for _, variant := range normalizeImageVariants(service.Image) {
				failed = failed || pullFailed[variant]
			}
			if failed {
				update.Status = "pull_failed"
				update.Message = "拉取失败，已沿用本地镜像"
			} else if changed {
				update.Status = "updated"
				update.Message = "容器已使用本次目标镜像"
			} else {
				update.Status = "unchanged"
				update.Message = "容器已使用当前镜像，无需重建"
			}
		}
		result = append(result, *update)
	}
	var appliedImages []string
	for _, image := range pull.PulledImages {
		ok := true
		variants := map[string]bool{}
		for _, variant := range normalizeImageVariants(image) {
			variants[variant] = true
		}
		for _, update := range result {
			for _, variant := range normalizeImageVariants(update.Image) {
				if variants[variant] && update.Status != "updated" && update.Status != "unchanged" {
					ok = false
				}
			}
		}
		if ok {
			appliedImages = append(appliedImages, image)
		}
	}
	if len(problems) > 0 {
		return result, appliedImages, errors.Join(interrupted, errors.New(strings.Join(problems, "；")))
	}
	return result, appliedImages, interrupted
}
