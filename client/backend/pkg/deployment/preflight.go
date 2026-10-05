package deployment

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"

	"dockerpanel/backend/pkg/secrets"

	"github.com/docker/go-connections/nat"
	"gopkg.in/yaml.v3"
)

const defaultRequiredDiskBytes = uint64(512 << 20)

type DockerRuntimeInfo struct {
	APIVersion   string `json:"apiVersion"`
	Architecture string `json:"architecture"`
}

type PathProbeResult struct {
	Exists    bool `json:"exists"`
	Directory bool `json:"directory"`
	Writable  bool `json:"writable"`
}

type PreflightDependencies struct {
	DockerInfo     func(context.Context) (DockerRuntimeInfo, error)
	PathStatus     func(string) (PathProbeResult, error)
	DiskAvailable  func(string) (uint64, error)
	PortAvailable  func(context.Context, int, string) (bool, error)
	NetworkExists  func(context.Context, string) (bool, error)
	ImageReachable func(context.Context, string) (bool, error)
	DeviceExists   func(string) bool
	ComposeConfig  func(context.Context, Intent, string) error
	// ResolveSymlinks 解析符号链接链（默认 filepath.EvalSymlinks）；测试
	// 注入替身。所有 Compose 源路径（env_file/build context/Dockerfile）
	// 在词法校验后必须再经真实路径解析，防止链接逃逸。
	ResolveSymlinks func(string) (string, error)
	// Controller-validated auxiliary file candidates, never model-supplied
	// options. They will be materialized atomically before execution.
	PlannedConfigFiles map[string]bool
}

// checkPreflightPathWithinRoot performs the lexical boundary check and then
// resolves symlinks (nearest existing ancestor for not-yet-created paths) and
// re-checks the resolved real path against the same root.
func checkPreflightPathWithinRoot(deps *PreflightDependencies, sourceDir, path string) error {
	if err := LexicalPathWithinRoot(sourceDir, path); err != nil {
		return err
	}
	resolve := filepath.EvalSymlinks
	if deps != nil && deps.ResolveSymlinks != nil {
		resolve = deps.ResolveSymlinks
	}
	resolved, err := resolveNearestExistingWith(resolve, path)
	if err != nil {
		return err
	}
	resolvedRoot, err := resolveNearestExistingWith(resolve, sourceDir)
	if err != nil {
		return err
	}
	return LexicalPathWithinRoot(resolvedRoot, resolved)
}

func resolveNearestExistingWith(resolve func(string) (string, error), path string) (string, error) {
	path = filepath.Clean(path)
	if _, err := os.Lstat(path); err == nil {
		if real, evalErr := resolve(path); evalErr == nil {
			return real, nil
		}
	}
	dir := path
	tail := ""
	for {
		if _, err := os.Lstat(dir); err == nil {
			real, evalErr := resolve(dir)
			if evalErr != nil {
				return "", evalErr
			}
			if tail == "" {
				return real, nil
			}
			return filepath.Clean(filepath.Join(real, tail)), nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return path, nil
		}
		if tail == "" {
			tail = filepath.Base(dir)
		} else {
			tail = filepath.Join(filepath.Base(dir), tail)
		}
		dir = parent
	}
}

type composePort struct {
	Published int
	Protocol  string
}

