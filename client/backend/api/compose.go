package api

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"dockerpanel/backend/pkg/composehistory"
	"dockerpanel/backend/pkg/deployment"

	"github.com/docker/docker/api/types"
	dockercontainer "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/gin-gonic/gin"
	"gopkg.in/yaml.v3"

	"dockerpanel/backend/pkg/database"
	"dockerpanel/backend/pkg/logging"
	"dockerpanel/backend/pkg/settings"
)

const composeProjectListCacheTTL = 5 * time.Second

type composeProjectListCacheState struct {
	fetchedAt time.Time
	projects  []*ComposeProject
}

var composeProjectListCacheMu sync.Mutex
var composeProjectListCache composeProjectListCacheState
var composeProjectMutationLocks sync.Map
var composeGitSourceLookup = database.GetComposeGitSourceInEnvironment
var composeGitSourceUpsert = database.UpsertComposeGitSourceInEnvironment
var composeGitRename = os.Rename
var composeGitRemoveAll = os.RemoveAll
var composeDeploymentCommandRunnerWithSource = runComposeStreamLinesWithComposeSource
var composeDeploymentCommandRunner = defaultComposeDeploymentCommandRunner
var composeDeploymentVerifier = func(ctx context.Context, projectName string) (deployment.VerificationResult, error) {
	return deployment.VerifyDeployment(ctx, dockerDeploymentObserver{}, nil, deployment.VerificationRequest{
		EnvironmentID: database.LocalEnvironmentID,
		ProjectName:   projectName,
		Attempts:      20,
		Interval:      time.Second,
	})
}

func defaultComposeDeploymentCommandRunner(ctx context.Context, projectDir string, args []string, onLine func(string)) error {
	return composeDeploymentCommandRunnerWithSource(ctx, projectDir, filepath.Join(projectDir, "docker-compose.yml"), args, onLine)
}

var errComposeFileNotFound = errors.New("compose file not found")

// composeTaskCancels 记录运行中任务的取消函数，供 POST /compose/tasks/:id/cancel 触发。
var composeTaskCancels sync.Map // taskID -> context.CancelFunc

func registerComposeTaskCancel(taskID string, cancel context.CancelFunc) {
	if taskID == "" || cancel == nil {
		return
	}
	composeTaskCancels.Store(taskID, cancel)
}

func unregisterComposeTaskCancel(taskID string) {
	composeTaskCancels.Delete(taskID)
}

func lockComposeProjectMutation(projectName string) func() {
	lockValue, _ := composeProjectMutationLocks.LoadOrStore(projectName, &sync.Mutex{})
	projectLock := lockValue.(*sync.Mutex)
	projectLock.Lock()
	return projectLock.Unlock
}

func cloneComposeProjects(items []*ComposeProject) []*ComposeProject {
	if len(items) == 0 {
		return nil
	}
	out := make([]*ComposeProject, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		cp := *item
		out = append(out, &cp)
	}
	return out
}

func invalidateComposeProjectListCache() {
	dockerDiskDisplayCache.invalidate()
	composeProjectListCacheMu.Lock()
	composeProjectListCache = composeProjectListCacheState{}
	composeProjectListCacheMu.Unlock()
}

// getProjectsBaseDir 获取项目根目录
func getProjectsBaseDir() string {
	return settings.GetProjectRoot()
}

func normalizeComposeBindMountsForHostWithRootRef(composeContent, hostProjectDir, hostProjectRoot, containerProjectDir, containerProjectRoot, defaultContainerProjectDir, defaultContainerProjectRoot, hostProjectRootRef string, interpolationValues map[string]string) (string, error) {
	hostProjectDir = strings.TrimSpace(hostProjectDir)
	hostProjectRoot = strings.TrimSpace(hostProjectRoot)
	hostProjectRootRef = strings.TrimSpace(hostProjectRootRef)
	if hostProjectDir == "" {
		return composeContent, nil
	}
	if hostProjectRoot == "" {
		return "", fmt.Errorf("宿主机项目根目录未配置")
	}

	var root map[string]interface{}
	if err := yaml.Unmarshal([]byte(composeContent), &root); err != nil {
		return "", err
	}

	servicesRaw, ok := root["services"]
	if !ok {
		return composeContent, nil
	}
	services, ok := servicesRaw.(map[string]interface{})
	if !ok {
		return composeContent, nil
	}

	rewriteSource := func(source string) (string, bool, error) {
		src := strings.TrimSpace(source)
		if src == "" {
			return source, false, nil
		}
		if strings.Contains(src, "${") {
			expanded, err := expandComposeValue(src, interpolationValues)
			if err != nil {
				return "", false, err
			}
			src = strings.TrimSpace(expanded)
		}
		var candidate string
		switch {
		case src == ".":
			candidate = filepath.Clean(hostProjectDir)
		case strings.HasPrefix(src, "./") || strings.HasPrefix(src, "../"):
			candidate = filepath.Clean(filepath.Join(hostProjectDir, src))
		case containerProjectDir != "" && strings.HasPrefix(src, containerProjectDir):
			// 将容器内项目绝对路径转换为宿主机项目绝对路径
			rel, err := filepath.Rel(containerProjectDir, src)
			if err != nil {
				return "", false, err
			}
			candidate = filepath.Clean(filepath.Join(hostProjectDir, rel))
		case containerProjectRoot != "" && strings.HasPrefix(src, containerProjectRoot):
			// 将容器内项目根绝对路径转换为宿主机项目根绝对路径
			rel, err := filepath.Rel(containerProjectRoot, src)
			if err != nil {
				return "", false, err
			}
			candidate = filepath.Clean(filepath.Join(hostProjectRoot, rel))
		case defaultContainerProjectDir != "" && defaultContainerProjectDir != containerProjectDir && strings.HasPrefix(src, defaultContainerProjectDir):
			// 兼容旧版容器内项目绝对路径（如 /app/client/backend/project/xxx）
			rel, err := filepath.Rel(defaultContainerProjectDir, src)
			if err != nil {
				return "", false, err
			}
			candidate = filepath.Clean(filepath.Join(hostProjectDir, rel))
		case defaultContainerProjectRoot != "" && defaultContainerProjectRoot != containerProjectRoot && strings.HasPrefix(src, defaultContainerProjectRoot):
			// 兼容旧版容器内项目根绝对路径
			rel, err := filepath.Rel(defaultContainerProjectRoot, src)
			if err != nil {
				return "", false, err
			}
			candidate = filepath.Clean(filepath.Join(hostProjectRoot, rel))
		case filepath.IsAbs(src):
			// 已经是 PROJECT_ROOT 下的宿主机路径时，也统一渲染为 PROJECT_ROOT 引用。
			if err := validatePathWithinRoot(hostProjectRoot, src); err != nil {
				return source, false, nil
			}
			candidate = filepath.Clean(src)
		default:
			return source, false, nil
		}

		if err := validatePathWithinRoot(hostProjectRoot, candidate); err != nil {
			return "", false, fmt.Errorf("挂载路径 %q 超出 PROJECT_ROOT: %w", source, err)
		}
		if hostProjectRootRef != "" {
			rel, err := filepath.Rel(hostProjectRoot, candidate)
			if err != nil {
				return "", false, err
			}
			if rel == "." {
				return hostProjectRootRef, true, nil
			}
			return strings.TrimRight(hostProjectRootRef, "/") + "/" + filepath.ToSlash(rel), true, nil
		}
		return candidate, true, nil
	}

	for _, svcRaw := range services {
		svc, ok := svcRaw.(map[string]interface{})
		if !ok {
			continue
		}
		volRaw, ok := svc["volumes"]
		if !ok {
			continue
		}
		vols, ok := volRaw.([]interface{})
		if !ok {
			continue
		}

		for i := range vols {
			switch v := vols[i].(type) {
			case string:
				source, suffix, ok := splitComposeVolumeShortSyntax(v)
				if !ok {
					continue
				}
				newSrc, changed, err := rewriteSource(source)
				if err != nil {
					return "", err
				}
				if !changed {
					continue
				}
				vols[i] = newSrc + ":" + suffix
			case map[string]interface{}:
				srcVal, ok := v["source"]
				if !ok {
					continue
				}
				srcStr, ok := srcVal.(string)
				if !ok {
					continue
				}
				newSrc, changed, err := rewriteSource(srcStr)
				if err != nil {
					return "", err
				}
				if !changed {
					continue
				}
				v["source"] = newSrc
				vols[i] = v
			}
		}

		svc["volumes"] = vols
	}

	out, err := marshalComposeYAMLOrdered(root)
	if err != nil {
		return "", err
	}
	return out, nil
}

func isRelativeComposeBindSource(source string) bool {
	src := strings.TrimSpace(source)
	return src == "." || strings.HasPrefix(src, "./") || strings.HasPrefix(src, "../")
}

func splitComposeVolumeShortSyntax(value string) (string, string, bool) {
	depth := 0
	for i := 0; i < len(value); i++ {
		switch value[i] {
		case '{':
			if i > 0 && value[i-1] == '$' {
				depth++
			}
		case '}':
			if depth > 0 {
				depth--
			}
		case ':':
			if depth == 0 {
				return value[:i], value[i+1:], true
			}
		}
	}
	return "", "", false
}

func composeHasRelativeBindMounts(composeContent string) bool {
	var root map[string]interface{}
	if err := yaml.Unmarshal([]byte(composeContent), &root); err != nil {
		return false
	}

	servicesRaw, ok := root["services"]
	if !ok {
		return false
	}
	services, ok := servicesRaw.(map[string]interface{})
	if !ok {
		return false
	}

	for _, svcRaw := range services {
		svc, ok := svcRaw.(map[string]interface{})
		if !ok {
			continue
		}
		volRaw, ok := svc["volumes"]
		if !ok {
			continue
		}
		vols, ok := volRaw.([]interface{})
		if !ok {
			continue
		}

		for _, item := range vols {
			switch v := item.(type) {
			case string:
				source, _, ok := splitComposeVolumeShortSyntax(v)
				if ok && isRelativeComposeBindSource(source) {
					return true
				}
			case map[string]interface{}:
				typeVal, _ := v["type"].(string)
				if typeVal != "" && !strings.EqualFold(strings.TrimSpace(typeVal), "bind") {
					continue
				}
				srcVal, ok := v["source"]
				if !ok {
					srcVal = v["src"]
				}
				srcStr, ok := srcVal.(string)
				if ok && isRelativeComposeBindSource(srcStr) {
					return true
				}
			}
		}
	}
	return false
}

func expandComposeValue(raw string, values map[string]string) (string, error) {
	var out strings.Builder
	for i := 0; i < len(raw); {
		if raw[i] != '$' || i+1 >= len(raw) || raw[i+1] != '{' {
			out.WriteByte(raw[i])
			i++
			continue
		}
		end := strings.IndexByte(raw[i+2:], '}')
		if end < 0 {
			return "", fmt.Errorf("未闭合的变量表达式: %s", raw)
		}
		end += i + 2
		expr := raw[i+2 : end]
		name := expr
		fallback := ""
		mode := ""
		if idx := strings.Index(expr, ":-"); idx >= 0 {
			name, fallback, mode = expr[:idx], expr[idx+2:], ":-"
		} else if idx := strings.Index(expr, "-"); idx >= 0 {
			name, fallback, mode = expr[:idx], expr[idx+1:], "-"
		}
		name = strings.TrimSpace(name)
		if !isLikelyEnvKey(name) {
			return "", fmt.Errorf("无效的变量表达式: ${%s}", expr)
		}
		value, exists := values[name]
		switch mode {
		case ":-":
			if !exists || value == "" {
				value = fallback
			}
		case "-":
			if !exists {
				value = fallback
			}
		default:
			if !exists {
				return "", fmt.Errorf("volume 路径变量 %s 未配置", name)
			}
		}
		out.WriteString(value)
		i = end + 1
	}
	return out.String(), nil
}

func normalizeComposeBindMountsForRuntime(composeContent string, projectName string, interpolationValues ...map[string]string) (string, error) {
	return normalizeComposeBindMountsForRuntimeInComposeDir(composeContent, projectName, "", interpolationValues...)
}

func normalizeComposeBindMountsForRuntimeInComposeDir(composeContent, projectName, composeRelativeDir string, interpolationValues ...map[string]string) (string, error) {
	hostRoot := effectiveHostProjectRoot()
	containerRoot := getProjectsBaseDir()
	if hostRoot == "" {
		if composeHasRelativeBindMounts(composeContent) {
			return "", fmt.Errorf("无法确定 Docker daemon 的宿主机项目目录，请设置 PROJECT_ROOT")
		}
		return composeContent, nil
	}

	composeRelativeDir = filepath.Clean(strings.TrimSpace(composeRelativeDir))
	if composeRelativeDir == "." {
		composeRelativeDir = ""
	}
	if filepath.IsAbs(composeRelativeDir) || composeRelativeDir == ".." || strings.HasPrefix(composeRelativeDir, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("Compose 目录超出项目目录")
	}

	hostProjectDir := filepath.Join(hostRoot, projectName, composeRelativeDir)
	containerProjectDir := filepath.Join(containerRoot, projectName, composeRelativeDir)
	defaultContainerRoot := filepath.Join(filepath.Dir(settings.GetDataDir()), "project")
	defaultContainerProjectDir := filepath.Join(defaultContainerRoot, projectName, composeRelativeDir)
	values := map[string]string{}
	if len(interpolationValues) > 0 && interpolationValues[0] != nil {
		values = interpolationValues[0]
	}
	return normalizeComposeBindMountsForHostWithRootRef(
		composeContent,
		hostProjectDir,
		hostRoot,
		containerProjectDir,
		containerRoot,
		defaultContainerProjectDir,
		defaultContainerRoot,
		"${PROJECT_ROOT}",
		values,
	)
}

// marshalComposeYAMLOrdered 将 Compose 数据结构序列化为 YAML，并统一字段顺序。
func marshalComposeYAMLOrdered(v any) (string, error) {
	b, err := yaml.Marshal(v)
	if err != nil {
		return "", err
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(b, &doc); err != nil {
		return "", err
	}
	if len(doc.Content) > 0 {
		reorderComposeRootNode(doc.Content[0])
	}

	if len(doc.Content) == 0 {
		return string(b), nil
	}

	out, err := yaml.Marshal(doc.Content[0])
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func reorderComposeRootNode(root *yaml.Node) {
	if root == nil {
		return
	}
	if root.Kind != yaml.MappingNode {
		return
	}

	reorderMappingNodeWithPreferredKeys(root, []string{"version", "name", "services", "networks", "volumes", "configs", "secrets"})

	services := mappingGetValue(root, "services")
	if services != nil {
		reorderComposeServicesNode(services)
	}
}

func reorderComposeServicesNode(services *yaml.Node) {
	if services == nil {
		return
	}
	if services.Kind != yaml.MappingNode {
		return
	}
	for i := 0; i+1 < len(services.Content); i += 2 {
		svcVal := services.Content[i+1]
		reorderComposeServiceNode(svcVal)
	}
}

func reorderComposeServiceNode(service *yaml.Node) {
	if service == nil {
		return
	}
	if service.Kind != yaml.MappingNode {
		return
	}

	firstKeys := map[string]bool{"image": true, "ports": true, "volumes": true, "env_file": true, "environment": true}
	lastKeys := map[string]bool{"healthcheck": true, "command": true, "cmd": true, "entrypoint": true}

	content := service.Content
	first := make([]*yaml.Node, 0, len(content))
	others := make([]*yaml.Node, 0, len(content))
	health := make([]*yaml.Node, 0, 2)
	cmd := make([]*yaml.Node, 0, 2)

	pushPair := func(dst *[]*yaml.Node, k *yaml.Node, v *yaml.Node) {
		*dst = append(*dst, k, v)
	}

	seen := map[string]bool{}
	getPair := func(name string) (*yaml.Node, *yaml.Node, bool) {
		for i := 0; i+1 < len(content); i += 2 {
			k := content[i]
			v := content[i+1]
			if k.Kind == yaml.ScalarNode && k.Value == name {
				return k, v, true
			}
		}
		return nil, nil, false
	}

	for _, kname := range []string{"image", "ports", "volumes", "env_file", "environment"} {
		k, v, ok := getPair(kname)
		if !ok {
			continue
		}
		pushPair(&first, k, v)
		seen[kname] = true
	}

	for i := 0; i+1 < len(content); i += 2 {
		k := content[i]
		v := content[i+1]
		name := ""
		if k.Kind == yaml.ScalarNode {
			name = k.Value
		}
		if name != "" {
			if seen[name] {
				continue
			}
			seen[name] = true
			if firstKeys[name] {
				continue
			}
			if lastKeys[name] {
				switch name {
				case "healthcheck":
					pushPair(&health, k, v)
				case "command", "cmd", "entrypoint":
					pushPair(&cmd, k, v)
				}
				continue
			}
		}
		pushPair(&others, k, v)
	}

	newContent := make([]*yaml.Node, 0, len(content))
	newContent = append(newContent, first...)
	newContent = append(newContent, others...)
	newContent = append(newContent, health...)
	newContent = append(newContent, cmd...)
	service.Content = newContent
}

func reorderMappingNodeWithPreferredKeys(node *yaml.Node, preferred []string) {
	if node == nil {
		return
	}
	if node.Kind != yaml.MappingNode {
		return
	}

	content := node.Content
	if len(content) < 2 {
		return
	}

	used := make([]bool, len(content))
	newContent := make([]*yaml.Node, 0, len(content))

	for _, key := range preferred {
		for i := 0; i+1 < len(content); i += 2 {
			k := content[i]
			if k.Kind == yaml.ScalarNode && k.Value == key {
				newContent = append(newContent, content[i], content[i+1])
				used[i] = true
				used[i+1] = true
				break
			}
		}
	}

	for i := 0; i+1 < len(content); i += 2 {
		if used[i] {
			continue
		}
		newContent = append(newContent, content[i], content[i+1])
	}

	node.Content = newContent
}

func mappingGetValue(node *yaml.Node, key string) *yaml.Node {
	if node == nil {
		return nil
	}
	if node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		k := node.Content[i]
		v := node.Content[i+1]
		if k.Kind == yaml.ScalarNode && k.Value == key {
			return v
		}
	}
	return nil
}

var selfIdentityOnce sync.Once
var selfContainerID string
var selfContainerName string
var selfComposeProject string
var selfComposeDirName string
var selfComposeWorkingDir string
var selfComposeConfigFiles []string
var selfHostProjectRootOnce sync.Once
var selfHostProjectRoot string

const protectedComposeProjectName = "tradis"
const protectedImageRepo = "coracoo/tradis"

func effectiveHostProjectRoot() string {
	if envRoot := settings.GetHostProjectRoot(); envRoot != "" {
		return envRoot
	}
	return detectHostProjectRootFromSelfMount()
}

func detectHostProjectRootFromSelfMount() string {
	selfHostProjectRootOnce.Do(func() {
		id := detectSelfContainerID()
		if id == "" {
			return
		}

		cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
		if err != nil {
			return
		}
		defer cli.Close()

		inspect, err := cli.ContainerInspect(context.Background(), id)
		if err != nil && len(id) >= 8 {
			containers, listErr := cli.ContainerList(context.Background(), types.ContainerListOptions{All: true})
			if listErr == nil {
				for _, ctr := range containers {
					if strings.HasPrefix(ctr.ID, id) {
						inspect, err = cli.ContainerInspect(context.Background(), ctr.ID)
						if err == nil {
							break
						}
					}
				}
			}
		}
		if err != nil {
			return
		}

		containerProjectRoot := filepath.Clean(settings.GetProjectRoot())
		for _, mount := range inspect.Mounts {
			source := strings.TrimSpace(mount.Source)
			destination := strings.TrimSpace(mount.Destination)
			if source == "" || destination == "" {
				continue
			}

			dest := filepath.Clean(destination)
			if dest == containerProjectRoot {
				selfHostProjectRoot = filepath.Clean(source)
				return
			}

			rel, relErr := filepath.Rel(dest, containerProjectRoot)
			if relErr != nil || rel == "." || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
				continue
			}
			selfHostProjectRoot = filepath.Clean(filepath.Join(source, rel))
			return
		}
	})
	return selfHostProjectRoot
}

func isProtectedImage(image string) bool {
	v := strings.TrimSpace(image)
	if v == "" {
		return false
	}
	if v == protectedImageRepo || strings.HasPrefix(v, protectedImageRepo+"@") {
		return true
	}
	tagPrefix := protectedImageRepo + ":"
	if !strings.HasPrefix(v, tagPrefix) {
		return false
	}

	// Client 与 Server 历史上共用 coracoo/tradis 仓库。server_* / server-*
	// 标签属于模板管理服务，不是当前 Client 面板自身，不能禁止其终端和日志。
	tag := strings.ToLower(strings.TrimPrefix(v, tagPrefix))
	return !strings.HasPrefix(tag, "server_") && !strings.HasPrefix(tag, "server-")
}

func isProtectedLabels(labels map[string]string) bool {
	if len(labels) == 0 {
		return false
	}
	return isSelfProjectName(labels["com.docker.compose.project"])
}

func isProtectedContainer(image string, labels map[string]string) bool {
	return isProtectedImage(image) || isProtectedLabels(labels)
}

func isSelfOrProtectedContainer(containerID string, containerName string, image string, labels map[string]string) bool {
	if containerID != "" && isSelfContainerID(containerID) {
		return true
	}
	if containerName != "" && isSelfContainerID(containerName) {
		return true
	}
	return isProtectedContainer(image, labels)
}