func Preflight(ctx context.Context, intent Intent, profile NASProfile, dependencies PreflightDependencies) PreflightResult {
	deps := withDefaultPreflightDependencies(dependencies)
	result := PreflightResult{Issues: make([]PreflightIssue, 0)}
	add := func(code string, severity PreflightSeverity, retryable bool, actions []string, details map[string]any) {
		result.Issues = append(result.Issues, PreflightIssue{
			Code:        code,
			Severity:    severity,
			Retryable:   retryable,
			NextActions: normalizeNextActions(actions),
			Details:     details,
		})
	}

	profile = normalizeNASProfile(profile)
	if err := validateNASProfile(profile); err != nil {
		add("profile_invalid", PreflightBlocking, false, []string{"update_nas_profile"}, map[string]any{"reason": err.Error()})
	}

	checkDockerRuntime(ctx, profile, deps, add)
	checkProjectStorage(intent, profile, deps, add)

	document, services, ok := parsePreflightCompose(intent.ComposeYAML)
	if !ok {
		add("compose_invalid", PreflightBlocking, false, []string{"fix_compose_yaml"}, nil)
		return result
	}
	if len(services) == 0 {
		add("compose_services_missing", PreflightBlocking, false, []string{"add_compose_service"}, nil)
		return result
	}

	sourceDir := preflightSourceDir(intent, profile)
	if err := deps.ComposeConfig(ctx, intent, sourceDir); err != nil {
		text := err.Error()
		var sensitive []string
		for key, value := range intent.Parameters {
			if secret, ok := value.(string); ok && secret != "" && secrets.IsSensitiveKey(key) {
				sensitive = append(sensitive, secret)
			}
		}
		sort.Slice(sensitive, func(i, j int) bool { return len(sensitive[i]) > len(sensitive[j]) })
		for _, secret := range sensitive {
			text = strings.ReplaceAll(text, secret, secrets.RedactedValue)
		}
		diagnostic := []rune(secrets.RedactString(text))
		if len(diagnostic) > 512 {
			diagnostic = diagnostic[:512]
		}
		add("compose_config_invalid", PreflightBlocking, false, []string{"fix_compose_configuration"}, map[string]any{"diagnostic": string(diagnostic)})
	}
	checkRequiredEnvironment(intent, add)
	checkEnvFiles(sourceDir, services, intent.Parameters, deps, add)
	checkExternalNetworks(ctx, document, deps, add)
	checkComposeBuild(sourceDir, services, deps, add)
	checkComposePorts(ctx, services, profile, deps, add)
	checkComposeVolumes(services, profile, add)
	checkRequiredBindSources(sourceDir, services, profile, deps, add)
	checkComposeDevices(services, profile, deps, add)
	checkComposeImages(ctx, services, profile, deps, add)
	checkComposeNetworkPolicy(services, profile, add)

	return result
}

func checkRequiredBindSources(sourceDir string, services map[string]any, profile NASProfile, deps PreflightDependencies, add func(string, PreflightSeverity, bool, []string, map[string]any)) {
	for _, serviceName := range sortedMapKeys(services) {
		service, _ := stringMap(services[serviceName])
		for _, item := range asAnySlice(service["volumes"]) {
			source := ""
			required := false
			if text, ok := item.(string); ok {
				parts := strings.SplitN(text, ":", 2)
				if len(parts) == 2 && looksLikeBindSource(parts[0]) {
					source = parts[0]
				}
			} else {
				volume, _ := stringMap(item)
				if volume["type"] != "bind" {
					continue
				}
				source, _ = volume["source"].(string)
				bind, _ := stringMap(volume["bind"])
				if create, ok := bind["create_host_path"].(bool); ok && !create {
					required = true
				}
			}
			if source == "" {
				continue
			}
			path := source
			if !filepath.IsAbs(path) {
				path = filepath.Join(sourceDir, path)
			} else if rel, err := filepath.Rel(profile.ProjectRoot, path); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
				path = filepath.Join(profile.ContainerProjectRoot, rel)
			}
			path = filepath.Clean(path)
			knownFile := deps.PlannedConfigFiles[path]
			if !required && !knownFile {
				continue
			}
			details := map[string]any{"service": serviceName, "path": source}
			status, err := deps.PathStatus(path)
			if err != nil {
				add("bind_source_unreadable", PreflightBlocking, true, []string{"inspect_bind_path"}, details)
			} else if knownFile && status.Exists && status.Directory {
				add("config_file_type_conflict", PreflightBlocking, false, []string{"use_new_config_file_path"}, details)
			} else if !status.Exists && !knownFile {
				add("bind_source_missing", PreflightBlocking, true, []string{"materialize_config_file"}, details)
			}
		}
	}
}

func withDefaultPreflightDependencies(deps PreflightDependencies) PreflightDependencies {
	if deps.ResolveSymlinks == nil {
		deps.ResolveSymlinks = filepath.EvalSymlinks
	}
	if deps.PathStatus == nil {
		deps.PathStatus = defaultPathStatus
	}
	if deps.DiskAvailable == nil {
		deps.DiskAvailable = defaultDiskAvailable
	}
	if deps.PortAvailable == nil {
		deps.PortAvailable = func(context.Context, int, string) (bool, error) { return true, nil }
	}
	if deps.NetworkExists == nil {
		deps.NetworkExists = func(context.Context, string) (bool, error) { return true, nil }
	}
	if deps.ImageReachable == nil {
		deps.ImageReachable = func(context.Context, string) (bool, error) { return true, nil }
	}
	if deps.DeviceExists == nil {
		deps.DeviceExists = func(path string) bool {
			_, err := os.Stat(path)
			return err == nil
		}
	}
	if deps.ComposeConfig == nil {
		deps.ComposeConfig = func(context.Context, Intent, string) error { return nil }
	}
	return deps
}