func getSelfIdentity() (containerID string, containerName string, composeProject string, composeDirName string) {
	selfIdentityOnce.Do(func() {
		id := detectSelfContainerID()
		if id == "" {
			return
		}

		cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
		if err != nil {
			return
		}
		defer cli.Close()

		inspect, err := cli.ContainerInspect(context.Background(), id)
		if err != nil {
			if len(id) >= 8 {
				containers, listErr := cli.ContainerList(context.Background(), types.ContainerListOptions{All: true})
				if listErr == nil {
					for _, ctr := range containers {
						if strings.HasPrefix(ctr.ID, id) {
							inspect, err = cli.ContainerInspect(context.Background(), ctr.ID)
							if err == nil {
								break
							}
						}
					}
				}
			}
		}
		if err != nil {
			return
		}

		selfContainerID = inspect.ID
		selfContainerName = strings.TrimPrefix(inspect.Name, "/")
		if inspect.Config != nil && inspect.Config.Labels != nil {
			selfComposeProject = strings.TrimSpace(inspect.Config.Labels["com.docker.compose.project"])
			workingDir := strings.TrimSpace(inspect.Config.Labels["com.docker.compose.project.working_dir"])
			configFiles := strings.TrimSpace(inspect.Config.Labels["com.docker.compose.project.config_files"])
			if workingDir != "" {
				selfComposeWorkingDir = workingDir
				selfComposeDirName = filepath.Base(workingDir)
			} else if configFiles != "" {
				selfComposeDirName = filepath.Base(filepath.Dir(configFiles))
			}
			if configFiles != "" {
				for _, file := range strings.Split(configFiles, ",") {
					if file = strings.TrimSpace(file); file != "" {
						selfComposeConfigFiles = append(selfComposeConfigFiles, file)
					}
				}
			}
		}
	})
	return selfContainerID, selfContainerName, selfComposeProject, selfComposeDirName
}

// getSelfComposeConfigLocation 返回 tradis 自身容器的 Compose 工作目录与配置文件
// 绝对路径（宿主机路径，来自 compose 标准 label）。label 缺失时返回空值，
// 调用方据此优雅降级。
func getSelfComposeConfigLocation() (workingDir string, configFiles []string) {
	getSelfIdentity()
	return selfComposeWorkingDir, append([]string(nil), selfComposeConfigFiles...)
}

func detectSelfContainerID() string {
	hostname, _ := os.Hostname()
	h := strings.TrimSpace(strings.ToLower(hostname))
	idRe := regexp.MustCompile(`^[0-9a-f]{12,64}$`)
	if idRe.MatchString(h) {
		return h
	}

	cgroupBytes, err := os.ReadFile("/proc/self/cgroup")
	if err == nil {
		lines := strings.Split(string(cgroupBytes), "\n")
		findRe := regexp.MustCompile(`([0-9a-f]{64}|[0-9a-f]{12})`)
		for _, line := range lines {
			l := strings.ToLower(line)
			if !strings.Contains(l, "docker") && !strings.Contains(l, "kubepods") && !strings.Contains(l, "containerd") {
				continue
			}
			m := findRe.FindStringSubmatch(l)
			if len(m) > 1 {
				return m[1]
			}
		}
	}

	// cgroup v2 / network_mode: host 等场景下，/proc/self/cgroup 可能不包含容器 ID，
	// 此时可尝试从 /proc/1/cpuset 读取。
	cpusetBytes, err := os.ReadFile("/proc/1/cpuset")
	if err == nil {
		findRe := regexp.MustCompile(`([0-9a-f]{64}|[0-9a-f]{12})`)
		parts := strings.Split(string(cpusetBytes), "/")
		for _, part := range parts {
			part = strings.TrimSpace(strings.ToLower(part))
			if findRe.MatchString(part) {
				return part
			}
		}
	}

	// host 网络下 hostname 与 cgroup 都拿不到容器 ID；Docker 会把
	// /var/lib/docker/containers/<id>/hostname 等文件挂进容器，mountinfo 可靠暴露完整 ID。
	if id := containerIDFromMountInfo(); id != "" {
		return id
	}

	return ""
}

// containerIDFromMountInfo 从 /proc/self/mountinfo 中解析 Docker 注入的
// containers/<64 位 ID>/ 挂载源路径，取出现次数最多的完整容器 ID。
func containerIDFromMountInfo() string {
	data, err := os.ReadFile("/proc/self/mountinfo")
	if err != nil {
		return ""
	}
	return containerIDFromMountInfoContent(string(data))
}

func containerIDFromMountInfoContent(content string) string {
	idRe := regexp.MustCompile(`containers/([0-9a-f]{64})/`)
	counts := make(map[string]int)
	best, bestCount := "", 0
	for _, m := range idRe.FindAllStringSubmatch(content, -1) {
		counts[m[1]]++
		if counts[m[1]] > bestCount {
			best, bestCount = m[1], counts[m[1]]
		}
	}
	return best
}

func isSelfContainerID(id string) bool {
	selfID, selfName, _, _ := getSelfIdentity()
	if selfID == "" && selfName == "" {
		return false
	}
	raw := strings.TrimSpace(strings.TrimPrefix(id, "/"))
	if raw == "" {
		return false
	}
	if selfName != "" && raw == selfName {
		return true
	}
	if selfID == "" {
		return false
	}
	return raw == selfID || strings.HasPrefix(selfID, raw) || strings.HasPrefix(raw, selfID)
}

func forbidIfSelfContainer(c *gin.Context, containerID string) bool {
	if err := validateContainerManagementTarget(c.Request.Context(), containerID); err != nil {
		respondError(c, http.StatusForbidden, err.Error(), nil)
		return true
	}
	return false
}

func validateContainerManagementTarget(ctx context.Context, containerID string) error {
	if isSelfContainerID(containerID) {
		return errors.New("容器化部署模式下，禁止管理自身容器")
	}
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil
	}
	defer cli.Close()

	inspect, err := cli.ContainerInspect(ctx, containerID)
	if err != nil {
		return nil
	}

	if inspect.Config != nil && isProtectedContainer(inspect.Config.Image, inspect.Config.Labels) {
		return errors.New("容器化部署模式下，禁止管理自身容器")
	}

	return nil
}

func isSelfProjectName(name string) bool {
	_, _, composeProject, composeDir := getSelfIdentity()
	return isSelfProjectNameForIdentity(name, composeProject, composeDir)
}

func isSelfProjectNameForIdentity(name, composeProject, composeDir string) bool {
	n := strings.TrimSpace(name)
	if strings.EqualFold(n, protectedComposeProjectName) {
		return true
	}
	if composeProject == "" && composeDir == "" {
		return false
	}
	if n == "" {
		return false
	}
	return (composeProject != "" && strings.EqualFold(n, composeProject)) ||
		(composeDir != "" && strings.EqualFold(n, composeDir))
}

func forbidIfSelfProject(c *gin.Context, projectName string) bool {
	if !isSelfProjectName(projectName) {
		return false
	}
	respondError(c, http.StatusForbidden, "容器化部署模式下，禁止管理自身项目", nil)
	return true
}

var composeNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

// 独立容器备注在 compose_project_metadata 中用该前缀隔离，避免与同名 Compose 项目冲突。
const containerRemarkKeyPrefix = "container:"

var containerNameRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)

func validateComposeProjectName(raw string) (string, bool) {
	name := strings.TrimSpace(strings.ToLower(raw))
	if name == "" {
		return "", false
	}
	if !composeNameRe.MatchString(name) {
		return "", false
	}
	return name, true
}

// ComposeProject 定义项目结构
type ComposeProject struct {
	Name               string    `json:"name"`
	ComposeProjectName string    `json:"composeProjectName,omitempty"`
	IdentitySource     string    `json:"identitySource,omitempty"`
	IdentityError      string    `json:"identityError,omitempty"`
	Remark             string    `json:"remark,omitempty"`
	Path               string    `json:"path"`
	Compose            string    `json:"compose"`
	AutoStart          bool      `json:"autoStart"`
	Containers         int       `json:"containers"`
	Status             string    `json:"status"`
	UpdateAvailable    bool      `json:"updateAvailable"`
	UpdateCount        int       `json:"updateCount"`
	CreateTime         time.Time `json:"createTime"`
	IsSelf             bool      `json:"isSelf"`
	IsManaged          bool      `json:"isManaged"`
	// InvalidName is kept in the response contract for old frontends. New
	// identity resolution separates filesystem names from Compose names, so
	// Unicode and uppercase directories no longer set it.
	InvalidName   bool                       `json:"invalidName"`
	InvalidReason string                     `json:"invalidReason,omitempty"`
	GitSource     *database.ComposeGitSource `json:"gitSource,omitempty"`
}

func setSSEHeaders(c *gin.Context) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
}

func sseNextIDFromLastEventID(c *gin.Context) int64 {
	raw := strings.TrimSpace(c.GetHeader("Last-Event-ID"))
	if raw == "" {
		return 1
	}
	last, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || last < 0 {
		return 1
	}
	return last + 1
}

func sseWriteStringEvent(c *gin.Context, id int64, event string, data string) {
	_, _ = fmt.Fprintf(c.Writer, "id: %d\n", id)
	if event != "" && event != "message" {
		_, _ = fmt.Fprintf(c.Writer, "event: %s\n", event)
	}

	payload := strings.ReplaceAll(string(data), "\r\n", "\n")
	payload = strings.TrimRight(payload, "\n")
	if payload == "" {
		_, _ = c.Writer.WriteString("data:\n\n")
		c.Writer.Flush()
		return
	}

	for _, line := range strings.Split(payload, "\n") {
		_, _ = fmt.Fprintf(c.Writer, "data: %s\n", line)
	}
	_, _ = c.Writer.WriteString("\n")
	c.Writer.Flush()
}

func sseWriteJSONEvent(c *gin.Context, id int64, event string, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		sseWriteStringEvent(c, id, event, fmt.Sprintf(`{"type":"error","message":"marshal failed: %s"}`, err.Error()))
		return
	}
	sseWriteStringEvent(c, id, event, string(b))
}

func standardExampleCandidates(target string) []string {
	candidates := []string{
		target + ".example",
		target + ".sample",
		target + ".template",
	}
	if filepath.Base(target) == ".env" {
		candidates = append(candidates, filepath.Join(filepath.Dir(target), "example.env"))
	}
	return candidates
}

func copyFirstStandardExample(projectDir, target string) (string, error) {
	cleanTarget := filepath.Clean(strings.TrimSpace(target))
	if cleanTarget == "." || filepath.IsAbs(cleanTarget) || cleanTarget == ".." || strings.HasPrefix(cleanTarget, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("配置文件路径不安全: %s", target)
	}
	targetPath := filepath.Join(projectDir, cleanTarget)
	if _, err := os.Stat(targetPath); err == nil {
		return "", nil
	} else if err != nil && !os.IsNotExist(err) {
		return "", err
	}

	for _, candidate := range standardExampleCandidates(cleanTarget) {
		candidatePath := filepath.Join(projectDir, candidate)
		info, err := os.Stat(candidatePath)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		content, err := os.ReadFile(candidatePath)
		if err != nil {
			return "", err
		}
		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			return "", err
		}
		if err := os.WriteFile(targetPath, content, info.Mode().Perm()); err != nil {
			return "", err
		}
		return candidate, nil
	}
	return "", nil
}

func looksLikeEnvFilePath(path string) bool {
	base := strings.ToLower(filepath.Base(filepath.Clean(path)))
	return base == ".env" || strings.HasPrefix(base, ".env.") || strings.HasSuffix(base, ".env")
}

func collectRelativeBindSources(composeContent string, values map[string]string) ([]string, error) {
	var root map[string]any
	if err := yaml.Unmarshal([]byte(composeContent), &root); err != nil {
		return nil, err
	}
	services, _ := root["services"].(map[string]any)
	seen := map[string]struct{}{}
	out := []string{}
	add := func(source string) error {
		source = strings.TrimSpace(source)
		if strings.Contains(source, "${") {
			expanded, err := expandComposeValue(source, values)
			if err != nil {
				return err
			}
			source = strings.TrimSpace(expanded)
		}
		if !isRelativeComposeBindSource(source) {
			return nil
		}
		clean := filepath.Clean(source)
		if _, exists := seen[clean]; exists {
			return nil
		}
		seen[clean] = struct{}{}
		out = append(out, clean)
		return nil
	}
	for _, rawService := range services {
		service, _ := rawService.(map[string]any)
		volumes, _ := service["volumes"].([]any)
		for _, rawVolume := range volumes {
			switch volume := rawVolume.(type) {
			case string:
				source, _, ok := splitComposeVolumeShortSyntax(volume)
				if ok {
					if err := add(source); err != nil {
						return nil, err
					}
				}
			case map[string]any:
				volumeType := strings.TrimSpace(fmt.Sprintf("%v", volume["type"]))
				if volumeType != "" && !strings.EqualFold(volumeType, "bind") {
					continue
				}
				source := strings.TrimSpace(fmt.Sprintf("%v", volume["source"]))
				if source == "" {
					source = strings.TrimSpace(fmt.Sprintf("%v", volume["src"]))
				}
				if err := add(source); err != nil {
					return nil, err
				}
			}
		}
	}
	return out, nil
}

func composeInterpolationValues(projectDir string) map[string]string {
	values := map[string]string{}
	for _, item := range os.Environ() {
		key, value, ok := strings.Cut(item, "=")
		if ok {
			values[key] = value
		}
	}
	if content, err := os.ReadFile(filepath.Join(projectDir, ".env")); err == nil {
		for key, value := range parseDotenvToMap(string(content)) {
			values[key] = value
		}
	}
	return values
}

func prepareGitComposeProjectFiles(projectDir, composeDir, composeContent string) ([]string, error) {
	projectName := filepath.Base(filepath.Clean(projectDir))
	if database.GetDB() == nil {
		return nil, nil
	}
	if _, err := database.GetComposeGitSourceInEnvironment(database.LocalEnvironmentID, projectName); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	copied := []string{}
	copyTarget := func(baseDir, target string, required bool) error {
		cleanTarget := filepath.Clean(strings.TrimSpace(target))
		if cleanTarget == "." || filepath.IsAbs(cleanTarget) {
			return fmt.Errorf("配置文件路径不安全: %s", target)
		}
		targetPath := filepath.Clean(filepath.Join(baseDir, cleanTarget))
		relativeTarget, err := filepath.Rel(projectDir, targetPath)
		if err != nil || relativeTarget == "." || relativeTarget == ".." || strings.HasPrefix(relativeTarget, ".."+string(filepath.Separator)) {
			return fmt.Errorf("配置文件路径不安全: %s", target)
		}

		example, err := copyFirstStandardExample(projectDir, relativeTarget)
		if err != nil {
			return err
		}
		if example != "" {
			copied = append(copied, fmt.Sprintf("%s → %s", example, filepath.ToSlash(relativeTarget)))
			return nil
		}
		if required {
			if _, err := os.Stat(targetPath); os.IsNotExist(err) {
				return fmt.Errorf("Git 项目缺少必需配置文件 %s，且未找到 .example/.sample/.template 模板", target)
			}
		}
		return nil
	}

	// Compose 默认读取项目根目录 .env；存在标准模板时自动复制。
	if err := copyTarget(projectDir, ".env", false); err != nil {
		return nil, err
	}

	envRefs, err := extractEnvFileRefs(composeContent)
	if err != nil {
		return nil, err
	}
	for _, ref := range envRefs {
		if err := copyTarget(composeDir, ref.Path, ref.Required); err != nil {
			return nil, err
		}
	}

	values := composeInterpolationValues(projectDir)
	bindSources, err := collectRelativeBindSources(composeContent, values)
	if err != nil {
		return nil, err
	}
	for _, source := range bindSources {
		if !looksLikeEnvFilePath(source) {
			continue
		}
		if err := copyTarget(composeDir, source, true); err != nil {
			return nil, err
		}
	}
	return copied, nil
}

func pathsReferToSameFile(a string, b string) bool {
	if filepath.Clean(a) == filepath.Clean(b) {
		return true
	}
	aInfo, aErr := os.Stat(a)
	bInfo, bErr := os.Stat(b)
	if aErr != nil || bErr != nil {
		return false
	}
	return os.SameFile(aInfo, bInfo)
}

func prepareComposeCommand(projectDir string, args []string) ([]string, []string, func(), error) {
	return prepareComposeCommandWithSource(projectDir, args, "")
}

// prepareComposeCommandWithSource applies the standard runtime bind-mount and
// host-path conversion to either the project Compose file or an explicitly
// supplied project-local Compose document.
func prepareComposeCommandWithSource(projectDir string, args []string, sourcePath string) ([]string, []string, func(), error) {
	if len(args) == 0 || args[0] != "compose" {
		return args, nil, func() {}, nil
	}
	absoluteProjectDir, err := filepath.Abs(projectDir)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("解析项目目录失败: %w", err)
	}
	projectDir = absoluteProjectDir

	composePath := strings.TrimSpace(sourcePath)
	if composePath == "" {
		composePath, err = findComposeFile(projectDir)
		if err != nil {
			return nil, nil, nil, err
		}
	} else {
		absolutePath, err := filepath.Abs(composePath)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("解析运行时 Compose 文件路径失败: %w", err)
		}
		composePath = absolutePath
	}
	relativeComposePath, err := filepath.Rel(projectDir, composePath)
	if err != nil || relativeComposePath == ".." || strings.HasPrefix(relativeComposePath, ".."+string(filepath.Separator)) {
		return nil, nil, nil, fmt.Errorf("运行时 Compose 文件不在项目目录内")
	}
	relativeComposePath = filepath.Clean(relativeComposePath)
	composeRelativeDir := filepath.Dir(relativeComposePath)
	if composeRelativeDir == "." {
		composeRelativeDir = ""
	}
	composeDir := filepath.Join(projectDir, composeRelativeDir)
	content, err := os.ReadFile(composePath)
	if err != nil {
		return nil, nil, nil, err
	}
	if _, err := prepareGitComposeProjectFiles(projectDir, composeDir, string(content)); err != nil {
		return nil, nil, nil, err
	}

	projectName := filepath.Base(filepath.Clean(projectDir))
	values := composeInterpolationValues(projectDir)
	runtimeContent, err := normalizeComposeBindMountsForRuntimeInComposeDir(string(content), projectName, composeRelativeDir, values)
	if err != nil {
		return nil, nil, nil, err
	}
	if composeHasRelativeBindMounts(runtimeContent) {
		return nil, nil, nil, fmt.Errorf("运行时 Compose 仍包含相对 bind mount，已拒绝执行")
	}

	runtimePath := composePath
	cleanup := func() {}
	if hostRoot := effectiveHostProjectRoot(); hostRoot != "" {
		runtimeDir := filepath.Join(hostRoot, projectName, composeRelativeDir)
		runtimePath = filepath.Join(hostRoot, projectName, relativeComposePath)
		cleanupRuntimeDir := false
		if pathsReferToSameFile(runtimePath, composePath) {
			runtimePath = composePath
			if runtimeContent != string(content) {
				runtimeDir = filepath.Join(hostRoot, ".tradis-runtime", projectName, composeRelativeDir)
				runtimePath = filepath.Join(hostRoot, ".tradis-runtime", projectName, relativeComposePath)
				cleanupRuntimeDir = true
			}
		}
		if filepath.Clean(runtimePath) != filepath.Clean(composePath) {
			if err := os.MkdirAll(runtimeDir, 0755); err != nil {
				return nil, nil, nil, fmt.Errorf("创建运行时 Compose 目录失败: %w", err)
			}
			runtimeFile, err := os.OpenFile(runtimePath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
			if err != nil {
				return nil, nil, nil, fmt.Errorf("创建运行时 Compose 文件失败: %w", err)
			}
			cleanup = func() {
				if cleanupRuntimeDir {
					_ = os.RemoveAll(runtimeDir)
					return
				}
				_ = os.Remove(runtimePath)
			}
			if _, err := runtimeFile.WriteString(runtimeContent); err != nil {
				_ = runtimeFile.Close()
				cleanup()
				return nil, nil, nil, err
			}
			if err := runtimeFile.Close(); err != nil {
				cleanup()
				return nil, nil, nil, err
			}
		}
	}

	prepared := []string{
		"compose",
	}
	if composeRelativeDir != "" {
		prepared = append(prepared, "--project-name", projectName)
	}
	prepared = append(prepared, "--project-directory", composeDir, "--file", runtimePath)
	hasEnvFile := false
	for _, arg := range args[1:] {
		if arg == "--env-file" {
			hasEnvFile = true
			break
		}
	}
	envPath := filepath.Join(projectDir, ".env")
	if !hasEnvFile {
		if _, err := os.Stat(envPath); err == nil {
			prepared = append(prepared, "--env-file", envPath)
		}
	}
	prepared = append(prepared, args[1:]...)

	commandEnv := []string{}
	if hostRoot := effectiveHostProjectRoot(); hostRoot != "" {
		commandEnv = append(commandEnv, "PROJECT_ROOT="+hostRoot)
	}
	return prepared, commandEnv, cleanup, nil
}

func newDockerCommand(ctx context.Context, dir string, env []string, args []string) (*exec.Cmd, func(), error) {
	return newDockerCommandWithComposeSource(ctx, dir, env, args, "")
}

func newDockerCommandWithComposeSource(ctx context.Context, dir string, env []string, args []string, composeSource string) (*exec.Cmd, func(), error) {
	preparedArgs, composeEnv, cleanup, err := prepareComposeCommandWithSource(dir, args, composeSource)
	if err != nil {
		return nil, nil, err
	}

	cmd := exec.CommandContext(ctx, "docker", preparedArgs...)
	cmd.Dir = dir
	cmd.Env = composeCommandEnvironment(dir, preparedArgs, env, composeEnv)
	return cmd, cleanup, nil
}

// composeCommandEnvironment keeps Docker CLI runtime settings while ensuring
// the managed project's dotenv wins over unrelated variables inherited from
// the TRADIS backend process. Explicit deployment values remain highest.
func composeCommandEnvironment(dir string, preparedArgs, explicitEnv, composeEnv []string) []string {
	result := append([]string(nil), os.Environ()...)
	for index := 0; index < len(preparedArgs); index++ {
		path := ""
		switch {
		case preparedArgs[index] == "--env-file" && index+1 < len(preparedArgs):
			index++
			path = preparedArgs[index]
		case strings.HasPrefix(preparedArgs[index], "--env-file="):
			path = strings.TrimPrefix(preparedArgs[index], "--env-file=")
		}
		if strings.TrimSpace(path) == "" {
			continue
		}
		if !filepath.IsAbs(path) {
			path = filepath.Join(dir, path)
		}
		raw, err := os.ReadFile(filepath.Clean(path))
		if err != nil {
			continue
		}
		values := parseDotenvToMap(string(raw))
		// Only identify names here. Compose owns quoting, comments, multiline
		// values and interpolation; injecting parsed values would override it.
		filtered := result[:0]
		for _, entry := range result {
			key, _, _ := strings.Cut(entry, "=")
			if _, declared := values[key]; !declared || !isLikelyEnvKey(key) {
				filtered = append(filtered, entry)
			}
		}
		result = filtered
	}
	result = append(result, explicitEnv...)
	result = append(result, composeEnv...)
	return result
}

func runDockerCombinedOutput(ctx context.Context, dir string, env []string, args []string) ([]byte, error) {
	cmd, cleanup, err := newDockerCommand(ctx, dir, env, args)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	output, err := cmd.CombinedOutput()
	if err != nil {
		return output, fmt.Errorf("%s", describeExecOutputError(output, err))
	}
	return output, nil
}

// describeExecError 把 exec 失败时被 Go 吞掉的 stderr 翻出来，让用户看到
// docker compose 真正的报错（而不是无意义的 "exit status N"）。
func describeExecError(err error) string {
	if err == nil {
		return ""
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		if stderr := strings.TrimSpace(string(exitErr.Stderr)); stderr != "" {
			return stderr
		}
	}
	return strings.TrimSpace(err.Error())
}

// describeExecOutputError 用于 CombinedOutput / Run 场景：命令的 stderr 已经
// 混在 output 字节里（或没被捕获），优先用 output 文本，再回退到 ExitError.Stderr。
func describeExecOutputError(output []byte, err error) string {
	if err == nil {
		return ""
	}
	if text := strings.TrimSpace(string(output)); text != "" {
		return text
	}
	return describeExecError(err)
}

func runCommandStreamLines(ctx context.Context, dir string, env []string, args []string, onLine func(string)) error {
	return runCommandStreamLinesWithComposeSource(ctx, dir, env, args, "", onLine)
}

func runCommandStreamLinesWithComposeSource(ctx context.Context, dir string, env []string, args []string, composeSource string, onLine func(string)) error {
	cmd, cleanup, err := newDockerCommandWithComposeSource(ctx, dir, env, args, composeSource)
	if err != nil {
		return err
	}
	defer cleanup()

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	pr, pw := io.Pipe()
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _ = io.Copy(pw, stdout)
	}()
	go func() {
		defer wg.Done()
		_, _ = io.Copy(pw, stderr)
	}()
	go func() {
		wg.Wait()
		_ = pw.Close()
	}()

	scanner := bufio.NewScanner(pr)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		onLine(line)
	}

	if err := cmd.Wait(); err != nil {
		return err
	}
	return nil
}

func runComposeStreamLines(ctx context.Context, projectDir string, args []string, onLine func(string)) error {
	env := []string{"COMPOSE_PROGRESS=plain", "COMPOSE_NO_COLOR=1"}
	return runCommandStreamLines(ctx, projectDir, env, args, onLine)
}

func runComposeStreamLinesWithComposeSource(ctx context.Context, projectDir string, composeSource string, args []string, onLine func(string)) error {
	env := []string{"COMPOSE_PROGRESS=plain", "COMPOSE_NO_COLOR=1"}
	return runCommandStreamLinesWithComposeSource(ctx, projectDir, env, args, composeSource, onLine)
}

// upsertDotenvKeyValue 在 dotenv 文本中更新/插入 KEY=VALUE（尽量保留原注释与格式）
func upsertDotenvKeyValue(dotenvText string, key string, val string) string {
	key = strings.TrimSpace(key)
	if !isLikelyEnvKey(key) {
		return dotenvText
	}
	lines := strings.Split(strings.ReplaceAll(dotenvText, "\r\n", "\n"), "\n")
	needle := key + "="
	for i := 0; i < len(lines); i++ {
		raw := lines[i]
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		prefix := ""
		line := trimmed
		if strings.HasPrefix(line, "export ") {
			prefix = "export "
			line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		}

		if strings.HasPrefix(line, needle) || line == key {
			lines[i] = prefix + key + "=" + val
			return strings.Join(lines, "\n")
		}
	}
	if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) != "" {
		lines = append(lines, "")
	}
	lines = append(lines, key+"="+val)
	return strings.Join(lines, "\n")
}

func composeYAMLHasServices(content []byte) bool {
	var doc map[string]any
	if err := yaml.Unmarshal(content, &doc); err != nil {
		return false
	}
	services, ok := doc["services"].(map[string]any)
	return ok && len(services) > 0
}

func isComposeYAMLFile(path string) bool {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	content, err := os.ReadFile(path)
	return err == nil && composeYAMLHasServices(content)
}

var standardComposeFilenames = []string{
	"docker-compose.yaml",
	"docker-compose.yml",
	"compose.yaml",
	"compose.yml",
}

func isStandardComposeFilename(name string) bool {
	for _, standardName := range standardComposeFilenames {
		if name == standardName {
			return true
		}
	}
	return false
}

func isImportedComposeYAMLFile(projectDir, candidate string) bool {
	relativePath, err := filepath.Rel(projectDir, candidate)
	if err != nil || relativePath == "." || relativePath == ".." || strings.HasPrefix(relativePath, ".."+string(filepath.Separator)) {
		return false
	}

	parentDir := projectDir
	for _, component := range strings.Split(filepath.Dir(relativePath), string(filepath.Separator)) {
		if component == "." || component == "" {
			continue
		}
		parentDir = filepath.Join(parentDir, component)
		info, err := os.Lstat(parentDir)
		if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return false
		}
	}

	return isComposeYAMLFile(candidate)
}

func findImportedComposeFile(projectDir, preferredRelativePath string) (string, error) {
	projectDir, err := filepath.Abs(projectDir)
	if err != nil {
		return "", err
	}

	if preferred := strings.TrimSpace(preferredRelativePath); preferred != "" {
		preferredPath := filepath.Clean(filepath.FromSlash(preferred))
		if !filepath.IsAbs(preferredPath) && preferredPath != "." && preferredPath != ".." && !strings.HasPrefix(preferredPath, ".."+string(filepath.Separator)) {
			parts := strings.Split(preferredPath, string(filepath.Separator))
			if len(parts) <= 3 && isStandardComposeFilename(filepath.Base(preferredPath)) {
				candidate := filepath.Join(projectDir, preferredPath)
				if isImportedComposeYAMLFile(projectDir, candidate) {
					return candidate, nil
				}
			}
		}
	}

	findInDirectory := func(dir string) string {
		for _, name := range standardComposeFilenames {
			candidate := filepath.Join(dir, name)
			if isImportedComposeYAMLFile(projectDir, candidate) {
				return candidate
			}
		}
		return ""
	}
	if candidate := findInDirectory(projectDir); candidate != "" {
		return candidate, nil
	}

	entries, err := os.ReadDir(projectDir)
	if err != nil {
		return "", err
	}
	firstLevelDirs := make([]string, 0)
	for _, entry := range entries {
		if entry.IsDir() {
			firstLevelDirs = append(firstLevelDirs, entry.Name())
		}
	}

	orderedFirstLevelDirs := make([]string, 0, len(firstLevelDirs))
	for _, preferredDir := range []string{"docker", "deploy"} {
		for _, dir := range firstLevelDirs {
			if dir == preferredDir {
				orderedFirstLevelDirs = append(orderedFirstLevelDirs, dir)
				break
			}
		}
	}
	for _, dir := range firstLevelDirs {
		if dir != "docker" && dir != "deploy" {
			orderedFirstLevelDirs = append(orderedFirstLevelDirs, dir)
		}
	}
	for _, dir := range orderedFirstLevelDirs {
		if candidate := findInDirectory(filepath.Join(projectDir, dir)); candidate != "" {
			return candidate, nil
		}
	}

	secondLevelDirs := make([]string, 0)
	for _, firstLevelDir := range firstLevelDirs {
		entries, err := os.ReadDir(filepath.Join(projectDir, firstLevelDir))
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() {
				secondLevelDirs = append(secondLevelDirs, filepath.Join(firstLevelDir, entry.Name()))
			}
		}
	}
	sort.Strings(secondLevelDirs)
	for _, dir := range secondLevelDirs {
		if candidate := findInDirectory(filepath.Join(projectDir, dir)); candidate != "" {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("%w: %w", errComposeFileNotFound, os.ErrNotExist)
}

// findComposeFile 只返回根节点包含非空 services 映射的 Compose YAML。
// 标准文件名优先；非标准 *.yaml/*.yml 也必须通过相同语义校验。
func findComposeFile(projectDir string) (string, error) {
	if database.GetDB() != nil {
		projectName := filepath.Base(filepath.Clean(projectDir))
		source, err := composeGitSourceLookup(database.LocalEnvironmentID, projectName)
		switch {
		case err == nil:
			path, discoveryErr := findImportedComposeFile(projectDir, source.ComposePath)
			if discoveryErr != nil {
				return "", fmt.Errorf("Git Compose 有界发现失败: %w", discoveryErr)
			}
			return path, nil
		case errors.Is(err, sql.ErrNoRows):
			// Manual and legacy projects retain root-level custom YAML fallback.
		default:
			return "", fmt.Errorf("读取 Git Compose 来源失败: %w", err)
		}
	}

	standard := make(map[string]struct{}, len(standardComposeFilenames))
	for _, name := range standardComposeFilenames {
		standard[name] = struct{}{}
		path := filepath.Join(projectDir, name)
		if isComposeYAMLFile(path) {
			return path, nil
		}
	}

	entries, err := os.ReadDir(projectDir)
	if err != nil {
		return "", err
	}

	matched := make([]string, 0)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if _, ok := standard[name]; ok {
			continue
		}
		lower := strings.ToLower(name)
		if !strings.HasSuffix(lower, ".yaml") && !strings.HasSuffix(lower, ".yml") {
			continue
		}
		path := filepath.Join(projectDir, name)
		if isComposeYAMLFile(path) {
			matched = append(matched, name)
		}
	}

	if len(matched) == 0 {
		return "", os.ErrNotExist
	}
	sort.Strings(matched)
	return filepath.Join(projectDir, matched[0]), nil
}

// RegisterComposeRoutes 注册路由
func RegisterComposeRoutes(r *gin.RouterGroup) {
	group := r.Group("/compose")
	{
		group.GET("/list", listProjects)
		group.GET("/projects", listProjects)
		group.POST("/deploy-task", deployComposeTask)
		group.POST("/deploy", deployComposeTask)
		group.POST("/import-git", importComposeFromGit)
		group.POST("/:name/sync-git", syncComposeFromGit)
		group.GET("/tasks", listComposeTasks)
		group.GET("/tasks/:id", getComposeTask)
		group.POST("/tasks/:id/cancel", cancelComposeTask)
		group.GET("/tasks/:id/events", composeTaskEvents)
		group.POST("/projects/:name/start", startProject)
		group.POST("/:name/start", startProject)
		group.GET("/:name/start/events", startProjectEvents)
		group.POST("/projects/:name/start/tasks", startProjectTask)
		group.POST("/:name/start/tasks", startProjectTask)
		group.POST("/projects/:name/stop", stopProject)
		group.POST("/:name/stop", stopProject)
		group.GET("/:name/stop/events", stopProjectEvents)
		group.POST("/projects/:name/stop/tasks", stopProjectTask)
		group.POST("/:name/stop/tasks", stopProjectTask)
		group.POST("/projects/:name/kill/tasks", killProjectTask)
		group.POST("/:name/kill/tasks", killProjectTask)
		group.POST("/projects/:name/restart", restartProject)
		group.POST("/:name/restart", restartProject) // 添加重启路由
		group.GET("/:name/restart/events", restartProjectEvents)
		group.POST("/projects/:name/restart/tasks", restartProjectTask)
		group.POST("/:name/restart/tasks", restartProjectTask)
		group.POST("/projects/:name/down/tasks", downProjectTask)
		group.POST("/:name/down/tasks", downProjectTask)
		group.POST("/projects/:name/remove/tasks", removeProjectTask)
		group.POST("/:name/remove/tasks", removeProjectTask)
		group.GET("/projects/:name/destroy-preview", previewComposeDestroy)
		group.GET("/:name/destroy-preview", previewComposeDestroy)
		group.POST("/projects/:name/destroy/tasks", destroyProjectTask)
		group.POST("/:name/destroy/tasks", destroyProjectTask)
		group.GET("/projects/:name/update/events", updateProjectEvents)
		group.GET("/:name/update/events", updateProjectEvents) // 添加 SSE 更新路由
		group.POST("/projects/:name/update/tasks", updateProjectTask)
		group.POST("/:name/update/tasks", updateProjectTask)
		group.POST("/projects/:name/build", buildProject)
		group.POST("/:name/build", buildProject)             // 保留 POST 构建路由用于兼容
		group.GET("/:name/build/events", buildProjectEvents) // 添加 SSE 构建路由
		group.POST("/projects/:name/build/tasks", buildProjectTask)
		group.POST("/:name/build/tasks", buildProjectTask)
		group.GET("/projects/:name/status", getStackStatus)
		group.GET("/:name/status", getStackStatus)
		group.PUT("/projects/:name/metadata", updateComposeProjectMetadata)
		group.GET("/container-remarks", listContainerRemarks)
		group.PUT("/containers/:name/remark", updateContainerRemark)
		group.POST("/projects/:name/down", downProject)
		group.DELETE("/projects/:name/down", downProject)
		group.DELETE("/:name/down", downProject) // 添加清除(down)路由
		group.DELETE("/projects/:name", removeProject)
		group.DELETE("/remove/:name", removeProject) // 修改为匹配当前请求格式
		group.GET("/:name/logs", getComposeLogs)     // 确保这个路由已添加
		group.GET("/projects/:name/yaml", getProjectYaml)
		group.GET("/:name/yaml", getProjectYaml) // 添加获取 YAML 路由
		group.GET("/projects/:name/env", getProjectEnv)
		group.GET("/:name/env", getProjectEnv) // 添加获取 .env 路由
		group.POST("/projects/:name/yaml", saveProjectYaml)
		group.POST("/:name/yaml", saveProjectYaml) // 添加保存 YAML 路由
		group.POST("/projects/:name/env", saveProjectEnv)
		group.POST("/:name/env", saveProjectEnv) // 添加保存 .env 路由
		group.POST("/:name/config/preview", previewComposeConfig)
		group.POST("/:name/config/apply", applyComposeConfig)
		group.GET("/:name/config/history", listComposeConfigHistory)
		group.POST("/:name/config/history/:historyId/preview", previewComposeHistoryRestore)
		group.POST("/:name/config/history/:historyId/restore", restoreComposeHistory)
	}
	registerEditionComposeRoutes(group)
}

type composeDeployRequest struct {
	Name           string `json:"name"`
	Compose        string `json:"compose"`
	Dotenv         string `json:"dotenv"`
	Env            string `json:"env"`
	AutoStart      *bool  `json:"autoStart"`
	Pull           bool   `json:"pull"`
	Rebuild        bool   `json:"rebuild"`
	Replace        bool   `json:"replace"`
	PreflightToken string `json:"preflightToken"`
	IdempotencyKey string `json:"idempotencyKey"`
}

type composeGitImportRequest struct {
	RepoURL        string `json:"repoUrl"`
	Name           string `json:"name"`
	Branch         string `json:"branch"`
	AcceleratorURL string `json:"acceleratorUrl"`
	Overwrite      bool   `json:"overwrite"`
}