func defaultPathStatus(path string) (PathProbeResult, error) {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return PathProbeResult{}, nil
	}
	if err != nil {
		return PathProbeResult{}, err
	}
	return PathProbeResult{
		Exists:    true,
		Directory: info.IsDir(),
		Writable:  info.Mode().Perm()&0o222 != 0,
	}, nil
}

func defaultDiskAvailable(path string) (uint64, error) {
	probePath := filepath.Clean(path)
	for {
		if _, err := os.Stat(probePath); err == nil {
			break
		}
		parent := filepath.Dir(probePath)
		if parent == probePath {
			return 0, fmt.Errorf("no existing parent for %s", path)
		}
		probePath = parent
	}
	var stat syscall.Statfs_t
	if err := syscall.Statfs(probePath, &stat); err != nil {
		return 0, err
	}
	return stat.Bavail * uint64(stat.Bsize), nil
}

func checkDockerRuntime(
	ctx context.Context,
	profile NASProfile,
	deps PreflightDependencies,
	add func(string, PreflightSeverity, bool, []string, map[string]any),
) {
	if deps.DockerInfo == nil {
		add("docker_unavailable", PreflightBlocking, true, []string{"check_docker_socket"}, nil)
		return
	}
	info, err := deps.DockerInfo(ctx)
	if err != nil {
		add("docker_unavailable", PreflightBlocking, true, []string{"check_docker_socket"}, nil)
		return
	}
	runtimeArch := normalizeArchitecture(info.Architecture)
	if runtimeArch != "" && profile.Architecture != "" && runtimeArch != profile.Architecture {
		add("architecture_mismatch", PreflightBlocking, false, []string{"select_compatible_image"}, map[string]any{
			"profile": profile.Architecture,
			"runtime": runtimeArch,
		})
	}
	add("docker_ready", PreflightInfo, false, nil, map[string]any{
		"apiVersion":   info.APIVersion,
		"architecture": runtimeArch,
	})
}

func checkProjectStorage(
	intent Intent,
	profile NASProfile,
	deps PreflightDependencies,
	add func(string, PreflightSeverity, bool, []string, map[string]any),
) {
	status, err := deps.PathStatus(profile.ContainerProjectRoot)
	if err != nil || !status.Exists || !status.Directory {
		add("project_root_unavailable", PreflightBlocking, true, []string{"create_project_root", "fix_project_root_mount"}, map[string]any{
			"path": profile.ContainerProjectRoot,
		})
	} else if !status.Writable {
		add("project_root_not_writable", PreflightBlocking, false, []string{"fix_project_root_permissions"}, map[string]any{
			"path": profile.ContainerProjectRoot,
		})
	}

	requiredBytes := requiredDiskBytes(intent)
	available, diskErr := deps.DiskAvailable(profile.ContainerProjectRoot)
	if diskErr != nil {
		add("disk_space_unknown", PreflightWarning, true, []string{"check_disk_space"}, nil)
	} else if available < requiredBytes {
		add("disk_space_low", PreflightBlocking, true, []string{"free_disk_space", "choose_another_project_root"}, map[string]any{
			"availableBytes": available,
			"requiredBytes":  requiredBytes,
		})
	}
}

func requiredDiskBytes(intent Intent) uint64 {
	value := intent.Options["requiredDiskBytes"]
	switch typed := value.(type) {
	case uint64:
		if typed > 0 {
			return typed
		}
	case int:
		if typed > 0 {
			return uint64(typed)
		}
	case int64:
		if typed > 0 {
			return uint64(typed)
		}
	case float64:
		if typed > 0 {
			return uint64(typed)
		}
	case string:
		if parsed, err := strconv.ParseUint(strings.TrimSpace(typed), 10, 64); err == nil && parsed > 0 {
			return parsed
		}
	}
	return defaultRequiredDiskBytes
}