func normalizePublicGitHubRepoURL(raw string) (string, string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", "", fmt.Errorf("仓库地址无效: %w", err)
	}
	if parsed.Scheme != "https" || !strings.EqualFold(parsed.Hostname(), "github.com") {
		return "", "", fmt.Errorf("目前仅支持公开 GitHub HTTPS 仓库")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Port() != "" {
		return "", "", fmt.Errorf("仓库地址不能包含认证信息、端口、查询参数或片段")
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return "", "", fmt.Errorf("仓库地址格式应为 https://github.com/owner/repository")
	}
	ownerPattern := regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,38})$`)
	repoPattern := regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
	repo := strings.TrimSuffix(parts[1], ".git")
	if !ownerPattern.MatchString(parts[0]) || !repoPattern.MatchString(repo) || repo == "" {
		return "", "", fmt.Errorf("GitHub 仓库路径不合法")
	}
	return "https://github.com/" + parts[0] + "/" + repo + ".git", repo, nil
}

func validateGitRef(ref string) bool {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return true
	}
	if len(ref) > 200 || strings.HasPrefix(ref, "-") || strings.Contains(ref, "..") || strings.ContainsAny(ref, " ~^:?*[\\") {
		return false
	}
	return true
}

func normalizeGitAcceleratorURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return "", fmt.Errorf("Git 加速地址必须是有效的 HTTPS URL")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("Git 加速地址不能包含认证信息、查询参数或片段")
	}
	host := strings.ToLower(strings.TrimSpace(parsed.Hostname()))
	if host == "" || host == "localhost" || strings.HasSuffix(host, ".local") {
		return "", fmt.Errorf("Git 加速地址不能指向本机或局域网")
	}
	if ip := net.ParseIP(host); ip != nil {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() {
			return "", fmt.Errorf("Git 加速地址不能指向内网 IP")
		}
	}
	return strings.TrimRight(raw, "/"), nil
}

func buildAcceleratedGitURL(repoURL, acceleratorURL string) (string, error) {
	acceleratorURL, err := normalizeGitAcceleratorURL(acceleratorURL)
	if err != nil || acceleratorURL == "" {
		return repoURL, err
	}
	result := ""
	if strings.Contains(acceleratorURL, "{url}") {
		result = strings.ReplaceAll(acceleratorURL, "{url}", repoURL)
	} else {
		result = acceleratorURL + "/" + repoURL
	}
	parsed, err := url.Parse(result)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
		return "", fmt.Errorf("拼接后的 Git 加速地址无效")
	}
	addresses, err := net.LookupIP(parsed.Hostname())
	if err != nil {
		return "", fmt.Errorf("无法解析 Git 加速地址: %w", err)
	}
	for _, ip := range addresses {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() {
			return "", fmt.Errorf("Git 加速地址解析到了内网 IP")
		}
	}
	return result, nil
}

func findRootComposeFile(projectDir string) (string, error) {
	for _, name := range []string{"docker-compose.yaml", "docker-compose.yml", "compose.yaml", "compose.yml"} {
		path := filepath.Join(projectDir, name)
		info, err := os.Lstat(path)
		if err != nil {
			continue
		}
		if info.Mode().IsRegular() {
			return path, nil
		}
		return "", fmt.Errorf("Compose 文件不能是符号链接或特殊文件: %s", name)
	}
	return "", fmt.Errorf("仓库根目录未找到 docker-compose.yml、docker-compose.yaml、compose.yml 或 compose.yaml")
}

// dirHasComposeFile reports whether a directory holds a recognizable Compose
// file, used to surface unmanageable (invalid-name) project directories.
func dirHasComposeFile(dir string) bool {
	_, err := findComposeFile(dir)
	return err == nil
}

func importComposeFromGit(c *gin.Context) {
	var req composeGitImportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "无效的请求参数", err)
		return
	}

	repoURL, repoName, err := normalizePublicGitHubRepoURL(req.RepoURL)
	if err != nil {
		respondError(c, http.StatusBadRequest, err.Error(), nil)
		return
	}
	if !validateGitRef(req.Branch) {
		respondError(c, http.StatusBadRequest, "分支或标签名称不合法", nil)
		return
	}
	acceleratorURL, err := normalizeGitAcceleratorURL(req.AcceleratorURL)
	if err != nil {
		respondError(c, http.StatusBadRequest, err.Error(), nil)
		return
	}

	projectNameRaw := strings.TrimSpace(req.Name)
	if projectNameRaw == "" {
		projectNameRaw = strings.ToLower(repoName)
	}
	projectName, ok := validateComposeProjectName(projectNameRaw)
	if !ok {
		respondError(c, http.StatusBadRequest, "项目名不合法：仅支持小写字母/数字，且可包含 _ -，并以字母或数字开头", nil)
		return
	}
	if forbidIfSelfProject(c, projectName) {
		return
	}
	if req.Overwrite {
		containerCount, checkErr := composeProjectContainerCount(c.Request.Context(), projectName)
		if checkErr != nil {
			respondError(c, http.StatusInternalServerError, "检查项目容器失败", checkErr)
			return
		}
		if containerCount > 0 {
			respondError(c, http.StatusConflict, "覆盖前请先执行“清除容器”，避免容器继续引用旧项目目录", nil)
			return
		}
	}

	taskID := fmt.Sprintf("%d", time.Now().UnixNano())
	_ = database.UpsertTask(taskID, "compose_git_import", "pending")
	go runComposeGitSyncTask(taskID, "compose_git_import", projectName, repoURL, strings.TrimSpace(req.Branch), acceleratorURL, req.Overwrite)

	c.JSON(http.StatusAccepted, gin.H{
		"message": "Git 下载任务已提交",
		"taskId":  taskID,
		"name":    projectName,
	})
}

func syncComposeFromGit(c *gin.Context) {
	projectName, ok := validateComposeProjectName(c.Param("name"))
	if !ok {
		respondError(c, http.StatusBadRequest, "项目名不合法", nil)
		return
	}
	if forbidIfSelfProject(c, projectName) {
		return
	}
	containerCount, err := composeProjectContainerCount(c.Request.Context(), projectName)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "检查项目容器失败", err)
		return
	}
	if containerCount > 0 {
		respondError(c, http.StatusConflict, "同步前请先执行“清除容器”，避免运行中的挂载继续引用旧项目目录", nil)
		return
	}
	source, err := database.GetComposeGitSourceInEnvironment(database.LocalEnvironmentID, projectName)
	if err != nil {
		if err == sql.ErrNoRows {
			respondError(c, http.StatusNotFound, "该项目没有已保存的 Git 来源", nil)
			return
		}
		respondError(c, http.StatusInternalServerError, "读取 Git 来源失败", err)
		return
	}
	taskID := fmt.Sprintf("%d", time.Now().UnixNano())
	_ = database.UpsertTask(taskID, "compose_git_sync", "pending")
	go runComposeGitSyncTask(
		taskID,
		"compose_git_sync",
		projectName,
		source.RepoURL,
		source.Branch,
		source.AcceleratorURL,
		true,
	)
	c.JSON(http.StatusAccepted, gin.H{
		"message": "Git 同步任务已提交",
		"taskId":  taskID,
		"name":    projectName,
	})
}

func composeProjectContainerCount(ctx context.Context, projectName string) (int, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return 0, fmt.Errorf("Docker 客户端初始化失败: %w", err)
	}
	defer cli.Close()
	containers, err := cli.ContainerList(ctx, types.ContainerListOptions{
		All: true,
		Filters: filters.NewArgs(
			filters.Arg("label", "com.docker.compose.project="+projectName),
		),
	})
	if err != nil {
		return 0, err
	}
	return len(containers), nil
}

func runComposeGitSyncTask(taskID, taskType, projectName, repoURL, branch, acceleratorURL string, replaceExisting bool) {
	defer lockComposeProjectMutation(projectName)()

	seq := int64(0)
	done := false
	appendLog := func(logType, message string) {
		seq++
		_ = database.AppendTaskLogWithSeq(taskID, seq, time.Now(), logType, message)
	}
	finish := func(status string, result any, errText string) {
		if done {
			return
		}
		done = true
		invalidateComposeProjectListCache()
		_ = database.FinishTask(taskID, status, result, errText)
		success := status == "success" || status == "completed"
		if success {
			return
		}
		action := "导入"
		if replaceExisting {
			action = "同步"
		}
		if strings.TrimSpace(errText) == "" {
			errText = "未知错误"
		}
		message := fmt.Sprintf("Compose 项目 %s Git %s失败：%s", projectName, action, errText)
		_ = database.SaveNotification(&database.Notification{
			Type:     "error",
			Category: "git_task",
			Message:  message,
			Read:     false,
		})
	}

	_ = database.UpsertTask(taskID, taskType, "running")
	appendLog("info", "开始下载 Git 仓库")

	projectRoot := getProjectsBaseDir()
	if err := os.MkdirAll(projectRoot, 0755); err != nil {
		appendLog("error", "创建项目根目录失败: "+err.Error())
		finish("error", nil, err.Error())
		return
	}
	projectDir, err := resolveProjectDir(projectRoot, projectName)
	if err != nil {
		appendLog("error", err.Error())
		finish("error", nil, err.Error())
		return
	}
	_, statErr := os.Stat(projectDir)
	if !replaceExisting && statErr == nil {
		errText := fmt.Sprintf("项目 '%s' 已存在", projectName)
		appendLog("error", errText)
		finish("error", nil, errText)
		return
	} else if statErr != nil && !os.IsNotExist(statErr) {
		appendLog("error", "检查项目目录失败: "+statErr.Error())
		finish("error", nil, statErr.Error())
		return
	}
	if replaceExisting && os.IsNotExist(statErr) {
		errText := fmt.Sprintf("项目 '%s' 目录不存在", projectName)
		appendLog("error", errText)
		finish("error", nil, errText)
		return
	}

	tempRoot, err := os.MkdirTemp(projectRoot, "."+projectName+"-git-")
	if err != nil {
		appendLog("error", "创建临时目录失败: "+err.Error())
		finish("error", nil, err.Error())
		return
	}
	defer os.RemoveAll(tempRoot)
	downloadDir := filepath.Join(tempRoot, "source")

	commitHash, err := downloadPublicGitHubRepository(
		repoURL,
		branch,
		acceleratorURL,
		downloadDir,
		appendLog,
	)
	if err != nil {
		errText := "下载 Git 仓库失败: " + err.Error()
		appendLog("error", errText)
		finish("error", nil, errText)
		return
	}
	appendLog("success", "Git 仓库下载完成")

	preferredComposePath := ""
	if replaceExisting {
		if source, err := database.GetComposeGitSourceInEnvironment(database.LocalEnvironmentID, projectName); err == nil {
			preferredComposePath = source.ComposePath
		}
	}
	composePath, err := findImportedComposeFile(downloadDir, preferredComposePath)
	if err != nil {
		appendLog("error", err.Error())
		finish("error", nil, err.Error())
		return
	}
	composeRelativePath, err := filepath.Rel(downloadDir, composePath)
	if err != nil {
		appendLog("error", "解析 Compose 路径失败: "+err.Error())
		finish("error", nil, err.Error())
		return
	}
	composeRelativePath = filepath.ToSlash(composeRelativePath)
	appendLog("info", "已发现 Compose 文件: "+composeRelativePath)
	composeBytes, err := os.ReadFile(composePath)
	if err != nil {
		appendLog("error", "读取 Compose 文件失败: "+err.Error())
		finish("error", nil, err.Error())
		return
	}
	composeContent := string(composeBytes)
	if !composeYAMLHasServices(composeBytes) {
		errText := "Compose 文件无效，根节点必须包含非空 services 映射"
		appendLog("error", errText)
		finish("error", nil, errText)
		return
	}

	composeDir := filepath.Dir(composePath)
	if pathErrs := validateGitImportedComposeAssetPaths(downloadDir, composeDir, composeContent); len(pathErrs) > 0 {
		errText := "Compose 路径校验失败: " + strings.Join(pathErrs, "; ")
		appendLog("error", errText)
		finish("error", nil, errText)
		return
	}

	if replaceExisting {
		oldEnv := filepath.Join(projectDir, ".env")
		newEnv := filepath.Join(downloadDir, ".env")
		if _, err := os.Stat(newEnv); os.IsNotExist(err) {
			if content, readErr := os.ReadFile(oldEnv); readErr == nil {
				if writeErr := os.WriteFile(newEnv, content, 0600); writeErr != nil {
					appendLog("error", "保留本地 .env 失败: "+writeErr.Error())
					finish("error", nil, writeErr.Error())
					return
				}
				appendLog("info", "已保留项目原有 .env")
			}
		}
	}
	_ = os.RemoveAll(filepath.Join(downloadDir, ".git"))

	backupDir := projectDir + ".git-backup-" + taskID
	if replaceExisting {
		if err := composeGitRename(projectDir, backupDir); err != nil {
			appendLog("error", "备份现有项目失败: "+err.Error())
			finish("error", nil, err.Error())
			return
		}
	}
	if err := composeGitRename(downloadDir, projectDir); err != nil {
		if replaceExisting {
			if restoreErr := composeGitRename(backupDir, projectDir); restoreErr != nil {
				errText := fmt.Sprintf("替换项目目录失败: %v; 恢复原项目失败: %v; 原项目备份保留在 %s，请手动恢复", err, restoreErr, backupDir)
				appendLog("error", errText)
				finish("error", nil, errText)
				return
			}
		}
		appendLog("error", "替换项目目录失败: "+err.Error())
		finish("error", nil, err.Error())
		return
	}
	if err := composeGitSourceUpsert(database.LocalEnvironmentID, database.ComposeGitSource{
		ProjectName:    projectName,
		RepoURL:        repoURL,
		Branch:         branch,
		AcceleratorURL: acceleratorURL,
		CommitHash:     commitHash,
		ComposePath:    composeRelativePath,
	}); err != nil {
		errText := "保存 Git 来源失败: " + err.Error()
		if rollbackErr := rollbackComposeGitProjectReplacement(projectDir, backupDir, replaceExisting); rollbackErr != nil {
			errText += "; " + rollbackErr.Error()
		}
		appendLog("error", errText)
		finish("error", nil, errText)
		return
	}
	if replaceExisting {
		_ = composeGitRemoveAll(backupDir)
	}

	appendLog("success", "Git 仓库已同步到项目目录，请确认 "+composeRelativePath+" 和配置后手动部署")
	finish("success", gin.H{"project": projectName, "commit": commitHash, "composePath": composeRelativePath}, "")
}

func rollbackComposeGitProjectReplacement(projectDir, backupDir string, restoreBackup bool) error {
	if err := composeGitRemoveAll(projectDir); err != nil {
		if restoreBackup {
			return fmt.Errorf("删除新项目目录失败: %v; 原项目备份保留在 %s，请手动恢复", err, backupDir)
		}
		return fmt.Errorf("删除新项目目录失败: %w", err)
	}
	if !restoreBackup {
		return nil
	}
	if err := composeGitRename(backupDir, projectDir); err != nil {
		return fmt.Errorf("恢复原项目失败: %v; 原项目备份保留在 %s，请手动恢复", err, backupDir)
	}
	return nil
}

func deployComposeTask(c *gin.Context) {
	var req composeDeployRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "无效的请求参数", err)
		return
	}

	projectName, candidate, ok := composeDeploymentCandidate(c, req)
	if !ok {
		return
	}
	if handleEditionComposeDeployment(c, req, projectName, candidate) {
		return
	}
	if forbidIfSelfProject(c, projectName) {
		return
	}

	autoStart := true
	if req.AutoStart != nil {
		autoStart = *req.AutoStart
	}

	taskID := fmt.Sprintf("%d", time.Now().UnixNano())
	_ = database.UpsertTask(taskID, "compose_deploy", "pending")

	ctx, cancel := context.WithCancel(context.Background())
	registerComposeTaskCancel(taskID, cancel)
	go func() {
		defer unregisterComposeTaskCancel(taskID)
		runComposeDeployTaskWithTypeContext(ctx, taskID, "compose_deploy", projectName, req.Compose, req.Dotenv, req.Env, autoStart, composeOperationOptions{
			Pull:    req.Pull,
			Rebuild: req.Rebuild,
		})
	}()

	c.JSON(http.StatusOK, gin.H{
		"message": "部署任务已提交",
		"taskId":  taskID,
	})
}

func composeDeploymentCandidate(c *gin.Context, req composeDeployRequest) (string, deployment.DeploymentCandidate, bool) {
	projectNameRaw := strings.TrimSpace(req.Name)
	if projectNameRaw == "" || strings.TrimSpace(req.Compose) == "" {
		respondError(c, http.StatusBadRequest, "项目名称和配置内容不能为空", nil)
		return "", deployment.DeploymentCandidate{}, false
	}
	if !composeYAMLHasServices([]byte(req.Compose)) {
		respondError(c, http.StatusBadRequest, "Compose YAML 根节点必须包含非空 services 映射", nil)
		return "", deployment.DeploymentCandidate{}, false
	}
	projectName, valid := validateComposeProjectName(projectNameRaw)
	if !valid {
		respondError(c, http.StatusBadRequest, "项目名不合法：仅支持小写字母/数字，且可包含 _ -，并以字母或数字开头", nil)
		return "", deployment.DeploymentCandidate{}, false
	}
	autoStart := true
	if req.AutoStart != nil {
		autoStart = *req.AutoStart
	}
	dotenv := strings.ReplaceAll(req.Dotenv, "\r\n", "\n")
	if strings.TrimSpace(req.Env) != "" {
		values := map[string]string{}
		if json.Unmarshal([]byte(req.Env), &values) == nil {
			for key, value := range values {
				dotenv = upsertDotenvKeyValue(dotenv, key, value)
			}
		}
	}
	dotenv = filterDotenvByAllowedKeys(dotenv, extractComposeInterpolationKeys(req.Compose))
	candidate := deployment.DeploymentCandidate{
		Version: deployment.DeploymentCandidateVersion, EnvironmentID: database.LocalEnvironmentID, ProjectName: projectName,
		SourceType: deployment.DeploymentSourceManualCompose, ComposeYAML: req.Compose, Dotenv: dotenv,
		Options:        deployment.DeploymentOptions{AutoStart: autoStart, Pull: req.Pull, Rebuild: req.Rebuild, Replace: req.Replace},
		IdempotencyKey: strings.TrimSpace(req.IdempotencyKey),
	}
	return projectName, candidate, true
}

func runComposeDeployTask(taskID string, projectName string, compose string, dotenvRaw string, envRaw string, autoStart bool, options composeOperationOptions) {
	runComposeDeployTaskWithType(taskID, "compose_deploy", projectName, compose, dotenvRaw, envRaw, autoStart, options)
}

func runComposeDeployTaskWithType(taskID string, taskType string, projectName string, compose string, dotenvRaw string, envRaw string, autoStart bool, options composeOperationOptions) {
	runComposeDeployTaskWithTypeContext(context.Background(), taskID, taskType, projectName, compose, dotenvRaw, envRaw, autoStart, options)
}

func runComposeDeployTaskWithTypeContext(ctx context.Context, taskID string, taskType string, projectName string, compose string, dotenvRaw string, envRaw string, autoStart bool, options composeOperationOptions) {
	runComposeDeployTaskWithTypeAfterSeqContext(ctx, taskID, taskType, projectName, compose, dotenvRaw, envRaw, autoStart, options, 0)
}

func runComposeDeployTaskWithTypeAfterSeq(taskID string, taskType string, projectName string, compose string, dotenvRaw string, envRaw string, autoStart bool, options composeOperationOptions, initialSeq int64) {
	runComposeDeployTaskWithTypeAfterSeqContext(context.Background(), taskID, taskType, projectName, compose, dotenvRaw, envRaw, autoStart, options, initialSeq)
}

func runComposeDeployTaskWithTypeAfterSeqContext(ctx context.Context, taskID string, taskType string, projectName string, compose string, dotenvRaw string, envRaw string, autoStart bool, options composeOperationOptions, initialSeq int64) {
	if ctx == nil {
		ctx = context.Background()
	}
	if runLocalComposeDeploymentWithService(ctx, taskID, taskType, projectName, compose, dotenvRaw, envRaw, autoStart, options, initialSeq) {
		return
	}
	unlockProject := lockComposeProjectMutation(projectName)
	defer unlockProject()
	taskType = strings.TrimSpace(taskType)
	if taskType == "" {
		taskType = "compose_deploy"
	}
	editionHooks := composeDeployEditionHooksForTask(taskType)
	seq := initialSeq
	appendLog := func(logType string, message string) {
		seq++
		_ = database.AppendTaskLogWithSeq(taskID, seq, time.Now(), logType, message)
	}

	done := false
	finish := func(status string, result any, errStr string) {
		if done {
			return
		}
		done = true
		invalidateComposeProjectListCache()
		_ = database.FinishTask(taskID, status, result, errStr)

		if editionHooks.notifyCompletion {
			st := strings.ToLower(strings.TrimSpace(status))
			notifyType := "info"
			notifyMsg := fmt.Sprintf("Compose 项目 %s 部署任务结束", projectName)
			if st == "success" || st == "completed" {
				notifyType = "success"
				notifyMsg = fmt.Sprintf("Compose 项目 %s 部署成功", projectName)
			} else {
				notifyType = "error"
				errText := strings.TrimSpace(errStr)
				if errText == "" {
					errText = "未知错误"
				}
				notifyMsg = fmt.Sprintf("Compose 项目 %s 部署失败：%s", projectName, errText)
			}
			_ = database.SaveNotification(&database.Notification{
				Type:     notifyType,
				Category: "deploy_task",
				Message:  notifyMsg,
				Read:     false,
			})
		}
	}
	cancelIfRequested := func() bool {
		if ctx.Err() == nil {
			return false
		}
		appendLog("warning", "部署已取消")
		finish("canceled", nil, "部署已取消")
		return true
	}
	finishCommandError := func(prefix string, err error) {
		if ctx.Err() != nil {
			cancelIfRequested()
			return
		}
		appendLog("error", prefix+err.Error())
		finish("error", nil, err.Error())
	}

	_ = database.UpsertTask(taskID, taskType, "running")
	appendLog("info", fmt.Sprintf("开始部署项目：%s", projectName))
	if cancelIfRequested() {
		return
	}

	projectDir, err := resolveProjectDir(getProjectsBaseDir(), projectName)
	if err != nil {
		appendLog("error", "项目目录校验失败: "+err.Error())
		finish("error", nil, err.Error())
		return
	}
	composePath := filepath.Join(projectDir, "docker-compose.yml")
	envPath := filepath.Join(projectDir, ".env")

	// 容器内目录必然映射到宿主机 PROJECT_ROOT；向用户明确展示两端路径
	hostProjectDir := ""
	if hostRoot := effectiveHostProjectRoot(); hostRoot != "" {
		hostProjectDir = filepath.Join(hostRoot, projectName)
	}
	appendLog("info", fmt.Sprintf("项目目录（容器内）: %s", projectDir))
	if hostProjectDir != "" {
		appendLog("info", fmt.Sprintf("对应宿主机目录（PROJECT_ROOT）: %s", hostProjectDir))
	} else {
		appendLog("warning", "未探测到 PROJECT_ROOT，宿主机映射路径未知；请确认已正确设置并挂载 PROJECT_ROOT")
	}

	projectDirCreated := false
	if _, err := os.Stat(projectDir); err == nil {
		_, findErr := findComposeFile(projectDir)
		switch {
		case findErr == nil && !options.Replace:
			appendLog("error", fmt.Sprintf("项目 '%s' 已存在，如需重新部署请先删除现有项目", projectName))
			finish("error", nil, "project exists")
			return
		case findErr == nil:
			appendLog("info", fmt.Sprintf("项目 '%s' 已存在，将保留项目目录并应用新的 Compose 配置", projectName))
		case errors.Is(findErr, os.ErrNotExist):
			// 目录已存在但明确没有 Compose 文件（空目录/手工复制杂项）：
			// 采纳该目录写入本次 YAML 继续部署；其他错误（目录不可读、
			// Git 来源查询失败等）一律按检查失败终止，不盲目采纳。
			appendLog("info", fmt.Sprintf("目录 '%s' 已存在但不含 Compose 文件，将在其中创建并继续部署", projectName))
		default:
			appendLog("error", "检查项目 Compose 文件失败: "+findErr.Error())
			finish("error", nil, findErr.Error())
			return
		}
	} else if !os.IsNotExist(err) {
		appendLog("error", "检查项目目录失败: "+err.Error())
		finish("error", nil, err.Error())
		return
	} else {
		projectDirCreated = true
	}

	if err := os.MkdirAll(projectDir, 0755); err != nil {
		appendLog("error", "创建项目目录失败: "+err.Error())
		finish("error", nil, err.Error())
		return
	}
	finalizeProjectDir, err := editionHooks.prepareProjectDir(projectDir, projectDirCreated)
	if err != nil {
		appendLog("error", "应用 NAS 默认项目属主失败: "+err.Error())
		finish("error", nil, err.Error())
		return
	}

	composeToWrite := compose

	if err := editionHooks.writeProjectFile(composePath, []byte(composeToWrite), 0644); err != nil {
		appendLog("error", "保存配置文件失败: "+err.Error())
		finish("error", nil, err.Error())
		return
	}

	envMap := make(map[string]string)
	if strings.TrimSpace(envRaw) != "" {
		if err := json.Unmarshal([]byte(envRaw), &envMap); err != nil {
			appendLog("warning", "解析 env 参数失败，将仅使用 dotenv: "+err.Error())
			envMap = make(map[string]string)
		}
	}

	dotenvText := strings.ReplaceAll(dotenvRaw, "\r\n", "\n")
	if strings.TrimSpace(dotenvText) == "" && len(envMap) > 0 {
		dotenvText = renderDotenvFromMap(envMap)
	} else if len(envMap) > 0 {
		for k, v := range envMap {
			dotenvText = upsertDotenvKeyValue(dotenvText, k, v)
		}
	}

	dotenvText, err = editionHooks.filterDotenv(composeToWrite, dotenvText, projectDir, hostProjectDir)
	if err != nil {
		appendLog("error", err.Error())
		finish("error", nil, err.Error())
		return
	}

	if err := editionHooks.writeProjectFile(envPath, []byte(dotenvText), 0600); err != nil {
		appendLog("error", "保存 .env 文件失败: "+err.Error())
		finish("error", nil, err.Error())
		return
	}

	if len(options.ExtraFiles) > 0 {
		written, err := writeComposeExtraFiles(projectDir, options.ExtraFiles)
		if err != nil {
			appendLog("error", "保存额外配置文件失败: "+err.Error())
			finish("error", nil, err.Error())
			return
		}
		for _, item := range written {
			appendLog("info", "已写入额外配置文件: "+item)
		}
	}
	if err := finalizeProjectDir(); err != nil {
		appendLog("error", "保存配置后调整 NAS 默认项目属主失败: "+err.Error())
		finish("error", nil, err.Error())
		return
	}

	appendLog("success", "配置已保存")
	if !autoStart {
		finish("success", gin.H{"project": projectName, "autoStart": false}, "")
		return
	}
	if cancelIfRequested() {
		return
	}
	streamLog := func(line string) {
		msgType := "info"
		if strings.Contains(line, "error") || strings.Contains(line, "Error") {
			msgType = "error"
		} else if strings.Contains(line, "Created") || strings.Contains(line, "Started") || strings.Contains(line, "Built") {
			msgType = "success"
		}
		appendLog(msgType, line)
	}
	commandPlan := buildComposeDeployCommandPlan(options)
	if len(commandPlan.Pull) > 0 {
		appendLog("info", "开始拉取镜像...")
		if err := composeDeploymentCommandRunner(ctx, projectDir, commandPlan.Pull, streamLog); err != nil {
			finishCommandError("拉取镜像失败: ", err)
			return
		}
	}
	if cancelIfRequested() {
		return
	}
	if len(commandPlan.Build) > 0 {
		appendLog("info", "开始无缓存重构镜像...")
		if err := composeDeploymentCommandRunner(ctx, projectDir, commandPlan.Build, streamLog); err != nil {
			finishCommandError("重构镜像失败: ", err)
			return
		}
	}
	if cancelIfRequested() {
		return
	}

	upMessage := "正在启动服务..."
	if options.Recreate {
		upMessage = "正在使用现有镜像重建容器..."
	}
	appendLog("info", upMessage)
	if err := composeDeploymentCommandRunner(ctx, projectDir, commandPlan.Up, streamLog); err != nil {
		finishCommandError("部署失败: ", err)
		return
	}
	if cancelIfRequested() {
		return
	}

	if options.DeferVerification {
		appendLog("info", "服务启动命令已完成，等待部署任务核验")
		finish("success", gin.H{"project": projectName, "verification_pending": true}, "")
		return
	}
	verification, err := composeDeploymentVerifier(ctx, projectName)
	if err != nil || !verification.Verified {
		if err == nil {
			err = fmt.Errorf("部署验证未通过")
		}
		appendLog("error", "部署验证失败: "+err.Error())
		finish("error", gin.H{"project": projectName, "verification": verification}, err.Error())
		return
	}

	appendLog("success", "所有服务已成功启动")
	finish("success", gin.H{"project": projectName, "containers": len(verification.Containers), "verification": verification}, "")
}

type composeOperationOptions struct {
	ExpectedConfigHash        string
	ExpectedConfigFiles       map[string]string
	DeferVerification         bool // Internal Job adapter only; never accepted from API input.
	Pull                      bool
	Rebuild                   bool
	Recreate                  bool
	ExtraFiles                map[string]string
	Profiles                  []string
	Replace                   bool
	AIGrantedCapabilities     []string
	AICapabilityCandidateHash string
	ProjectDir                string
	ComposeProjectName        string
	DisplayName               string
	RelativePath              string
	DestroyFingerprint        string
	DestroySelected           []string
}

func composeUpdateCommands() (pullArgs []string, createArgs []string) {
	return []string{"compose", "pull"}, []string{"compose", "create", "--remove-orphans"}
}

func composeBoolQuery(c *gin.Context, names ...string) bool {
	for _, name := range names {
		value := strings.TrimSpace(strings.ToLower(c.Query(name)))
		if value == "true" || value == "1" || value == "yes" || value == "on" {
			return true
		}
	}
	return false
}

func composeBuildArgs(options composeOperationOptions) []string {
	args := composeCommandWithProfiles(options.Profiles, "build")
	if options.Pull {
		args = append(args, "--pull")
	}
	if options.Rebuild {
		args = append(args, "--no-cache")
	}
	return args
}

type composeDeployCommandPlan struct {
	Pull  []string
	Build []string
	Up    []string
}

func buildComposeDeployCommandPlan(options composeOperationOptions) composeDeployCommandPlan {
	plan := composeDeployCommandPlan{
		Up: composeCommandWithProfiles(options.Profiles, "up", "-d"),
	}
	if options.Recreate {
		plan.Up = append(plan.Up, "--force-recreate", "--no-build")
		return plan
	}
	if options.Pull {
		plan.Pull = composeCommandWithProfiles(options.Profiles, "pull")
	}
	if options.Rebuild {
		plan.Build = composeBuildArgs(options)
	}
	if options.Pull && !options.Rebuild {
		plan.Up = append(plan.Up, "--pull", "always")
	}
	return plan
}

func composeCommandWithProfiles(profiles []string, command ...string) []string {
	args := []string{"compose"}
	for _, profile := range normalizeComposeProfiles(profiles) {
		args = append(args, "--profile", profile)
	}
	return append(args, command...)
}

type composeExtraFileBatch struct {
	updates []composehistory.FileUpdate
	written []string
}

func planComposeExtraFiles(projectDir string, files map[string]string) (*composeExtraFileBatch, error) {
	if len(files) == 0 {
		return nil, nil
	}
	root, err := deployment.ResolveWithinRoot(projectDir, ".")
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(files))
	for key := range files {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	written := make([]string, 0, len(keys))
	updates := make([]composehistory.FileUpdate, 0, len(keys))
	seen := make(map[string]bool, len(keys))
	for _, rawPath := range keys {
		cleanPath, err := cleanComposeExtraFilePath(rawPath)
		if err != nil {
			return nil, err
		}
		content := files[rawPath]
		if len(content) > 1024*1024 {
			return nil, fmt.Errorf("额外配置文件过大: %s", cleanPath)
		}
		// 真实路径解析：项目内的符号链接链不得把额外文件写到项目根之外。
		targetPath, err := deployment.ResolveWithinRoot(projectDir, cleanPath)
		if err != nil {
			return nil, fmt.Errorf("额外配置文件路径越界: %s", cleanPath)
		}
		resolvedRelative, err := filepath.Rel(root, targetPath)
		if err != nil {
			return nil, err
		}
		if _, err := cleanComposeExtraFilePath(resolvedRelative); err != nil {
			return nil, fmt.Errorf("额外配置文件不允许通过别名覆盖核心文件: %s", cleanPath)
		}
		if seen[targetPath] {
			return nil, fmt.Errorf("额外配置文件路径重复: %s", cleanPath)
		}
		seen[targetPath] = true
		if info, statErr := os.Lstat(filepath.Join(projectDir, cleanPath)); statErr == nil && !info.Mode().IsRegular() {
			return nil, fmt.Errorf("config_file_type_conflict: 配置路径不是普通文件: %s", cleanPath)
		} else if statErr != nil && !os.IsNotExist(statErr) {
			return nil, fmt.Errorf("config_file_type_conflict: 读取配置路径失败: %s: %w", cleanPath, statErr)
		}
		for parent := filepath.Dir(targetPath); ; parent = filepath.Dir(parent) {
			info, statErr := os.Stat(parent)
			if statErr == nil {
				if !info.IsDir() {
					return nil, fmt.Errorf("config_file_type_conflict: 配置父路径不是目录: %s", cleanPath)
				}
				break
			}
			if !os.IsNotExist(statErr) || filepath.Dir(parent) == parent {
				return nil, fmt.Errorf("读取配置父路径失败: %s: %w", cleanPath, statErr)
			}
		}
		updates = append(updates, composehistory.FileUpdate{Path: targetPath, Content: []byte(content), Mode: 0644})
		written = append(written, filepath.ToSlash(cleanPath))
	}
	// Validate the whole batch before staging any content. Reuse the same
	// rollback-capable replacement used by Compose configuration history.
	for _, update := range updates {
		for parent := filepath.Dir(update.Path); parent != filepath.Dir(parent); parent = filepath.Dir(parent) {
			if seen[parent] {
				return nil, fmt.Errorf("config_file_type_conflict: 配置文件同时被用作父目录")
			}
		}
	}
	return &composeExtraFileBatch{updates: updates, written: written}, nil
}

func writeComposeExtraFiles(projectDir string, files map[string]string) ([]string, error) {
	batch, err := planComposeExtraFiles(projectDir, files)
	if err != nil || batch == nil {
		return nil, err
	}
	for _, update := range batch.updates {
		if err := os.MkdirAll(filepath.Dir(update.Path), 0755); err != nil {
			return nil, err
		}
	}
	if err := composehistory.ReplaceFiles(batch.updates); err != nil {
		return nil, err
	}
	return batch.written, nil
}

func cleanComposeExtraFilePath(raw string) (string, error) {
	clean := filepath.Clean(strings.TrimSpace(raw))
	if clean == "" || clean == "." || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("额外配置文件路径不安全: %s", raw)
	}
	if clean == "docker-compose.yml" || clean == "docker-compose.yaml" || clean == "compose.yml" || clean == "compose.yaml" || clean == ".env" {
		return "", fmt.Errorf("额外配置文件不允许覆盖核心文件: %s", clean)
	}
	return clean, nil
}

func composeUpdateApplyCommand(wasRunning bool) (label string, args []string, result string) {
	if wasRunning {
		return "开始重新创建并启动更新后的容器...", []string{"compose", "up", "-d", "--remove-orphans"}, "项目更新完成，已恢复运行状态"
	}
	_, createArgs := composeUpdateCommands()
	return "开始创建更新后的容器（不启动）...", createArgs, "项目更新完成，容器已创建但未启动"
}

func composeProjectWasRunning(ctx context.Context, projectName string) (bool, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return false, err
	}
	defer cli.Close()

	containers, err := cli.ContainerList(ctx, types.ContainerListOptions{
		All: true,
		Filters: filters.NewArgs(
			filters.Arg("label", "com.docker.compose.project="+projectName),
		),
	})
	if err != nil {
		return false, err
	}
	for _, container := range containers {
		if strings.EqualFold(strings.TrimSpace(container.State), "running") {
			return true, nil
		}
	}
	return false, nil
}

func createComposeOperationTask(c *gin.Context, operation string, options composeOperationOptions) {
	reference, err := validateManagedProjectReference(c.Param("name"))
	if err != nil {
		respondError(c, http.StatusBadRequest, "Compose 项目目录引用不合法", err)
		return
	}
	target, err := resolveComposeOperationTarget(c.Request.Context(), reference)
	if err != nil {
		respondComposeTargetError(c, err)
		return
	}
	if forbidIfSelfProject(c, target.ComposeProjectName) {
		return
	}
	options.ProjectDir = target.ProjectDir
	options.ComposeProjectName = target.ComposeProjectName
	options.DisplayName = target.DisplayName
	options.RelativePath = target.RelativePath

	taskID := fmt.Sprintf("%d", time.Now().UnixNano())
	_ = database.UpsertTask(taskID, "compose_"+operation, "pending")

	ctx, cancel := context.WithCancel(context.Background())
	registerComposeTaskCancel(taskID, cancel)
	go func() {
		defer unregisterComposeTaskCancel(taskID)
		defer cancel()
		runComposeOperationTaskWithContext(ctx, taskID, target.DisplayName, operation, options)
	}()

	c.JSON(http.StatusOK, gin.H{
		"message": "Compose 操作任务已提交",
		"taskId":  taskID,
		"project": target.DisplayName,
		"type":    "compose_" + operation,
	})
}

func existingComposeProjectDir(baseDir, projectName string) (string, error) {
	projectDir, err := resolveProjectDir(baseDir, projectName)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(projectDir)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("项目路径不是目录: %s", projectDir)
	}
	return projectDir, nil
}

func runComposeOperationTask(taskID string, projectName string, operation string, options composeOperationOptions) {
	runComposeOperationTaskWithContext(context.Background(), taskID, projectName, operation, options)
}

func runComposeOperationTaskWithContext(ctx context.Context, taskID string, projectName string, operation string, options composeOperationOptions) {
	if ctx == nil {
		ctx = context.Background()
	}
	target := composeOperationTarget{
		DisplayName:        options.DisplayName,
		ProjectDir:         options.ProjectDir,
		RelativePath:       options.RelativePath,
		ComposeProjectName: options.ComposeProjectName,
	}
	if target.ProjectDir == "" || target.ComposeProjectName == "" {
		resolved, err := resolveComposeOperationTarget(ctx, projectName)
		if err != nil {
			_ = database.UpsertTask(taskID, "compose_"+operation, "running")
			_ = database.AppendTaskLogWithSeq(taskID, 1, time.Now(), "error", "项目身份解析失败: "+err.Error())
			_ = database.FinishTask(taskID, "error", nil, err.Error())
			return
		}
		target = resolved
	}
	if target.DisplayName == "" {
		target.DisplayName = projectName
	}
	projectName = target.DisplayName
	composeProjectName := target.ComposeProjectName
	projectDir := target.ProjectDir

	unlockProject := lockComposeProjectMutation(composeProjectName)
	defer unlockProject()
	seq := int64(0)
	appendLog := func(logType string, message string) {
		seq++
		_ = database.AppendTaskLogWithSeq(taskID, seq, time.Now(), logType, message)
	}
	finish := func(status string, result any, errStr string) {
		invalidateComposeProjectListCache()
		_ = database.FinishTask(taskID, status, result, errStr)

		success := status == "success" || status == "completed"
		actionNames := map[string]string{
			"start":   "启动",
			"stop":    "停止",
			"kill":    "强制停止",
			"restart": "重启",
			"build":   "构建",
			"update":  "更新",
			"down":    "清理",
			"remove":  "删除",
			"destroy": "销毁",
		}
		actionName := actionNames[operation]
		if actionName == "" {
			actionName = operation
		}

		// 启动、停止、重启等常规成功结果只在当前页面反馈；失败始终进入通知中心。
		if !success {
			errText := strings.TrimSpace(errStr)
			if errText == "" {
				errText = "未知错误"
			}
			_ = database.SaveNotification(&database.Notification{
				Type:     "error",
				Category: "deploy_task",
				Message:  fmt.Sprintf("Compose 项目 %s %s失败：%s", projectName, actionName, errText),
				Read:     false,
			})
			return
		}

		// 构建和更新会改变镜像或运行配置，保留关键成功通知。
		if operation == "build" || operation == "update" {
			_ = database.SaveNotification(&database.Notification{
				Type:     "success",
				Category: "deploy_task",
				Message:  fmt.Sprintf("Compose 项目 %s %s成功", projectName, actionName),
				Read:     false,
			})
		}
	}

	taskType := "compose_" + operation
	_ = database.UpsertTask(taskID, taskType, "running")
	if operation == "apply_config" || operation == "build_config" {
		current, err := loadComposeConfigTarget(target)
		if err != nil || options.ExpectedConfigHash == "" || composeConfigFingerprint(current) != options.ExpectedConfigHash {
			finish("error", nil, "项目配置已变化，未执行配置应用，请重新预览")
			return
		}
		if err := verifyComposeConfigFiles(projectDir, options.ExpectedConfigFiles); err != nil {
			finish("error", nil, err.Error())
			return
		}
	}

	if operation == "start" || operation == "build" || operation == "update" {
		composePath, findErr := findComposeFile(projectDir)
		if findErr != nil {
			appendLog("error", "查找 Compose 文件失败: "+findErr.Error())
			finish("error", nil, findErr.Error())
			return
		}
		content, readErr := os.ReadFile(composePath)
		if readErr != nil {
			appendLog("error", "读取 Compose 文件失败: "+readErr.Error())
			finish("error", nil, readErr.Error())
			return
		}
		copied, prepareErr := prepareGitComposeProjectFiles(projectDir, filepath.Dir(composePath), string(content))
		if prepareErr != nil {
			appendLog("error", "Git 项目启动预检失败: "+prepareErr.Error())
			finish("error", nil, prepareErr.Error())
			return
		}
		for _, item := range copied {
			appendLog("info", "已从标准模板生成配置文件: "+item)
		}
	}

	runStep := func(label string, args []string) error {
		appendLog("info", label)
		onLine := func(line string) {
			line = strings.TrimSpace(line)
			if line == "" {
				return
			}
			logType := "info"
			lower := strings.ToLower(line)
			if strings.Contains(lower, "error") || strings.Contains(lower, "failed") {
				logType = "error"
			} else if strings.Contains(lower, "done") || strings.Contains(lower, "started") || strings.Contains(lower, "created") {
				logType = "success"
			}
			appendLog(logType, line)
		}
		args = composeCommandWithProjectName(composeProjectName, args)
		if operation == "apply_config" || operation == "build_config" {
			path, err := findComposeFile(projectDir)
			if err != nil {
				return err
			}
			return composeDeploymentCommandRunnerWithSource(ctx, projectDir, path, args, onLine)
		}
		return runComposeStreamLines(ctx, projectDir, args, onLine)
	}

	switch operation {
	case "apply_config", "build_config":
		if operation == "build_config" {
			if err := runStep("构建当前项目镜像...", []string{"compose", "build"}); err != nil {
				finish("error", nil, err.Error())
				return
			}
		}
		if err := runStep("应用已保存的配置...", []string{"compose", "up", "-d", "--no-build", "--pull", "never"}); err != nil {
			finish("error", nil, err.Error())
			return
		}
		appendLog("success", "配置已应用，项目文件和数据已保留")
	case "start":
		if err := runStep("开始启动服务...", []string{"compose", "up", "-d"}); err != nil {
			appendLog("error", "启动失败: "+err.Error())
			finish("error", nil, err.Error())
			return
		}
		appendLog("success", "启动完成")
	case "stop":
		appendLog("info", "开始按项目身份停止服务...")
		cli, clientErr := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
		if clientErr != nil {
			appendLog("error", "连接 Docker 失败: "+clientErr.Error())
			finish("error", nil, clientErr.Error())
			return
		}
		operationCtx, cancel := context.WithTimeout(ctx, time.Minute)
		stopped, stopErr := stopComposeProjectContainers(operationCtx, cli, composeProjectName, 2)
		cancel()
		_ = cli.Close()
		if stopErr != nil {
			appendLog("error", "停止失败: "+stopErr.Error())
			finish("error", nil, stopErr.Error())
			return
		}
		if stopped == 0 {
			appendLog("info", "项目容器已经停止")
		} else {
			appendLog("success", fmt.Sprintf("停止完成，共停止 %d 个容器", stopped))
		}
	case "kill":
		appendLog("info", "开始强制停止项目容器（SIGKILL，不做优雅停止）...")
		cli, clientErr := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
		if clientErr != nil {
			appendLog("error", "连接 Docker 失败: "+clientErr.Error())
			finish("error", nil, clientErr.Error())
			return
		}
		killed, killErr := killComposeProjectContainers(ctx, cli, composeProjectName)
		_ = cli.Close()
		if killErr != nil {
			appendLog("error", "强制停止失败: "+killErr.Error())
			finish("error", nil, killErr.Error())
			return
		}
		appendLog("success", fmt.Sprintf("强制停止完成，共终止 %d 个运行中的容器", killed))
	case "restart":
		appendLog("info", "开始按项目身份重启服务...")
		cli, clientErr := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
		if clientErr != nil {
			appendLog("error", "连接 Docker 失败: "+clientErr.Error())
			finish("error", nil, clientErr.Error())
			return
		}
		operationCtx, cancel := context.WithTimeout(ctx, time.Minute)
		restarted, restartErr := restartComposeProjectContainers(operationCtx, cli, composeProjectName, 2)
		cancel()
		_ = cli.Close()
		if restartErr != nil {
			appendLog("error", "重启失败: "+restartErr.Error())
			finish("error", nil, restartErr.Error())
			return
		}
		appendLog("success", fmt.Sprintf("重启完成，共重启 %d 个容器", restarted))
	case "down":
		appendLog("info", "开始清理项目容器和网络...")
		if err, warning := cleanProjectResourcesTargetContext(ctx, projectDir, composeProjectName); err != nil {
			appendLog("error", "清理失败: "+err.Error())
			finish("error", nil, err.Error())
			return
		} else if warning != "" {
			appendLog("warning", warning)
		}
		appendLog("success", "项目容器和网络已清理")
	case "remove":
		appendLog("info", "开始删除项目资源...")
		if err, warning := cleanProjectResourcesTargetContext(ctx, projectDir, composeProjectName); err != nil {
			appendLog("error", "清理失败: "+err.Error())
			finish("error", nil, err.Error())
			return
		} else if warning != "" {
			appendLog("warning", warning)
		}
		if tx, err := database.GetDB().Begin(); err == nil {
			if derr := database.DeleteReservedPortsByOwnerTx(tx, composeProjectName); derr != nil {
				_ = tx.Rollback()
			} else {
				_ = tx.Commit()
			}
		}
		if err := os.RemoveAll(projectDir); err != nil {
			appendLog("error", "删除项目目录失败: "+err.Error())
			finish("error", nil, err.Error())
			return
		}
		_ = database.DeleteComposeGitSourceInEnvironment(database.LocalEnvironmentID, projectName)
		_ = database.DeleteComposeProjectMetadata(database.LocalEnvironmentID, projectName)
		if err := database.DeleteComposeHistoryForProject(database.LocalEnvironmentID, projectName); err != nil {
			logging.Warn("Compose history cleanup after project removal failed", "project", projectName, "error", err)
		}
		if target.RelativePath != "" {
			_ = database.DeleteComposeProjectIdentity(database.LocalEnvironmentID, target.RelativePath)
		}
		appendLog("success", "项目已删除")
	case "destroy":
		appendLog("info", "正在核对销毁清单")
		result, destroyErr := executeComposeDestroy(ctx, target, options.DestroyFingerprint, options.DestroySelected, appendLog)
		if destroyErr != nil {
			appendLog("error", "销毁失败: "+destroyErr.Error())
			finish("error", result, destroyErr.Error())
			return
		}
		appendLog("success", "项目销毁完成；无法安全删除的资源已记录为保留项")
		finish("success", result, "")
		return
	case "build":
		buildArgs := composeBuildArgs(options)
		if err := runStep("开始构建服务...", buildArgs); err != nil {
			appendLog("error", "构建失败: "+err.Error())
			finish("error", nil, err.Error())
			return
		}
		if err := runStep("开始重新创建服务...", []string{"compose", "up", "-d", "--remove-orphans", "--force-recreate"}); err != nil {
			appendLog("error", "重建失败: "+err.Error())
			finish("error", nil, err.Error())
			return
		}
		appendLog("success", "构建完成")
	case "update":
		wasRunning, stateErr := composeProjectWasRunning(ctx, composeProjectName)
		if stateErr != nil {
			appendLog("error", "读取项目更新前状态失败: "+stateErr.Error())
			finish("error", nil, stateErr.Error())
			return
		}
		if options.Pull {
			appendLog("info", "强制检查并拉取最新镜像")
			pullArgs, _ := composeUpdateCommands()
			if err := runStep("开始拉取最新镜像...", pullArgs); err != nil {
				appendLog("error", "拉取镜像失败: "+err.Error())
				finish("error", nil, err.Error())
				return
			}
		}
		if options.Rebuild {
			if err := runStep("开始无缓存重构镜像...", composeBuildArgs(options)); err != nil {
				appendLog("error", "重构镜像失败: "+err.Error())
				finish("error", nil, err.Error())
				return
			}
		}
		applyLabel, applyArgs, resultMessage := composeUpdateApplyCommand(wasRunning)
		if err := runStep(applyLabel, applyArgs); err != nil {
			appendLog("error", "应用更新失败: "+err.Error())
			finish("error", nil, err.Error())
			return
		}
		clearImageUpdateRecordsByImageRefs(resolveComposeUpdateCleanupImages(ctx, projectDir, composeProjectName)...)
		appendLog("success", resultMessage)
	default:
		err := fmt.Errorf("不支持的 Compose 操作: %s", operation)
		appendLog("error", err.Error())
		finish("error", nil, err.Error())
		return
	}

	result := gin.H{"project": projectName, "composeProjectName": composeProjectName, "operation": operation}
	if operation == "apply_config" || operation == "build_config" {
		result["configHash"] = options.ExpectedConfigHash
	}
	finish("success", result, "")
}

func startProjectTask(c *gin.Context) {
	createComposeOperationTask(c, "start", composeOperationOptions{})
}

func stopProjectTask(c *gin.Context) {
	createComposeOperationTask(c, "stop", composeOperationOptions{})
}

// killProjectTask 强制停止项目：对项目容器逐个发送 SIGKILL，不做优雅停止、不修改重启策略。
func killProjectTask(c *gin.Context) {
	createComposeOperationTask(c, "kill", composeOperationOptions{})
}

func restartProjectTask(c *gin.Context) {
	createComposeOperationTask(c, "restart", composeOperationOptions{})
}

func downProjectTask(c *gin.Context) {
	createComposeOperationTask(c, "down", composeOperationOptions{})
}

func removeProjectTask(c *gin.Context) {
	createComposeOperationTask(c, "remove", composeOperationOptions{})
}

func buildProjectTask(c *gin.Context) {
	createComposeOperationTask(c, "build", composeOperationOptions{
		Pull:    composeBoolQuery(c, "pull"),
		Rebuild: composeBoolQuery(c, "rebuild", "noCache", "forceRebuild"),
	})
}

func updateProjectTask(c *gin.Context) {
	createComposeOperationTask(c, "update", composeOperationOptions{
		Pull:    composeBoolQuery(c, "pull"),
		Rebuild: composeBoolQuery(c, "rebuild", "noCache", "forceRebuild"),
	})
}

func listComposeTasks(c *gin.Context) {
	typesRaw := strings.TrimSpace(c.Query("types"))
	statusesRaw := strings.TrimSpace(c.Query("statuses"))
	limitRaw := strings.TrimSpace(c.Query("limit"))

	var taskTypes []string
	if typesRaw != "" {
		for _, s := range strings.Split(typesRaw, ",") {
			if v := strings.TrimSpace(s); v != "" {
				taskTypes = append(taskTypes, v)
			}
		}
	}
	if len(taskTypes) == 0 {
		taskTypes = []string{"compose_deploy", "remote_compose_deploy", "remote_appstore_deploy", "compose_git_import", "compose_git_sync", "compose_start", "compose_stop", "compose_kill", "compose_restart", "compose_build", "compose_update", "compose_down", "compose_remove", "compose_destroy"}
	}

	var statuses []string
	if statusesRaw != "" {
		for _, s := range strings.Split(statusesRaw, ",") {
			if v := strings.TrimSpace(s); v != "" {
				statuses = append(statuses, v)
			}
		}
	}

	limit := 50
	if limitRaw != "" {
		if v, err := strconv.Atoi(limitRaw); err == nil && v > 0 {
			limit = v
		}
	}

	environmentID, ok := composeTaskEnvironmentScope(c)
	if !ok {
		return
	}
	list, err := database.ListTasksInEnvironment(environmentID, taskTypes, statuses, limit)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "获取任务列表失败", err)
		return
	}
	c.JSON(http.StatusOK, list)
}

func getComposeTask(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		respondError(c, http.StatusBadRequest, "任务不存在", nil)
		return
	}
	environmentID, ok := composeTaskEnvironmentScope(c)
	if !ok {
		return
	}
	t, err := database.GetTaskInEnvironment(environmentID, id)
	if err != nil {
		respondError(c, http.StatusNotFound, "任务不存在", err)
		return
	}
	c.JSON(http.StatusOK, t)
}

// cancelComposeTask 取消一个正在运行的任务（部署等注册过取消函数的任务）。
func cancelComposeTask(c *gin.Context) {
	taskID := strings.TrimSpace(c.Param("id"))
	if taskID == "" {
		respondError(c, http.StatusBadRequest, "任务不存在", nil)
		return
	}
	environmentID, ok := composeTaskEnvironmentScope(c)
	if !ok {
		return
	}
	if _, err := database.GetTaskInEnvironment(environmentID, taskID); err != nil {
		respondError(c, http.StatusNotFound, "任务不存在", err)
		return
	}
	if handleEditionComposeTaskCancel(c, environmentID, taskID) {
		return
	}
	cancelVal, ok := composeTaskCancels.Load(taskID)
	if !ok {
		respondError(c, http.StatusConflict, "该任务已结束或不可取消", nil)
		return
	}
	cancel, ok := cancelVal.(context.CancelFunc)
	if !ok || cancel == nil {
		respondError(c, http.StatusConflict, "该任务已结束或不可取消", nil)
		return
	}
	cancel()
	c.JSON(http.StatusOK, gin.H{"message": "已请求中止任务", "taskId": taskID})
}

func composeTaskEvents(c *gin.Context) {
	environmentID, ok := composeTaskEnvironmentScope(c)
	if !ok {
		return
	}
	streamDatabaseTaskEventsInEnvironment(c, environmentID, strings.TrimSpace(c.Param("id")))
}

func composeTaskEnvironmentScope(c *gin.Context) (string, bool) {
	environmentID, ok := requestEnvironmentScope(c)
	if !ok {
		return "", false
	}
	if environmentID == "" || environmentID == database.LocalEnvironmentID {
		return database.LocalEnvironmentID, true
	}
	environment, err := database.GetEnvironment(environmentID)
	if err != nil || environment.ConnectionMode != "agent" || environment.Status == "revoked" {
		respondError(c, http.StatusNotFound, "远程环境不存在或已撤销", nil)
		return "", false
	}
	return environmentID, true
}

func resolveComposeReadTarget(c *gin.Context) (composeOperationTarget, bool) {
	target, err := resolveComposeOperationTarget(c.Request.Context(), c.Param("name"))
	if err != nil {
		respondComposeTargetError(c, err)
		return composeOperationTarget{}, false
	}
	return target, true
}

func respondComposeTargetError(c *gin.Context, err error) {
	if errors.Is(err, errComposeIdentityConflict) {
		respondErrorWithCode(c, http.StatusConflict, "compose_identity_conflict", "Compose 项目身份冲突", err)
		return
	}
	respondError(c, http.StatusNotFound, "Compose 项目目录不存在或身份无法确定", err)
}

func resolveComposeRequestTarget(c *gin.Context) (composeOperationTarget, bool) {
	target, ok := resolveComposeReadTarget(c)
	if !ok || forbidIfSelfProject(c, target.ComposeProjectName) {
		return composeOperationTarget{}, false
	}
	return target, true
}

func resolveComposeDockerProjectName(c *gin.Context) (string, bool) {
	if target, err := resolveComposeOperationTarget(c.Request.Context(), c.Param("name")); err == nil {
		return target.ComposeProjectName, true
	}
	name := strings.TrimSpace(c.Param("name"))
	if !isExactComposeProjectName(name) {
		respondError(c, http.StatusNotFound, "Compose 项目身份无法确定", nil)
		return "", false
	}
	return name, true
}

func resolveComposeSSETarget(c *gin.Context, eventID int64) (composeOperationTarget, bool) {
	target, err := resolveComposeOperationTarget(c.Request.Context(), c.Param("name"))
	if err != nil {
		sseWriteStringEvent(c, eventID, "log", "error: Compose 项目目录不存在或身份无法确定: "+err.Error())
		return composeOperationTarget{}, false
	}
	if isSelfProjectName(target.ComposeProjectName) {
		sseWriteStringEvent(c, eventID, "log", "error: 容器化部署模式下，禁止管理自身项目")
		return composeOperationTarget{}, false
	}
	return target, true
}

// startProject 启动项目
func startProject(c *gin.Context) {
	target, ok := resolveComposeRequestTarget(c)
	if !ok {
		return
	}

	// 异步执行启动命令
	go func() {
		// 使用 docker compose up 命令启动项目
		args := composeCommandWithProjectName(target.ComposeProjectName, []string{"compose", "up", "-d"})
		if output, err := runDockerCombinedOutput(context.Background(), target.ProjectDir, nil, args); err != nil {
			log.Printf("[ERROR] Error starting project %s: %s\nOutput: %s\n", target.DisplayName, err.Error(), string(output))
		}
	}()

	invalidateComposeProjectListCache()
	c.JSON(http.StatusAccepted, gin.H{"message": "项目启动指令已发送"})
}

// stopProject 停止项目
func stopProject(c *gin.Context) {
	target, ok := resolveComposeRequestTarget(c)
	if !ok {
		return
	}

	// 异步执行停止命令
	go func() {
		// 使用 docker compose stop 命令停止项目，添加 -t 2 缩短超时
		args := composeCommandWithProjectName(target.ComposeProjectName, []string{"compose", "stop", "-t", "2"})
		if output, err := runDockerCombinedOutput(context.Background(), target.ProjectDir, nil, args); err != nil {
			log.Printf("[ERROR] Error stopping project %s: %s\nOutput: %s\n", target.DisplayName, err.Error(), string(output))
		}
	}()

	invalidateComposeProjectListCache()
	c.JSON(http.StatusAccepted, gin.H{"message": "项目停止指令已发送"})
}

// restartProject 重启项目
func restartProject(c *gin.Context) {
	target, ok := resolveComposeRequestTarget(c)
	if !ok {
		return
	}

	// 异步执行重启命令
	go func() {
		// 使用 docker compose restart 命令重启项目，添加 -t 2 缩短超时
		args := composeCommandWithProjectName(target.ComposeProjectName, []string{"compose", "restart", "-t", "2"})
		if output, err := runDockerCombinedOutput(context.Background(), target.ProjectDir, nil, args); err != nil {
			log.Printf("[ERROR] Error restarting project %s: %s\nOutput: %s\n", target.DisplayName, err.Error(), string(output))
		}
	}()

	invalidateComposeProjectListCache()
	c.JSON(http.StatusAccepted, gin.H{"message": "项目重启指令已发送"})
}

// startProjectEvents 启动项目并推送 SSE 日志
func startProjectEvents(c *gin.Context) {
	setSSEHeaders(c)
	nextID := sseNextIDFromLastEventID(c)
	target, ok := resolveComposeSSETarget(c, nextID)
	if !ok {
		return
	}

	messageChan := make(chan string, 128)
	ctx := c.Request.Context()

	go func() {
		defer close(messageChan)

		send := func(line string) {
			line = strings.TrimSpace(line)
			if line == "" {
				return
			}
			select {
			case <-ctx.Done():
				return
			case messageChan <- line:
				return
			}
		}

		if _, err := os.Stat(target.ProjectDir); err != nil {
			send("error: 项目目录不存在")
			return
		}

		send("info: 开始启动服务...")
		args := composeCommandWithProjectName(target.ComposeProjectName, []string{"compose", "up", "-d"})
		if err := runComposeStreamLines(ctx, target.ProjectDir, args, send); err != nil {
			send(fmt.Sprintf("error: 启动失败: %s", err.Error()))
			return
		}

		send("success: 启动完成")
	}()

	c.Stream(func(w io.Writer) bool {
		select {
		case msg, ok := <-messageChan:
			if !ok {
				return false
			}
			sseWriteStringEvent(c, nextID, "log", msg)
			nextID++
			return true
		case <-c.Request.Context().Done():
			return false
		}
	})
}

// stopProjectEvents 停止项目并推送 SSE 日志
func stopProjectEvents(c *gin.Context) {
	setSSEHeaders(c)
	nextID := sseNextIDFromLastEventID(c)
	target, ok := resolveComposeSSETarget(c, nextID)
	if !ok {
		return
	}

	messageChan := make(chan string, 128)
	ctx := c.Request.Context()

	go func() {
		defer close(messageChan)

		send := func(line string) {
			line = strings.TrimSpace(line)
			if line == "" {
				return
			}
			select {
			case <-ctx.Done():
				return
			case messageChan <- line:
				return
			}
		}

		if _, err := os.Stat(target.ProjectDir); err != nil {
			send("error: 项目目录不存在")
			return
		}

		send("info: 开始停止服务...")
		args := composeCommandWithProjectName(target.ComposeProjectName, []string{"compose", "stop", "-t", "2"})
		if err := runComposeStreamLines(ctx, target.ProjectDir, args, send); err != nil {
			send(fmt.Sprintf("error: 停止失败: %s", err.Error()))
			return
		}

		send("success: 停止完成")
	}()

	c.Stream(func(w io.Writer) bool {
		select {
		case msg, ok := <-messageChan:
			if !ok {
				return false
			}
			sseWriteStringEvent(c, nextID, "log", msg)
			nextID++
			return true
		case <-c.Request.Context().Done():
			return false
		}
	})
}

// restartProjectEvents 重启项目并推送 SSE 日志
func restartProjectEvents(c *gin.Context) {
	setSSEHeaders(c)
	nextID := sseNextIDFromLastEventID(c)
	target, ok := resolveComposeSSETarget(c, nextID)
	if !ok {
		return
	}

	messageChan := make(chan string, 128)
	ctx := c.Request.Context()

	go func() {
		defer close(messageChan)

		send := func(line string) {
			line = strings.TrimSpace(line)
			if line == "" {
				return
			}
			select {
			case <-ctx.Done():
				return
			case messageChan <- line:
				return
			}
		}

		if _, err := os.Stat(target.ProjectDir); err != nil {
			send("error: 项目目录不存在")
			return
		}

		send("info: 开始重启服务...")
		args := composeCommandWithProjectName(target.ComposeProjectName, []string{"compose", "restart", "-t", "2"})
		if err := runComposeStreamLines(ctx, target.ProjectDir, args, send); err != nil {
			send(fmt.Sprintf("error: 重启失败: %s", err.Error()))
			return
		}

		send("success: 重启完成")
	}()

	c.Stream(func(w io.Writer) bool {
		select {
		case msg, ok := <-messageChan:
			if !ok {
				return false
			}
			sseWriteStringEvent(c, nextID, "log", msg)
			nextID++
			return true
		case <-c.Request.Context().Done():
			return false
		}
	})
}

// buildProjectEvents 构建项目并推送 SSE 事件
func buildProjectEvents(c *gin.Context) {
	setSSEHeaders(c)
	nextID := sseNextIDFromLastEventID(c)
	target, ok := resolveComposeSSETarget(c, nextID)
	if !ok {
		return
	}
	options := composeOperationOptions{
		Pull:    composeBoolQuery(c, "pull"),
		Rebuild: composeBoolQuery(c, "rebuild", "noCache", "forceRebuild"),
	}
	messageChan := make(chan string, 128)
	ctx := c.Request.Context()

	go func() {
		defer close(messageChan)

		send := func(line string) {
			line = strings.TrimSpace(line)
			if line == "" {
				return
			}
			select {
			case <-ctx.Done():
				return
			case messageChan <- line:
				return
			}
		}

		if _, err := os.Stat(target.ProjectDir); err != nil {
			send("error: 项目目录不存在")
			return
		}

		if options.Pull {
			send("info: 开始拉取最新镜像...")
			args := composeCommandWithProjectName(target.ComposeProjectName, []string{"compose", "pull"})
			if err := runComposeStreamLines(ctx, target.ProjectDir, args, send); err != nil {
				send(fmt.Sprintf("error: 拉取镜像失败: %s", err.Error()))
				return
			}
		}

		if options.Rebuild {
			send("info: 开始无缓存重构镜像...")
		} else {
			send("info: 开始构建服务...")
		}
		if err := runComposeStreamLines(ctx, target.ProjectDir, composeCommandWithProjectName(target.ComposeProjectName, composeBuildArgs(options)), send); err != nil {
			send(fmt.Sprintf("error: 构建失败: %s", err.Error()))
			return
		}

		send("info: 开始重新创建服务...")
		upArgs := composeCommandWithProjectName(target.ComposeProjectName, []string{"compose", "up", "-d", "--remove-orphans", "--force-recreate"})
		if err := runComposeStreamLines(ctx, target.ProjectDir, upArgs, send); err != nil {
			send(fmt.Sprintf("error: 重建失败: %s", err.Error()))
			return
		}

		send("success: 构建完成")
	}()

	// 发送事件
	c.Stream(func(w io.Writer) bool {
		select {
		case msg, ok := <-messageChan:
			if !ok {
				return false
			}
			sseWriteStringEvent(c, nextID, "log", msg)
			nextID++
			return true
		case <-c.Request.Context().Done():
			return false
		}
	})
}

// updateProjectEvents 更新项目（拉取镜像并创建容器，但不启动）并推送 SSE 事件
func updateProjectEvents(c *gin.Context) {
	setSSEHeaders(c)
	nextID := sseNextIDFromLastEventID(c)
	target, ok := resolveComposeSSETarget(c, nextID)
	if !ok {
		return
	}
	options := composeOperationOptions{
		Pull:    composeBoolQuery(c, "pull"),
		Rebuild: composeBoolQuery(c, "rebuild", "noCache", "forceRebuild"),
	}

	messageChan := make(chan string, 128)
	ctx := c.Request.Context()

	go func() {
		defer close(messageChan)

		send := func(line string) {
			line = strings.TrimSpace(line)
			if line == "" {
				return
			}
			select {
			case <-ctx.Done():
				return
			case messageChan <- line:
				return
			}
		}

		if _, err := os.Stat(target.ProjectDir); err != nil {
			send("error: 项目目录不存在")
			return
		}

		if options.Pull {
			send("info: 强制检查并拉取最新镜像...")
			send("info: 开始拉取最新镜像...")
			pullArgs, _ := composeUpdateCommands()
			if err := runComposeStreamLines(ctx, target.ProjectDir, composeCommandWithProjectName(target.ComposeProjectName, pullArgs), send); err != nil {
				send(fmt.Sprintf("error: 拉取镜像失败: %s", err.Error()))
				return
			}
		}

		wasRunning, stateErr := composeProjectWasRunning(ctx, target.ComposeProjectName)
		if stateErr != nil {
			send(fmt.Sprintf("error: 读取项目更新前状态失败: %s", stateErr.Error()))
			return
		}
		if options.Rebuild {
			send("info: 开始无缓存重构镜像...")
			if err := runComposeStreamLines(ctx, target.ProjectDir, composeCommandWithProjectName(target.ComposeProjectName, composeBuildArgs(options)), send); err != nil {
				send(fmt.Sprintf("error: 重构镜像失败: %s", err.Error()))
				return
			}
		}

		applyLabel, applyArgs, resultMessage := composeUpdateApplyCommand(wasRunning)
		send("info: " + applyLabel)
		if err := runComposeStreamLines(ctx, target.ProjectDir, composeCommandWithProjectName(target.ComposeProjectName, applyArgs), send); err != nil {
			send(fmt.Sprintf("error: 应用更新失败: %s", err.Error()))
			return
		}

		clearImageUpdateRecordsByImageRefs(resolveComposeUpdateCleanupImages(ctx, target.ProjectDir, target.ComposeProjectName)...)
		send("success: " + resultMessage)
	}()

	// 发送事件
	c.Stream(func(w io.Writer) bool {
		select {
		case msg, ok := <-messageChan:
			if !ok {
				return false
			}
			sseWriteStringEvent(c, nextID, "log", msg)
			nextID++
			return true
		case <-c.Request.Context().Done():
			return false
		}
	})
}

// buildProject 构建项目
func buildProject(c *gin.Context) {
	target, ok := resolveComposeRequestTarget(c)
	if !ok {
		return
	}

	args := composeBuildArgs(composeOperationOptions{
		Pull:    composeBoolQuery(c, "pull"),
		Rebuild: composeBoolQuery(c, "rebuild", "noCache", "forceRebuild"),
	})

	args = composeCommandWithProjectName(target.ComposeProjectName, args)
	if output, err := runDockerCombinedOutput(c.Request.Context(), target.ProjectDir, nil, args); err != nil {
		respondError(c, http.StatusInternalServerError, "构建失败", fmt.Errorf("%s\n%s", err.Error(), string(output)))
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "项目构建完成"})
}

func splitImageRef(raw string) (string, string) {
	ref := strings.TrimSpace(raw)
	if ref == "" {
		return "", ""
	}
	lastSlash := strings.LastIndex(ref, "/")
	lastColon := strings.LastIndex(ref, ":")
	if lastColon > lastSlash {
		name := strings.TrimSpace(ref[:lastColon])
		tag := strings.TrimSpace(ref[lastColon+1:])
		if tag == "" {
			tag = "latest"
		}
		return name, tag
	}
	return ref, "latest"
}

func imageHostFromName(name string) string {
	segments := strings.Split(name, "/")
	if len(segments) == 0 {
		return ""
	}
	host := segments[0]
	if strings.Contains(host, ".") || strings.Contains(host, ":") || host == "localhost" {
		return host
	}
	return ""
}

func normalizeImageVariants(raw string) []string {
	ref := strings.TrimSpace(raw)
	if ref == "" {
		return nil
	}
	// repo:tag@digest 写法：先按 @ 分离出裸 repo:tag 再解析（直接在最后一个冒号切
	// 会切到 digest 的冒号上），裸引用同样进入变体集合，让按 tag 记录的清理想法命中。
	namePart := ref
	if i := strings.Index(ref, "@"); i >= 0 {
		namePart = strings.TrimSpace(ref[:i])
	}
	name, tag := splitImageRef(namePart)
	if name == "" {
		return []string{ref}
	}
	if tag == "" {
		tag = "latest"
	}
	out := map[string]struct{}{}
	out[ref] = struct{}{}
	out[name+":"+tag] = struct{}{}

	if strings.HasPrefix(name, "docker.io/") {
		name = strings.TrimPrefix(name, "docker.io/")
		out[name+":"+tag] = struct{}{}
	}

	if strings.HasPrefix(name, "library/") {
		short := strings.TrimPrefix(name, "library/")
		out[short+":"+tag] = struct{}{}
		out["docker.io/library/"+short+":"+tag] = struct{}{}
	} else {
		out["docker.io/"+name+":"+tag] = struct{}{}
	}

	if imageHostFromName(name) == "" {
		if !strings.Contains(name, "/") {
			out["library/"+name+":"+tag] = struct{}{}
			out["docker.io/library/"+name+":"+tag] = struct{}{}
		} else {
			out["docker.io/"+name+":"+tag] = struct{}{}
		}
	}

	keys := make([]string, 0, len(out))
	for k := range out {
		keys = append(keys, k)
	}
	return keys
}

// resolveImageName 将镜像 ID（如果是 sha256 格式）解析为镜像名称
func resolveImageName(image string) string {
	if !strings.HasPrefix(image, "sha256:") {
		return image
	}
	// 获取完整的镜像 ID（去掉 sha256: 前缀）
	digest := strings.TrimPrefix(image, "sha256:")
	// 查找本地镜像获取镜像名称
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return image
	}
	defer cli.Close()
	// 使用镜像摘要查找
	if images, err := cli.ImageList(context.Background(), types.ImageListOptions{}); err == nil {
		for _, img := range images {
			if strings.HasPrefix(img.ID, "sha256:"+digest) || strings.HasPrefix(img.ID, digest) {
				// 找到匹配的镜像，返回第一个 RepoTag
				for _, tag := range img.RepoTags {
					if tag != "" && tag != "<none>:<none>" {
						return tag
					}
				}
			}
		}
	}
	return image
}

// 检查镜像是否有更新，同时排除远程不可用的镜像
func hasImageUpdate(updateMap map[string]bool, unavailableMap map[string]bool, image string) bool {
	if updateMap == nil {
		return false
	}
	for _, key := range normalizeImageVariants(image) {
		// 如果远程不可用（鉴权失败或不存在），不提示更新
		if unavailableMap != nil && unavailableMap[key] {
			continue
		}
		if updateMap[key] {
			return true
		}
	}
	return false
}

// parseComposeConfigImages 解析 docker compose config --images 的输出（每行一个渲染后的镜像引用）
func parseComposeConfigImages(output string) []string {
	var images []string
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			images = append(images, line)
		}
	}
	return images
}

// composeConfigRenderedImages 通过 docker compose config --images 获取解析（插值）后的
// 镜像引用列表；失败返回 nil，调用方负责回落。
func composeConfigRenderedImages(ctx context.Context, projectDir, composeProjectName string) []string {
	args := composeCommandWithProjectName(composeProjectName, []string{"compose", "config", "--images"})
	output, err := runDockerCombinedOutput(ctx, projectDir, nil, args)
	if err != nil {
		return nil
	}
	return parseComposeConfigImages(string(output))
}

// resolveComposeUpdateCleanupImages 返回 compose 更新成功后用于清理镜像更新记录的镜像
// 引用：优先用渲染后的引用（支持 image: ${VAR} 插值），失败回落原始 YAML 提取。
func resolveComposeUpdateCleanupImages(ctx context.Context, projectDir, composeProjectName string) []string {
	if images := composeConfigRenderedImages(ctx, projectDir, composeProjectName); len(images) > 0 {
		return images
	}
	return extractComposeImagesFromProject(projectDir)
}

func extractComposeImagesFromProject(projectPath string) []string {
	composePath, err := findComposeFile(projectPath)
	if err != nil {
		return nil
	}
	data, err := os.ReadFile(composePath)
	if err != nil {
		return nil
	}

	var root map[string]interface{}
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil
	}

	images := make([]string, 0)
	pushImage := func(v interface{}) {
		svc, ok := v.(map[string]interface{})
		if !ok {
			return
		}
		raw, ok := svc["image"]
		if !ok {
			return
		}
		image, ok := raw.(string)
		if !ok {
			return
		}
		image = strings.TrimSpace(image)
		if image == "" {
			return
		}
		images = append(images, image)
	}

	if servicesRaw, ok := root["services"]; ok {
		if services, ok := servicesRaw.(map[string]interface{}); ok {
			for _, svc := range services {
				pushImage(svc)
			}
			return images
		}
	}

	for _, svc := range root {
		pushImage(svc)
	}

	return images
}

func pathRelativeToRoot(root, candidate string) (string, bool) {
	root = strings.TrimSpace(root)
	candidate = strings.TrimSpace(candidate)
	if root == "" || candidate == "" {
		return "", false
	}
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(candidate))
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", false
	}
	return rel, true
}

// canonicalComposeProjectPath 将 Docker daemon 标签中的宿主机路径映射成
// TRADIS 容器内可读的项目路径，避免同一目录因两种绝对路径被识别为两个项目。
func canonicalComposeProjectPath(rawPath, projectName string) string {
	return canonicalComposeProjectPathWithFallback(rawPath, projectName, true)
}

// canonicalComposeObservedProjectPath only translates paths that are
// demonstrably under the configured host/container project root. Docker
// observation must never use the legacy name-based fallback because an
// unrelated external stack can share the same Compose project label.
func canonicalComposeObservedProjectPath(rawPath, projectName string) string {
	return canonicalComposeProjectPathWithFallback(rawPath, projectName, false)
}

func canonicalComposeProjectPathWithFallback(rawPath, projectName string, allowManagedFallback bool) string {
	rawPath = strings.TrimSpace(rawPath)
	containerRoot := filepath.Clean(getProjectsBaseDir())
	hostRoot := strings.TrimSpace(effectiveHostProjectRoot())
	if hostRoot != "" {
		hostRoot = filepath.Clean(hostRoot)
	}

	if rawPath != "" {
		cleanPath := filepath.Clean(rawPath)
		if rel, ok := pathRelativeToRoot(hostRoot, cleanPath); ok {
			if name, valid := validateComposeProjectName(projectName); valid {
				parts := strings.Split(filepath.Clean(rel), string(filepath.Separator))
				if len(parts) > 1 && parts[0] == name {
					return filepath.Join(containerRoot, name)
				}
			}
			return filepath.Clean(filepath.Join(containerRoot, rel))
		}
		if rel, ok := pathRelativeToRoot(containerRoot, cleanPath); ok {
			if name, valid := validateComposeProjectName(projectName); valid {
				parts := strings.Split(filepath.Clean(rel), string(filepath.Separator))
				if len(parts) > 1 && parts[0] == name {
					return filepath.Join(containerRoot, name)
				}
			}
			return cleanPath
		}
	}

	if allowManagedFallback {
		if name, ok := validateComposeProjectName(projectName); ok && !isSelfProjectName(name) {
			managedPath := filepath.Clean(filepath.Join(containerRoot, name))
			if _, err := os.Stat(managedPath); err == nil {
				return managedPath
			}
			if rawPath == "" {
				return managedPath
			}
		}
	}

	if rawPath != "" {
		return filepath.Clean(rawPath)
	}
	return ""
}

// listProjects 获取项目列表
func listProjects(c *gin.Context) {
	forceRefresh := c.Query("refresh") == "1" || c.Query("force") == "1"
	now := time.Now()

	if !forceRefresh {
		composeProjectListCacheMu.Lock()
		cachedAt := composeProjectListCache.fetchedAt
		cachedProjects := cloneComposeProjects(composeProjectListCache.projects)
		composeProjectListCacheMu.Unlock()
		if len(cachedProjects) > 0 && !cachedAt.IsZero() && now.Sub(cachedAt) < composeProjectListCacheTTL {
			c.JSON(http.StatusOK, cachedProjects)
			return
		}
	}

	// 创建 Docker 客户端
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		respondError(c, http.StatusInternalServerError, "创建 Docker 客户端失败", err)
		return
	}
	defer cli.Close()

	// 获取所有带有 compose 标签的容器
	containers, err := cli.ContainerList(c.Request.Context(), types.ContainerListOptions{
		All: true,
		Filters: filters.NewArgs(
			filters.Arg("label", "com.docker.compose.project"),
		),
	})
	if err != nil {
		respondError(c, http.StatusInternalServerError, "获取项目容器失败", err)
		return
	}

	// 以容器内可访问的 canonical path 为唯一键。
	projects := make(map[string]*ComposeProject)
	observedNamesByPath := make(map[string][]string)
	occupiedComposeNames := make(map[string]struct{})

	// 获取所有镜像更新信息
	allUpdates, _ := database.GetAllImageUpdates()
	updateMap := make(map[string]bool)
	unavailableMap := make(map[string]bool)

	// 首先获取所有远程不可用的镜像（包括那些不在 image_updates 表中的）
	unavailableImages, _ := database.GetAllUnavailableImageRemoteDigests()
	for _, repoTag := range unavailableImages {
		for _, key := range normalizeImageVariants(repoTag) {
			unavailableMap[key] = true
		}
	}

	// 处理有更新的镜像
	for _, u := range allUpdates {
		// 如果已经在 unavailableMap 中，跳过
		if unavailableMap[u.RepoTag] {
			continue
		}
		if u.LocalDigest != u.RemoteDigest {
			for _, key := range normalizeImageVariants(u.RepoTag) {
				updateMap[key] = true
			}
		}
	}

	// 遍历容器，按项目分组
	for _, container := range containers {
		projectName := strings.TrimSpace(container.Labels["com.docker.compose.project"])
		workingDir := strings.TrimSpace(container.Labels["com.docker.compose.project.working_dir"])
		configFile := strings.TrimSpace(container.Labels["com.docker.compose.project.config_files"])

		if projectName == "" {
			if workingDir != "" {
				projectName = filepath.Base(workingDir)
			} else if configFile != "" {
				projectName = filepath.Base(filepath.Dir(configFile))
			}
		}
		if projectName == "" {
			continue
		}
		if isExactComposeProjectName(projectName) {
			occupiedComposeNames[projectName] = struct{}{}
		}

		rawProjectPath := workingDir
		if rawProjectPath == "" && configFile != "" {
			rawProjectPath = filepath.Dir(strings.Split(configFile, ",")[0])
		}
		projectPath := canonicalComposeObservedProjectPath(rawProjectPath, projectName)
		if projectPath == "" {
			continue
		}
		projectKey := filepath.Clean(projectPath)
		observedNamesByPath[projectKey] = append(observedNamesByPath[projectKey], projectName)
		project := projects[projectKey]
		if project == nil {
			displayName := projectName
			_, managed := pathRelativeToRoot(getProjectsBaseDir(), projectPath)
			if managed {
				displayName = filepath.Base(projectPath)
			}
			isSelf := isSelfProjectName(projectName) || isSelfProjectName(displayName)
			project = &ComposeProject{
				Name:               displayName,
				ComposeProjectName: projectName,
				IdentitySource:     "docker_label",
				Path:               projectPath,
				Containers:         0,
				Status:             "已停止",
				CreateTime:         time.Unix(container.Created, 0),
				IsSelf:             isSelf,
				IsManaged:          managed || isSelf,
			}
			projects[projectKey] = project
		}

		project.Containers++

		imageName := resolveImageName(container.Image)
		if hasImageUpdate(updateMap, unavailableMap, imageName) {
			project.UpdateAvailable = true
			project.UpdateCount++
		}

		if container.State == "running" {
			project.Status = "运行中"
		}
	}

	// 补充扫描项目根目录下的项目，即使没有运行容器，也应该显示在列表中
	projectBaseDir := getProjectsBaseDir()
	entries, err := os.ReadDir(projectBaseDir)
	if err == nil {
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			displayName := entry.Name()
			if displayName == "#recycle" || !deployment.IsVisibleComposeProjectDirectory(displayName) {
				continue
			}
			projectDir, pathErr := resolveExistingManagedProjectDir(projectBaseDir, displayName)
			if pathErr != nil || !dirHasComposeFile(projectDir) {
				continue
			}

			projectKey := filepath.Clean(projectDir)
			project := projects[projectKey]
			if shouldHideEditionComposeDraft(projectDir, project != nil && project.Containers > 0) {
				continue
			}
			identity, identityErr := resolveManagedComposeIdentity(projectDir, observedNamesByPath[projectKey], occupiedComposeNames)
			if project == nil {
				info, _ := entry.Info()
				project = &ComposeProject{
					Name:       displayName,
					Path:       projectDir,
					Containers: 0,
					Status:     "已停止",
					CreateTime: info.ModTime(),
					IsManaged:  true,
				}
				projects[projectKey] = project
			}
			project.Name = displayName
			project.Path = projectDir
			project.IsManaged = true
			if identityErr != nil {
				project.IdentityError = identityErr.Error()
			} else {
				project.ComposeProjectName = identity.ComposeProjectName
				project.IdentitySource = identity.Source
				project.IsSelf = isSelfProjectName(identity.ComposeProjectName) || isSelfProjectName(displayName)
				occupiedComposeNames[identity.ComposeProjectName] = struct{}{}
			}
		}
	}

	for _, project := range projects {
		if project.UpdateAvailable {
			continue
		}
		images := extractComposeImagesFromProject(project.Path)
		if len(images) == 0 {
			continue
		}
		seen := make(map[string]struct{})
		for _, image := range images {
			if !hasImageUpdate(updateMap, unavailableMap, image) {
				continue
			}
			if _, exists := seen[image]; exists {
				continue
			}
			seen[image] = struct{}{}
			project.UpdateAvailable = true
			project.UpdateCount++
		}
	}

	// 转换为数组
	result := make([]*ComposeProject, 0, len(projects))
	containerProjectRoot := getProjectsBaseDir()

	gitSources, _ := database.ListComposeGitSourcesInEnvironment(database.LocalEnvironmentID)
	remarks, _ := database.ListComposeProjectRemarks(database.LocalEnvironmentID)
	for _, project := range projects {
		project.Remark = remarks[project.Name]
		if project.Remark == "" && project.ComposeProjectName != "" {
			project.Remark = remarks[project.ComposeProjectName]
		}
		if source, exists := gitSources[project.Name]; exists {
			sourceCopy := source
			project.GitSource = &sourceCopy
		}
		if composePath, err := findComposeFile(project.Path); err == nil {
			if data, err := os.ReadFile(composePath); err == nil {
				project.Compose = string(data)
			}
		}

		if relPath, ok := pathRelativeToRoot(containerProjectRoot, project.Path); ok {
			if relPath == "." {
				project.Path = "project"
			} else {
				project.Path = filepath.ToSlash(filepath.Join("project", relPath))
			}
		}

		result = append(result, project)
	}

	// 按创建时间倒序排序
	sort.Slice(result, func(i, j int) bool {
		return result[i].CreateTime.After(result[j].CreateTime)
	})

	composeProjectListCacheMu.Lock()
	composeProjectListCache.fetchedAt = now
	composeProjectListCache.projects = cloneComposeProjects(result)
	composeProjectListCacheMu.Unlock()

	c.JSON(http.StatusOK, result)
}

func updateComposeProjectMetadata(c *gin.Context) {
	name, err := validateManagedProjectReference(c.Param("name"))
	if err != nil {
		respondError(c, http.StatusBadRequest, "Compose 项目目录引用不合法", err)
		return
	}
	var request struct {
		Remark string `json:"remark"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		respondError(c, http.StatusBadRequest, "请求参数无效", err)
		return
	}
	request.Remark = strings.TrimSpace(request.Remark)
	if len([]rune(request.Remark)) > 128 {
		respondError(c, http.StatusBadRequest, "备注不能超过 128 个字符", nil)
		return
	}
	if err := database.SetComposeProjectRemark(database.LocalEnvironmentID, name, request.Remark); err != nil {
		respondError(c, http.StatusInternalServerError, "保存 Compose 备注失败", err)
		return
	}
	invalidateComposeProjectListCache()
	c.JSON(http.StatusOK, gin.H{"name": name, "remark": request.Remark})
}