func parsePreflightCompose(raw string) (map[string]any, map[string]any, bool) {
	document := make(map[string]any)
	if strings.TrimSpace(raw) == "" || yaml.Unmarshal([]byte(raw), &document) != nil {
		return nil, nil, false
	}
	services, ok := stringMap(document["services"])
	if !ok {
		return document, nil, true
	}
	return document, services, true
}

func stringMap(value any) (map[string]any, bool) {
	switch typed := value.(type) {
	case map[string]any:
		return typed, true
	case map[any]any:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			out[fmt.Sprint(key)] = item
		}
		return out, true
	default:
		return nil, false
	}
}

func preflightSourceDir(intent Intent, profile NASProfile) string {
	if path := cleanAbsolutePath(intent.BuildContext); path != "" {
		return path
	}
	if path := cleanAbsolutePath(stringOption(intent.Options, "sourceDir")); path != "" {
		return path
	}
	return filepath.Join(profile.ContainerProjectRoot, strings.TrimSpace(intent.ProjectName))
}

func stringOption(values map[string]any, key string) string {
	if values == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(values[key]))
}

var requiredEnvironmentPatterns = []*regexp.Regexp{
	regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*):\?[^}]*\}`),
	regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\?[^}]*\}`),
}

func checkRequiredEnvironment(
	intent Intent,
	add func(string, PreflightSeverity, bool, []string, map[string]any),
) {
	missing := make(map[string]struct{})
	raw := strings.ReplaceAll(intent.ComposeYAML, "$$", "")
	for _, pattern := range requiredEnvironmentPatterns {
		for _, match := range pattern.FindAllStringSubmatch(raw, -1) {
			name := match[1]
			if !hasIntentParameter(intent.Parameters, name) {
				missing[name] = struct{}{}
			}
		}
	}
	if len(missing) == 0 {
		return
	}
	names := make([]string, 0, len(missing))
	for name := range missing {
		names = append(names, name)
	}
	sort.Strings(names)
	add("required_env_missing", PreflightBlocking, false, []string{"provide_environment_values"}, map[string]any{"names": names})
}

func hasIntentParameter(parameters map[string]any, key string) bool {
	if parameters == nil {
		return false
	}
	value, exists := parameters[key]
	if !exists || value == nil {
		return false
	}
	return strings.TrimSpace(fmt.Sprint(value)) != ""
}