// listContainerRemarks 返回全部独立容器备注，键为容器名。
func listContainerRemarks(c *gin.Context) {
	all, err := database.ListComposeProjectRemarks(database.LocalEnvironmentID)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "读取容器备注失败", err)
		return
	}
	result := make(map[string]string)
	for key, remark := range all {
		if strings.HasPrefix(key, containerRemarkKeyPrefix) {
			result[strings.TrimPrefix(key, containerRemarkKeyPrefix)] = remark
		}
	}
	c.JSON(http.StatusOK, result)
}

// updateContainerRemark 保存独立容器备注，键与 Compose 项目隔离。
func updateContainerRemark(c *gin.Context) {
	name := strings.TrimSpace(c.Param("name"))
	if name == "" || !containerNameRe.MatchString(name) {
		respondError(c, http.StatusBadRequest, "容器名不合法：仅支持字母/数字，且可包含 _ . -，并以字母或数字开头", nil)
		return
	}
	var request struct {
		Remark string `json:"remark"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		respondError(c, http.StatusBadRequest, "请求参数无效", err)
		return
	}
	request.Remark = strings.TrimSpace(request.Remark)
	if len([]rune(request.Remark)) > 128 {
		respondError(c, http.StatusBadRequest, "备注不能超过 128 个字符", nil)
		return
	}
	if err := database.SetComposeProjectRemark(database.LocalEnvironmentID, containerRemarkKeyPrefix+name, request.Remark); err != nil {
		respondError(c, http.StatusInternalServerError, "保存容器备注失败", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"name": name, "remark": request.Remark})
}

// deployEvents 处理部署事件
func deployEvents(c *gin.Context) {
	projectNameRaw := c.Query("name")
	compose := c.Query("compose")
	dotenvRaw := c.Query("dotenv")
	envRaw := c.Query("env")
	options := composeOperationOptions{
		Pull:    composeBoolQuery(c, "pull"),
		Rebuild: composeBoolQuery(c, "rebuild", "noCache", "forceRebuild"),
	}

	if projectNameRaw == "" || compose == "" {
		respondError(c, http.StatusBadRequest, "项目名称和配置内容不能为空", nil)
		return
	}
	if !composeYAMLHasServices([]byte(compose)) {
		respondError(c, http.StatusBadRequest, "Compose YAML 根节点必须包含非空 services 映射", nil)
		return
	}

	projectName, ok := validateComposeProjectName(projectNameRaw)
	if !ok {
		respondError(c, http.StatusBadRequest, "项目名不合法：仅支持小写字母/数字，且可包含 _ -，并以字母或数字开头", nil)
		return
	}
	if forbidIfSelfProject(c, projectName) {
		return
	}

	setSSEHeaders(c)
	nextID := sseNextIDFromLastEventID(c)

	messageChan := make(chan map[string]interface{}, 128)
	doneChan := make(chan bool)
	ctx := c.Request.Context()

	go func() {
		defer close(messageChan)

		sendMessage := func(msgType, msg string) {
			payload := map[string]interface{}{
				"type":    msgType,
				"message": msg,
			}
			select {
			case <-ctx.Done():
				return
			case <-doneChan: // 检查是否已完成
				return
			case messageChan <- payload:
				return
			}
		}

		projectDir, err := resolveProjectDir(getProjectsBaseDir(), projectName)
		if err != nil {
			sendMessage("error", "项目目录校验失败: "+err.Error())
			return
		}
		composePath := filepath.Join(projectDir, "docker-compose.yml")
		envPath := filepath.Join(projectDir, ".env")

		hostProjectDir := ""
		if hostRoot := effectiveHostProjectRoot(); hostRoot != "" {
			hostProjectDir = filepath.Join(hostRoot, projectName)
		}
		sendMessage("info", fmt.Sprintf("项目目录（容器内）: %s", projectDir))
		if hostProjectDir != "" {
			sendMessage("info", fmt.Sprintf("对应宿主机目录（PROJECT_ROOT）: %s", hostProjectDir))
		} else {
			sendMessage("warning", "未探测到 PROJECT_ROOT，宿主机映射路径未知；请确认已正确设置并挂载 PROJECT_ROOT")
		}

		// 检查项目目录是否已存在
		if _, err := os.Stat(projectDir); err == nil {
			_, findErr := findComposeFile(projectDir)
			switch {
			case findErr == nil:
				sendMessage("error", fmt.Sprintf("项目 '%s' 已存在，如需重新部署请先删除现有项目", projectName))
				return
			case errors.Is(findErr, errComposeFileNotFound):
				// 空目录采纳：仅在明确未找到 Compose 文件时继续，其余错误终止。
				sendMessage("info", fmt.Sprintf("目录 '%s' 已存在但不含 Compose 文件，将在其中创建并继续部署", projectName))
			default:
				sendMessage("error", "检查项目 Compose 文件失败: "+findErr.Error())
				return
			}
		} else if !os.IsNotExist(err) {
			// 其他错误
			sendMessage("error", "检查项目目录失败: "+err.Error())
			return
		}

		// 创建项目目录（如果不存在）
		if err := os.MkdirAll(projectDir, 0755); err != nil {
			sendMessage("error", "创建项目目录失败: "+err.Error())
			return
		}

		composeToWrite := compose

		if pathErrs := validateComposeAssetPaths(composeToWrite); len(pathErrs) > 0 {
			sendMessage("error", "Compose 路径校验失败: "+strings.Join(pathErrs, "; "))
			return
		}

		envMap := make(map[string]string)
		if strings.TrimSpace(envRaw) != "" {
			if err := json.Unmarshal([]byte(envRaw), &envMap); err != nil {
				sendMessage("warning", "解析 env 参数失败，将仅使用 dotenv: "+err.Error())
				envMap = make(map[string]string)
			}
		}

		dotenvText := strings.ReplaceAll(dotenvRaw, "\r\n", "\n")
		if strings.TrimSpace(dotenvText) != "" && len(envMap) > 0 {
			for k, v := range envMap {
				dotenvText = upsertDotenvKeyValue(dotenvText, k, v)
			}
		}

		allowedKeys := extractComposeInterpolationKeys(composeToWrite)
		dotenvText = filterDotenvByAllowedKeys(dotenvText, allowedKeys)

		if err := os.WriteFile(composePath, []byte(composeToWrite), 0644); err != nil {
			sendMessage("error", "保存配置文件失败: "+err.Error())
			return
		}

		if err := os.WriteFile(envPath, []byte(dotenvText), 0600); err != nil {
			sendMessage("error", "保存 .env 文件失败: "+err.Error())
			return
		}

		streamMessage := func(line string) {
			msgType := "info"
			if strings.Contains(line, "error") || strings.Contains(line, "Error") {
				msgType = "error"
			} else if strings.Contains(line, "Created") || strings.Contains(line, "Started") || strings.Contains(line, "Built") {
				msgType = "success"
			}
			sendMessage(msgType, line)
		}
		if options.Pull {
			sendMessage("info", "开始拉取镜像...")
			if err := runComposeStreamLines(ctx, projectDir, []string{"compose", "pull"}, streamMessage); err != nil {
				sendMessage("error", "拉取镜像失败: "+err.Error())
				return
			}
		}
		if options.Rebuild {
			sendMessage("info", "开始无缓存重构镜像...")
			if err := runComposeStreamLines(ctx, projectDir, composeBuildArgs(options), streamMessage); err != nil {
				sendMessage("error", "重构镜像失败: "+err.Error())
				return
			}
		}

		sendMessage("info", "正在启动服务...")
		upArgs := []string{"compose", "up", "-d"}
		if options.Pull && !options.Rebuild {
			upArgs = append(upArgs, "--pull", "always")
		}
		if err := runComposeStreamLines(ctx, projectDir, upArgs, streamMessage); err != nil {
			sendMessage("error", "部署失败: "+err.Error())
			return
		}

		// 检查容器状态
		cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
		if err != nil {
			sendMessage("error", "Docker客户端初始化失败: "+err.Error())
			return
		}
		defer cli.Close()

		containers, err := cli.ContainerList(ctx, types.ContainerListOptions{
			All: true,
			Filters: filters.NewArgs(
				filters.Arg("label", "com.docker.compose.project="+projectName),
			),
		})
		if err != nil {
			sendMessage("error", "获取容器状态失败: "+err.Error())
			return
		}

		// 检查所有容器是否都在运行
		allRunning := true
		for _, container := range containers {
			if container.State != "running" {
				allRunning = false
				break
			}
		}

		if allRunning {
			sendMessage("success", "所有服务已成功启动")
		} else {
			sendMessage("warning", "部分服务可能未正常启动，请检查状态")
		}
	}()

	c.Stream(func(w io.Writer) bool {
		select {
		case msg, ok := <-messageChan:
			if !ok {
				close(doneChan) // 标记为已完成
				return false
			}
			sseWriteJSONEvent(c, nextID, "message", msg)
			nextID++
			return true
		case <-c.Request.Context().Done():
			close(doneChan)
			return false
		}
	})
}

// getStackStatus 获取堆栈状态
func getStackStatus(c *gin.Context) {
	composeProjectName, ok := resolveComposeDockerProjectName(c)
	if !ok {
		return
	}
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		respondError(c, http.StatusInternalServerError, "创建 Docker 客户端失败", err)
		return
	}
	defer cli.Close()

	// 获取项目的所有容器
	containers, err := cli.ContainerList(c.Request.Context(), types.ContainerListOptions{
		All: true,
		Filters: filters.NewArgs(
			filters.Arg("label", "com.docker.compose.project="+composeProjectName),
		),
	})
	if err != nil {
		respondError(c, http.StatusInternalServerError, "获取容器状态失败", err)
		return
	}

	// 转换容器信息为前端需要的格式
	containerList := make([]map[string]interface{}, 0)
	for _, container := range containers {
		// 移除 ContainerStats 调用以提高性能
		// stats, err := cli.ContainerStats(context.Background(), container.ID, false)

		containerInfo := map[string]interface{}{
			"name":      strings.TrimPrefix(container.Names[0], "/"),
			"service":   composeLogServiceName(container),
			"image":     container.Image,
			"status":    container.State,
			"state":     container.State,
			"cpu":       "0%",   // 暂不采集实时数据以优化性能
			"memory":    "0 MB", // 暂不采集实时数据以优化性能
			"networkRx": "0 B",
			"networkTx": "0 B",
		}
		containerList = append(containerList, containerInfo)
	}

	c.JSON(http.StatusOK, gin.H{
		"containers": containerList,
		"isSelf":     isSelfProjectName(composeProjectName),
	})
}

type composeProjectLifecycleClient interface {
	ContainerList(context.Context, types.ContainerListOptions) ([]types.Container, error)
	ContainerStop(context.Context, string, dockercontainer.StopOptions) error
	ContainerRestart(context.Context, string, dockercontainer.StopOptions) error
}

func listComposeProjectContainers(ctx context.Context, cli interface {
	ContainerList(context.Context, types.ContainerListOptions) ([]types.Container, error)
}, projectName string) ([]types.Container, error) {
	if !isExactComposeProjectName(projectName) {
		return nil, errors.New("Compose 项目身份不合法")
	}
	containers, err := cli.ContainerList(ctx, types.ContainerListOptions{
		All: true,
		Filters: filters.NewArgs(
			filters.Arg("label", "com.docker.compose.project="+projectName),
		),
	})
	if err != nil {
		return nil, fmt.Errorf("枚举项目容器失败: %w", err)
	}
	return containers, nil
}

func composeContainerDisplayName(container types.Container) string {
	if len(container.Names) > 0 && strings.TrimSpace(container.Names[0]) != "" {
		return strings.TrimPrefix(strings.TrimSpace(container.Names[0]), "/")
	}
	return strings.TrimSpace(container.ID)
}

func composeContainerIsLive(state string) bool {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "running", "restarting", "paused":
		return true
	default:
		return false
	}
}

// stopComposeProjectContainers uses the runtime project label as the source of
// truth. This remains correct when a historical on-disk Compose file has stale
// service names. A second observation prevents an empty Docker API success
// from being reported while a target container is still live.
func stopComposeProjectContainers(ctx context.Context, cli composeProjectLifecycleClient, projectName string, timeoutSeconds int) (int, error) {
	containers, err := listComposeProjectContainers(ctx, cli, projectName)
	if err != nil {
		return 0, err
	}
	stopped := 0
	failures := make([]string, 0)
	for _, container := range containers {
		if !composeContainerIsLive(container.State) {
			continue
		}
		timeout := timeoutSeconds
		if err := cli.ContainerStop(ctx, container.ID, dockercontainer.StopOptions{Timeout: &timeout}); err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", composeContainerDisplayName(container), err))
			continue
		}
		stopped++
	}
	observed, observeErr := listComposeProjectContainers(ctx, cli, projectName)
	if observeErr != nil {
		failures = append(failures, observeErr.Error())
	} else {
		for _, container := range observed {
			if composeContainerIsLive(container.State) {
				failures = append(failures, composeContainerDisplayName(container)+" 仍在运行")
			}
		}
	}
	if len(failures) > 0 {
		return stopped, fmt.Errorf("项目停止未完全生效: %s", strings.Join(failures, "; "))
	}
	return stopped, nil
}

// restartComposeProjectContainers restarts the containers observed from the
// runtime project label and refuses the compose-cli no-op case explicitly.
func restartComposeProjectContainers(ctx context.Context, cli composeProjectLifecycleClient, projectName string, timeoutSeconds int) (int, error) {
	containers, err := listComposeProjectContainers(ctx, cli, projectName)
	if err != nil {
		return 0, err
	}
	if len(containers) == 0 {
		return 0, errors.New("未找到该项目的运行态容器，请先启动项目")
	}
	restarted := 0
	failures := make([]string, 0)
	for _, container := range containers {
		state := strings.ToLower(strings.TrimSpace(container.State))
		if state == "dead" || state == "removing" {
			failures = append(failures, composeContainerDisplayName(container)+" 当前不可重启")
			continue
		}
		timeout := timeoutSeconds
		if err := cli.ContainerRestart(ctx, container.ID, dockercontainer.StopOptions{Timeout: &timeout}); err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", composeContainerDisplayName(container), err))
			continue
		}
		restarted++
	}
	observed, observeErr := listComposeProjectContainers(ctx, cli, projectName)
	if observeErr != nil {
		failures = append(failures, observeErr.Error())
	} else {
		live := 0
		for _, container := range observed {
			if composeContainerIsLive(container.State) {
				live++
			}
		}
		if live != restarted {
			failures = append(failures, fmt.Sprintf("重启后仅 %d/%d 个容器处于运行态", live, restarted))
		}
	}
	if len(failures) > 0 {
		return restarted, fmt.Errorf("部分项目容器重启失败: %s", strings.Join(failures, "; "))
	}
	return restarted, nil
}

// composeProjectKillClient 是项目级强制停止所需的最小 Docker 客户端能力。
type composeProjectKillClient interface {
	ContainerList(ctx context.Context, options types.ContainerListOptions) ([]types.Container, error)
	ContainerStop(ctx context.Context, containerID string, options dockercontainer.StopOptions) error
}

// killComposeProjectContainers 按 com.docker.compose.project 标签枚举项目容器，
// 对存活容器逐个以 0 宽限期停止（立即 SIGKILL，且按手动停止处理，重启策略不会再次拉起）。
// 单个容器失败不中断整体流程，最终汇总返回错误；返回值为成功终止的容器数。
func killComposeProjectContainers(ctx context.Context, cli composeProjectKillClient, projectName string) (int, error) {
	containers, err := cli.ContainerList(ctx, types.ContainerListOptions{
		All: true,
		Filters: filters.NewArgs(
			filters.Arg("label", "com.docker.compose.project="+projectName),
		),
	})
	if err != nil {
		return 0, fmt.Errorf("枚举项目容器失败: %w", err)
	}

	killed := 0
	var failures []string
	for _, ctr := range containers {
		switch strings.ToLower(strings.TrimSpace(ctr.State)) {
		case "running", "restarting", "paused":
			// 这些状态仍可能持有活动进程，强制停止时需要一并处理。
		default:
			continue
		}
		timeout := 0
		if err := cli.ContainerStop(ctx, ctr.ID, dockercontainer.StopOptions{Timeout: &timeout}); err != nil {
			name := ctr.ID
			if len(ctr.Names) > 0 {
				name = strings.TrimPrefix(ctr.Names[0], "/")
			}
			log.Printf("[WARN] Failed to kill container %s (%s): %v\n", ctr.ID, name, err)
			failures = append(failures, fmt.Sprintf("%s: %v", name, err))
			continue
		}
		killed++
	}
	if len(failures) > 0 {
		return killed, fmt.Errorf("部分容器强制停止失败: %s", strings.Join(failures, "; "))
	}
	return killed, nil
}

// cleanProjectResources 清理项目的容器和网络资源
// 1. 尝试使用 docker compose down
// 2. 扫描并强制删除所有带有 com.docker.compose.project=name 标签的残留容器
// 3. 清理关联网络
// 返回值：error 为阻断性错误；warning 为非阻断性提示（例如 compose down 失败但已用兜底逻辑清理），
// 调用方可选择性地在响应中展示给用户。
func cleanProjectResources(name string) (error, string) {
	target, err := resolveComposeOperationTarget(context.Background(), name)
	if err != nil {
		return err, ""
	}
	return cleanProjectResourcesTargetContext(context.Background(), target.ProjectDir, target.ComposeProjectName)
}

func cleanProjectResourcesTarget(projectDir, composeProjectName string) (error, string) {
	return cleanProjectResourcesTargetContext(context.Background(), projectDir, composeProjectName)
}

func cleanProjectResourcesTargetContext(parent context.Context, projectDir, composeProjectName string) (error, string) {
	if strings.TrimSpace(projectDir) == "" || !isExactComposeProjectName(composeProjectName) {
		return fmt.Errorf("Compose 项目身份不完整"), ""
	}
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, 3*time.Minute)
	defer cancel()

	var warning string

	// 1. 尝试使用 docker compose down 命令停止并删除容器
	if _, err := os.Stat(projectDir); err == nil {
		args := composeCommandWithProjectName(composeProjectName, []string{"compose", "down"})
		downCtx, downCancel := context.WithTimeout(ctx, time.Minute)
		output, downErr := runDockerCombinedOutput(downCtx, projectDir, nil, args)
		downCancel()
		if downErr != nil {
			// 仅打印日志，不中断流程（后续仍有手动 ContainerRemove 兜底）
			log.Printf("[WARN] docker compose down failed for %s: %v\nOutput: %s\n", composeProjectName, downErr, string(output))
			warning = fmt.Sprintf("docker compose down 执行失败（已尝试兜底清理），可能残留部分网络资源: %v", downErr)
		}
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("清理项目资源超时或已取消: %w", err), warning
	}

	// 2. 使用 Docker SDK 手动清理残留容器
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("Failed to create docker client: %v", err), warning
	}
	defer cli.Close()

	// 查找属于该项目的所有容器
	listCtx, listCancel := context.WithTimeout(ctx, 15*time.Second)
	containers, err := cli.ContainerList(listCtx, types.ContainerListOptions{
		All: true,
		Filters: filters.NewArgs(
			filters.Arg("label", "com.docker.compose.project="+composeProjectName),
		),
	})
	listCancel()

	if err != nil {
		return fmt.Errorf("枚举项目容器失败: %w", err), warning
	}
	var cleanupFailures []string
	hostPortDedup := make(map[int]struct{})
	for _, container := range containers {
		// 强制删除容器 (Force=true 会先停止容器)
		removeOpts := types.ContainerRemoveOptions{
			Force:         true,
			RemoveVolumes: true,
		}
		removeCtx, removeCancel := context.WithTimeout(ctx, 15*time.Second)
		removeErr := cli.ContainerRemove(removeCtx, container.ID, removeOpts)
		removeCancel()
		if removeErr != nil {
			log.Printf("[WARN] Failed to force remove container %s (%s): %v\n", container.ID, container.Names, removeErr)
			cleanupFailures = append(cleanupFailures, fmt.Sprintf("删除容器 %s 失败: %v", composeContainerDisplayName(container), removeErr))
			continue
		}
		log.Printf("[INFO] Successfully removed container %s (%s)\n", container.ID, container.Names)
		for _, p := range container.Ports {
			if p.PublicPort > 0 {
				hostPortDedup[int(p.PublicPort)] = struct{}{}
			}
		}
	}

	if len(hostPortDedup) > 0 {
		var hostPorts []int
		for p := range hostPortDedup {
			hostPorts = append(hostPorts, p)
		}
		if tx, txErr := database.GetDB().Begin(); txErr == nil {
			if derr := database.DeleteReservedPortsByPortsTx(tx, hostPorts); derr != nil {
				_ = tx.Rollback()
			} else {
				_ = tx.Commit()
			}
		}
	}

	// 3. 清理关联网络
	// 3.1 根据标签 com.docker.compose.project 清理
	if cli != nil {
		networkListCtx, networkListCancel := context.WithTimeout(ctx, 15*time.Second)
		nets, nerr := cli.NetworkList(networkListCtx, types.NetworkListOptions{
			Filters: filters.NewArgs(filters.Arg("label", "com.docker.compose.project="+composeProjectName)),
		})
		networkListCancel()
		if nerr == nil {
			for _, net := range nets {
				networkCtx, networkCancel := context.WithTimeout(ctx, 15*time.Second)
				networkErr := cli.NetworkRemove(networkCtx, net.ID)
				networkCancel()
				if networkErr != nil {
					cleanupFailures = append(cleanupFailures, fmt.Sprintf("删除网络 %s 失败: %v", net.Name, networkErr))
				}
			}
		} else {
			cleanupFailures = append(cleanupFailures, fmt.Sprintf("枚举项目网络失败: %v", nerr))
		}
	}
	if len(cleanupFailures) > 0 {
		return fmt.Errorf("项目资源未完全清理: %s", strings.Join(cleanupFailures, "; ")), warning
	}
	return nil, warning
}

// downProject 停止并移除项目容器和网络，但保留目录
func downProject(c *gin.Context) {
	target, ok := resolveComposeRequestTarget(c)
	if !ok {
		return
	}

	if err, warning := cleanProjectResourcesTargetContext(c.Request.Context(), target.ProjectDir, target.ComposeProjectName); err != nil {
		respondError(c, http.StatusInternalServerError, "清理项目资源失败", err)
		return
	} else if warning != "" {
		log.Printf("[WARN] 清理项目 %s 时: %s\n", target.DisplayName, warning)
	}

	invalidateComposeProjectListCache()
	c.JSON(http.StatusOK, gin.H{"message": "项目容器和网络已清理"})
}

// removeProject 删除项目
// 功能：
// 1. 尝试使用 docker compose down 停止并删除容器
// 2. 扫描并强制删除所有带有 com.docker.compose.project=name 标签的残留容器
// 3. 删除项目文件目录
func removeProject(c *gin.Context) {
	target, ok := resolveComposeRequestTarget(c)
	if !ok {
		return
	}

	// 清理资源
	if err, warning := cleanProjectResourcesTargetContext(c.Request.Context(), target.ProjectDir, target.ComposeProjectName); err != nil {
		respondError(c, http.StatusInternalServerError, "清理项目资源失败", err)
		return
	} else if warning != "" {
		log.Printf("[WARN] 删除项目 %s 时: %s\n", target.DisplayName, warning)
	}

	if tx, err := database.GetDB().Begin(); err == nil {
		if derr := database.DeleteReservedPortsByOwnerTx(tx, target.ComposeProjectName); derr != nil {
			_ = tx.Rollback()
		} else {
			_ = tx.Commit()
		}
	}

	// 4. 删除项目目录
	if err := os.RemoveAll(target.ProjectDir); err != nil {
		respondError(c, http.StatusInternalServerError, "删除项目目录失败", err)
		return
	}
	_ = database.DeleteComposeGitSourceInEnvironment(database.LocalEnvironmentID, target.DisplayName)
	if err := database.DeleteComposeProjectMetadata(database.LocalEnvironmentID, target.DisplayName); err != nil {
		logging.Warn("Compose metadata cleanup after project removal failed", "project", target.DisplayName, "error", err)
	}
	if err := database.DeleteComposeHistoryForProject(database.LocalEnvironmentID, target.DisplayName); err != nil {
		logging.Warn("Compose history cleanup after project removal failed", "project", target.DisplayName, "error", err)
	}
	_ = database.DeleteComposeProjectIdentity(database.LocalEnvironmentID, target.RelativePath)

	invalidateComposeProjectListCache()
	c.JSON(http.StatusOK, gin.H{"message": "项目已删除"})
}

// 添加获取 compose 日志的处理函数
func buildComposeLogArgs(tail string) []string {
	if strings.TrimSpace(tail) == "" {
		tail = "200"
	}
	return []string{"compose", "logs", "-f", "--timestamps", "--tail", tail}
}

func getComposeLogs(c *gin.Context) {
	composeProjectName, ok := resolveComposeDockerProjectName(c)
	if !ok {
		return
	}
	tail := strings.TrimSpace(c.DefaultQuery("tail", "200"))
	streamComposeLogsSSE(c, composeProjectName, tail)
}

// 添加获取 YAML 配置的处理函数
// getProjectYaml 获取项目 YAML 配置
func getProjectYaml(c *gin.Context) {
	target, ok := resolveComposeRequestTarget(c)
	if !ok {
		return
	}
	yamlPath, err := findComposeFile(target.ProjectDir)
	if err != nil {
		if os.IsNotExist(err) {
			respondError(c, http.StatusBadRequest, "未找到可用的 compose 配置文件，支持: *.yaml, *.yml, docker-compose.yaml, docker-compose.yml", nil)
			return
		}
		respondError(c, http.StatusInternalServerError, "扫描配置文件失败", err)
		return
	}

	// 读取 YAML 文件
	content, err := os.ReadFile(yamlPath)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "读取配置文件失败", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"content": string(content),
	})
}

// getProjectEnv 获取项目 .env 内容
func getProjectEnv(c *gin.Context) {
	target, ok := resolveComposeRequestTarget(c)
	if !ok {
		return
	}
	envPath := filepath.Join(target.ProjectDir, ".env")
	content, err := os.ReadFile(envPath)
	if err != nil {
		if os.IsNotExist(err) {
			c.JSON(http.StatusOK, gin.H{"content": ""})
			return
		}
		respondError(c, http.StatusInternalServerError, "读取 .env 文件失败", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"content": string(content)})
}

// saveProjectEnv 保存项目 .env 内容
func saveProjectEnv(c *gin.Context) {
	target, ok := resolveComposeRequestTarget(c)
	if !ok {
		return
	}

	var data struct {
		Content string `json:"content"`
	}
	if err := c.BindJSON(&data); err != nil {
		respondError(c, http.StatusBadRequest, "无效的请求数据", err)
		return
	}
	if _, err := os.Stat(target.ProjectDir); err != nil {
		respondError(c, http.StatusBadRequest, "项目目录不存在", err)
		return
	}

	envPath := filepath.Join(target.ProjectDir, ".env")
	content := strings.ReplaceAll(data.Content, "\r\n", "\n")
	if err := os.WriteFile(envPath, []byte(content), 0600); err != nil {
		respondError(c, http.StatusInternalServerError, "保存 .env 文件失败", err)
		return
	}

	invalidateComposeProjectListCache()
	c.JSON(http.StatusOK, gin.H{"message": ".env 已保存"})
}

// 添加保存 YAML 配置的处理函数
func saveProjectYaml(c *gin.Context) {
	target, ok := resolveComposeRequestTarget(c)
	if !ok {
		return
	}
	var data struct {
		Content string `json:"content"`
	}

	if err := c.BindJSON(&data); err != nil {
		respondError(c, http.StatusBadRequest, "无效的请求数据", err)
		return
	}
	if !composeYAMLHasServices([]byte(data.Content)) {
		respondError(c, http.StatusBadRequest, "Compose YAML 根节点必须包含非空 services 映射", nil)
		return
	}

	yamlPath, err := findComposeFile(target.ProjectDir)
	if err != nil {
		if os.IsNotExist(err) {
			// 如果不存在任何 YAML 文件，则默认写入 docker-compose.yml
			yamlPath = filepath.Join(target.ProjectDir, "docker-compose.yml")
		} else {
			respondError(c, http.StatusInternalServerError, "扫描配置文件失败", err)
			return
		}
	}

	// 保存 YAML 文件
	if err := os.WriteFile(yamlPath, []byte(data.Content), 0644); err != nil {
		respondError(c, http.StatusInternalServerError, "保存配置文件失败", err)
		return
	}

	invalidateComposeProjectListCache()
	c.JSON(http.StatusOK, gin.H{"message": "配置已保存"})
}

// 移除底部重复的 RegisterComposeRoutes