func checkEnvFiles(
	sourceDir string,
	services map[string]any,
	parameters map[string]any,
	deps PreflightDependencies,
	add func(string, PreflightSeverity, bool, []string, map[string]any),
) {
	interpolationValues := make(map[string]string, len(parameters))
	for key, value := range parameters {
		if value != nil {
			interpolationValues[key] = fmt.Sprint(value)
		}
	}
	missing := make([]string, 0)
	for _, serviceName := range sortedMapKeys(services) {
		service, _ := stringMap(services[serviceName])
		if service == nil {
			continue
		}
		for _, envFile := range composeEnvFiles(service["env_file"]) {
			if !envFile.Required {
				continue
			}
			path, err := expandComposePortValue(envFile.Path, interpolationValues)
			if err != nil || strings.TrimSpace(path) == "" {
				details := map[string]any{"path": envFile.Path, "service": serviceName}
				if err != nil {
					details["reason"] = err.Error()
				}
				add("env_file_interpolation_invalid", PreflightBlocking, false, []string{"provide_environment_values", "fix_env_file_path"}, details)
				continue
			}
			if !filepath.IsAbs(path) {
				path = filepath.Join(sourceDir, path)
			}
			path = filepath.Clean(path)
			if err := checkPreflightPathWithinRoot(&deps, sourceDir, path); err != nil {
				add("env_file_path_escape", PreflightBlocking, false, []string{"fix_env_file_path", "remove_env_file"}, map[string]any{
					"path": path, "service": serviceName, "reason": err.Error(),
				})
				continue
			}
			status, err := deps.PathStatus(path)
			if err != nil || !status.Exists || status.Directory {
				missing = append(missing, path)
			}
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		add("env_file_missing", PreflightBlocking, false, []string{"provide_env_file", "remove_env_file"}, map[string]any{"paths": missing})
	}
}

type composeEnvFile struct {
	Path     string
	Required bool
}

func composeEnvFiles(value any) []composeEnvFile {
	items := asAnySlice(value)
	if len(items) == 0 && value != nil {
		items = []any{value}
	}
	out := make([]composeEnvFile, 0, len(items))
	for _, item := range items {
		switch typed := item.(type) {
		case string:
			if path := strings.TrimSpace(typed); path != "" {
				out = append(out, composeEnvFile{Path: path, Required: true})
			}
		default:
			mapped, _ := stringMap(item)
			path := strings.TrimSpace(fmt.Sprint(mapped["path"]))
			if path == "" || path == "<nil>" {
				continue
			}
			required := true
			if raw, exists := mapped["required"]; exists {
				required = boolValue(raw, true)
			}
			out = append(out, composeEnvFile{Path: path, Required: required})
		}
	}
	return out
}

func checkExternalNetworks(
	ctx context.Context,
	document map[string]any,
	deps PreflightDependencies,
	add func(string, PreflightSeverity, bool, []string, map[string]any),
) {
	networks, _ := stringMap(document["networks"])
	for _, key := range sortedMapKeys(networks) {
		network, _ := stringMap(networks[key])
		if network == nil || !boolValue(network["external"], false) {
			continue
		}
		name := strings.TrimSpace(fmt.Sprint(network["name"]))
		if name == "" || name == "<nil>" {
			name = key
		}
		exists, err := deps.NetworkExists(ctx, name)
		if err != nil || !exists {
			add("external_network_missing", PreflightBlocking, true, []string{"create_network", "change_network"}, map[string]any{"name": name})
		}
	}
}

func checkComposeBuild(
	sourceDir string,
	services map[string]any,
	deps PreflightDependencies,
	add func(string, PreflightSeverity, bool, []string, map[string]any),
) {
	for _, serviceName := range sortedMapKeys(services) {
		service, _ := stringMap(services[serviceName])
		build, exists := service["build"]
		if service == nil || !exists {
			continue
		}
		contextPath, dockerfile := composeBuildPaths(build)
		if contextPath == "" {
			contextPath = "."
		}
		if dockerfile == "" {
			dockerfile = "Dockerfile"
		}
		buildDir := contextPath
		if !filepath.IsAbs(buildDir) {
			buildDir = filepath.Join(sourceDir, buildDir)
		}
		buildDir = filepath.Clean(buildDir)
		if err := checkPreflightPathWithinRoot(&deps, sourceDir, buildDir); err != nil {
			add("build_context_path_escape", PreflightBlocking, false, []string{"fetch_project_source", "fix_build_context"}, map[string]any{
				"service": serviceName, "path": buildDir, "reason": err.Error(),
			})
			continue
		}
		status, err := deps.PathStatus(buildDir)
		if err != nil || !status.Exists || !status.Directory {
			add("build_context_missing", PreflightBlocking, false, []string{"fetch_project_source", "fix_build_context"}, map[string]any{
				"service": serviceName,
				"path":    buildDir,
			})
			continue
		}
		dockerfilePath := dockerfile
		if !filepath.IsAbs(dockerfilePath) {
			dockerfilePath = filepath.Join(buildDir, dockerfilePath)
		}
		dockerfilePath = filepath.Clean(dockerfilePath)
		if err := checkPreflightPathWithinRoot(&deps, sourceDir, dockerfilePath); err != nil {
			add("dockerfile_path_escape", PreflightBlocking, false, []string{"fetch_project_source", "fix_dockerfile_path"}, map[string]any{
				"service": serviceName, "path": dockerfilePath, "reason": err.Error(),
			})
			continue
		}
		dockerfileStatus, dockerfileErr := deps.PathStatus(dockerfilePath)
		if dockerfileErr != nil || !dockerfileStatus.Exists || dockerfileStatus.Directory {
			add("dockerfile_missing", PreflightBlocking, false, []string{"fetch_project_source", "fix_dockerfile_path"}, map[string]any{
				"service": serviceName,
				"path":    dockerfilePath,
			})
		}
	}
}

func checkComposePorts(
	ctx context.Context,
	services map[string]any,
	profile NASProfile,
	deps PreflightDependencies,
	add func(string, PreflightSeverity, bool, []string, map[string]any),
) {
	for _, serviceName := range sortedMapKeys(services) {
		service, _ := stringMap(services[serviceName])
		if service == nil || strings.EqualFold(strings.TrimSpace(fmt.Sprint(service["network_mode"])), "host") {
			continue
		}
		for _, port := range composePublishedPorts(service["ports"]) {
			available, err := deps.PortAvailable(ctx, port.Published, port.Protocol)
			if err == nil && available {
				continue
			}
			severity := PreflightBlocking
			actions := []string{"change_published_port"}
			if profile.PortRange.AutoAllocate {
				severity = PreflightWarning
				actions = []string{"auto_allocate_port"}
			}
			add("port_in_use", severity, true, actions, map[string]any{
				"service":  serviceName,
				"port":     port.Published,
				"protocol": port.Protocol,
			})
		}
	}

	if !profile.PortRange.AutoAllocate {
		return
	}
	available := false
	for port := profile.PortRange.Start; port <= profile.PortRange.End; port++ {
		free, err := deps.PortAvailable(ctx, port, "tcp")
		if err == nil && free {
			available = true
			break
		}
	}
	if !available {
		add("port_pool_exhausted", PreflightBlocking, true, []string{"expand_port_range", "free_port"}, map[string]any{
			"start": profile.PortRange.Start,
			"end":   profile.PortRange.End,
		})
	}
}

func composePublishedPorts(value any) []composePort {
	items := asAnySlice(value)
	out := make([]composePort, 0, len(items))
	for _, item := range items {
		switch typed := item.(type) {
		case string:
			mappings, err := nat.ParsePortSpec(strings.TrimSpace(typed))
			if err != nil {
				continue
			}
			for _, mapping := range mappings {
				published, err := strconv.Atoi(mapping.Binding.HostPort)
				if err == nil && published > 0 {
					out = append(out, composePort{Published: published, Protocol: mapping.Port.Proto()})
				}
			}
		default:
			mapped, _ := stringMap(item)
			published, _ := strconv.Atoi(strings.TrimSpace(fmt.Sprint(mapped["published"])))
			if published <= 0 {
				continue
			}
			protocol := strings.ToLower(strings.TrimSpace(fmt.Sprint(mapped["protocol"])))
			if protocol == "" || protocol == "<nil>" {
				protocol = "tcp"
			}
			out = append(out, composePort{Published: published, Protocol: protocol})
		}
	}
	return out
}

func checkComposeVolumes(
	services map[string]any,
	profile NASProfile,
	add func(string, PreflightSeverity, bool, []string, map[string]any),
) {
	for _, serviceName := range sortedMapKeys(services) {
		service, _ := stringMap(services[serviceName])
		for _, source := range composeBindSources(service["volumes"]) {
			if !filepath.IsAbs(source) || pathAllowedByRoots(source, profile.AllowedBindRoots) {
				continue
			}
			add("bind_root_not_allowed", PreflightBlocking, false, []string{"confirm_bind_path", "map_to_project_directory"}, map[string]any{
				"service": serviceName,
				"path":    filepath.Clean(source),
			})
		}
	}
}

func composeBindSources(value any) []string {
	out := make([]string, 0)
	for _, item := range asAnySlice(value) {
		switch typed := item.(type) {
		case string:
			parts := strings.SplitN(strings.TrimSpace(typed), ":", 2)
			if len(parts) < 2 || !looksLikeBindSource(parts[0]) {
				continue
			}
			out = append(out, strings.TrimSpace(parts[0]))
		default:
			mapped, _ := stringMap(item)
			if !strings.EqualFold(strings.TrimSpace(fmt.Sprint(mapped["type"])), "bind") {
				continue
			}
			source := strings.TrimSpace(fmt.Sprint(mapped["source"]))
			if source != "" && source != "<nil>" {
				out = append(out, source)
			}
		}
	}
	return out
}

func looksLikeBindSource(source string) bool {
	source = strings.TrimSpace(source)
	return filepath.IsAbs(source) || strings.HasPrefix(source, ".") || strings.HasPrefix(source, "~")
}

func pathAllowedByRoots(path string, roots []string) bool {
	for _, root := range roots {
		if _, err := ResolveWithinRoot(root, path); err == nil {
			return true
		}
	}
	return false
}

func checkComposeDevices(
	services map[string]any,
	profile NASProfile,
	deps PreflightDependencies,
	add func(string, PreflightSeverity, bool, []string, map[string]any),
) {
	for _, serviceName := range sortedMapKeys(services) {
		service, _ := stringMap(services[serviceName])
		for _, source := range composeDeviceSources(service["devices"]) {
			clean := filepath.Clean(source)
			if !pathAllowedByRoots(clean, profile.Devices) {
				add("device_not_allowed", PreflightBlocking, false, []string{"update_nas_profile", "remove_device"}, map[string]any{
					"service": serviceName,
					"path":    clean,
				})
			}
			if !deps.DeviceExists(clean) {
				add("device_missing", PreflightBlocking, false, []string{"attach_device", "remove_device"}, map[string]any{
					"service": serviceName,
					"path":    clean,
				})
			}
		}
	}
}

func composeDeviceSources(value any) []string {
	out := make([]string, 0)
	for _, item := range asAnySlice(value) {
		switch typed := item.(type) {
		case string:
			parts := strings.SplitN(strings.TrimSpace(typed), ":", 2)
			if source := strings.TrimSpace(parts[0]); source != "" {
				out = append(out, source)
			}
		default:
			mapped, _ := stringMap(item)
			source := strings.TrimSpace(fmt.Sprint(mapped["source"]))
			if source != "" && source != "<nil>" {
				out = append(out, source)
			}
		}
	}
	return out
}

func checkComposeImages(
	ctx context.Context,
	services map[string]any,
	profile NASProfile,
	deps PreflightDependencies,
	add func(string, PreflightSeverity, bool, []string, map[string]any),
) {
	for _, serviceName := range sortedMapKeys(services) {
		service, _ := stringMap(services[serviceName])
		if service == nil {
			continue
		}
		platform := normalizeArchitecture(strings.TrimPrefix(strings.TrimSpace(fmt.Sprint(service["platform"])), "linux/"))
		if platform != "" && platform != "<nil>" && profile.Architecture != "" && platform != profile.Architecture {
			add("architecture_mismatch", PreflightBlocking, false, []string{"select_compatible_image"}, map[string]any{
				"service":       serviceName,
				"profile":       profile.Architecture,
				"imagePlatform": platform,
			})
		}
		image := strings.TrimSpace(fmt.Sprint(service["image"]))
		if image == "" || image == "<nil>" {
			continue
		}
		reachable, err := deps.ImageReachable(ctx, image)
		if err != nil || !reachable {
			add("image_unreachable", PreflightWarning, true, []string{"retry_registry", "check_dns", "configure_registry"}, map[string]any{
				"service": serviceName,
				"image":   image,
			})
		}
	}
}

func checkComposeNetworkPolicy(
	services map[string]any,
	profile NASProfile,
	add func(string, PreflightSeverity, bool, []string, map[string]any),
) {
	if profile.NetworkPolicy.AllowHost {
		return
	}
	for _, serviceName := range sortedMapKeys(services) {
		service, _ := stringMap(services[serviceName])
		if strings.EqualFold(strings.TrimSpace(fmt.Sprint(service["network_mode"])), "host") {
			add("host_network_not_allowed", PreflightBlocking, false, []string{"allow_host_network", "use_bridge_network"}, map[string]any{
				"service": serviceName,
			})
		}
	}
}

func composeBuildPaths(value any) (string, string) {
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text), ""
	}
	mapped, _ := stringMap(value)
	if mapped == nil {
		return "", ""
	}
	contextPath := strings.TrimSpace(fmt.Sprint(mapped["context"]))
	dockerfile := strings.TrimSpace(fmt.Sprint(mapped["dockerfile"]))
	if contextPath == "<nil>" {
		contextPath = ""
	}
	if dockerfile == "<nil>" {
		dockerfile = ""
	}
	return contextPath, dockerfile
}

func asAnySlice(value any) []any {
	switch typed := value.(type) {
	case []any:
		return typed
	case []string:
		out := make([]any, len(typed))
		for index, item := range typed {
			out[index] = item
		}
		return out
	default:
		return nil
	}
}

func boolValue(value any, fallback bool) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		parsed, err := strconv.ParseBool(strings.TrimSpace(typed))
		if err == nil {
			return parsed
		}
	}
	return fallback
}

func sortedMapKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
