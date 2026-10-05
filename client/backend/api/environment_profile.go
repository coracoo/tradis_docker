package api

import (
	"context"
	"database/sql"
	"dockerpanel/backend/pkg/database"
	"dockerpanel/backend/pkg/deployment"
	tradisdocker "dockerpanel/backend/pkg/docker"
	"dockerpanel/backend/pkg/settings"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"github.com/docker/docker/api/types"
)

var errInvalidLocalEnvironmentProfile = errors.New("invalid local environment profile")

func resolveEnvironmentProfile(environmentID string, appOverrides deployment.ProfileOverrides) (deployment.NASProfile, error) {
	if _, err := database.GetEnvironment(environmentID); err != nil {
		return deployment.NASProfile{}, err
	}
	if environmentID != database.LocalEnvironmentID {
		return deployment.NASProfile{}, errors.New("当前版本尚未启用远程环境画像")
	}
	base, err := defaultLocalNASProfile()
	if err != nil {
		return deployment.NASProfile{}, err
	}
	record, err := database.GetNASProfile(environmentID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return deployment.NASProfile{}, err
	}
	persisted := deployment.ProfileOverrides{}
	if err == nil && strings.TrimSpace(record.ProfileJSON) != "" && strings.TrimSpace(record.ProfileJSON) != "{}" {
		if decodeErr := json.Unmarshal([]byte(record.ProfileJSON), &persisted); decodeErr != nil {
			return deployment.NASProfile{}, decodeErr
		}
		persisted = userMaintainedNASOverrides(persisted)
	}
	profile, err := deployment.ResolveProfile(environmentID, base, persisted)
	if err != nil {
		return deployment.NASProfile{}, err
	}
	profile, err = deployment.ResolveProfile(environmentID, profile, appOverrides)
	if err != nil {
		return deployment.NASProfile{}, err
	}
	return withNASLibraryRoots(profile)
}

func userMaintainedNASOverrides(overrides deployment.ProfileOverrides) deployment.ProfileOverrides {
	var networkPolicy *deployment.NetworkPolicy
	if overrides.NetworkPolicy != nil {
		networkPolicy = &deployment.NetworkPolicy{
			DefaultNetwork: "bridge",
			AllowHost:      overrides.NetworkPolicy.AllowHost,
		}
	}
	return deployment.ProfileOverrides{
		PUID:          overrides.PUID,
		PGID:          overrides.PGID,
		Timezone:      overrides.Timezone,
		NetworkPolicy: networkPolicy,
		LibraryPaths:  overrides.LibraryPaths,
	}
}

func saveLocalEnvironmentProfile(overrides deployment.ProfileOverrides) (deployment.NASProfile, error) {
	overrides.ProjectRoot = nil
	overrides.ContainerProjectRoot = nil
	overrides.Architecture = nil
	overrides.PortRange = nil
	overrides.HostAddresses = nil
	overrides.AllowedBindRoots = nil
	overrides.Devices = nil
	overrides.BackupTargetID = nil

	base, err := defaultLocalNASProfile()
	if err != nil {
		return deployment.NASProfile{}, err
	}
	if _, err := deployment.ResolveProfile(database.LocalEnvironmentID, base, overrides); err != nil {
		return deployment.NASProfile{}, fmt.Errorf("%w: %v", errInvalidLocalEnvironmentProfile, err)
	}
	overrides = userMaintainedNASOverrides(overrides)
	raw, err := json.Marshal(overrides)
	if err != nil {
		return deployment.NASProfile{}, err
	}
	if _, err := database.UpsertNASProfile(database.LocalEnvironmentID, string(raw)); err != nil {
		return deployment.NASProfile{}, err
	}
	return resolveEnvironmentProfile(database.LocalEnvironmentID, deployment.ProfileOverrides{})
}

func recordLocalEnvironmentProfileUpdated() {
	_ = database.SaveNotification(&database.Notification{
		Type:      "success",
		EventType: "environment_profile_updated",
		Category:  "system",
		Message:   "NAS 部署默认值已更新",
		Read:      false,
	})
}

func withNASLibraryRoots(profile deployment.NASProfile) (deployment.NASProfile, error) {
	roots := append([]string{}, profile.AllowedBindRoots...)
	roots = append(roots,
		profile.LibraryPaths.Media,
		profile.LibraryPaths.Novel,
		profile.LibraryPaths.Comic,
		profile.LibraryPaths.Music,
		profile.LibraryPaths.Photo,
	)
	return deployment.ResolveProfile(profile.EnvironmentID, profile, deployment.ProfileOverrides{
		AllowedBindRoots: roots,
	})
}

func defaultLocalNASProfile() (deployment.NASProfile, error) {
	current, err := settings.GetSettings()
	if err != nil {
		return deployment.NASProfile{}, err
	}
	containerProjectRoot := filepath.Clean(settings.GetProjectRoot())
	hostProjectRoot := strings.TrimSpace(settings.GetHostProjectRoot())
	if hostProjectRoot == "" {
		hostProjectRoot = containerProjectRoot
	}
	hostProjectRoot = filepath.Clean(hostProjectRoot)
	profile := deployment.NASProfile{
		EnvironmentID:        database.LocalEnvironmentID,
		ProjectRoot:          hostProjectRoot,
		ContainerProjectRoot: containerProjectRoot,
		PUID:                 environmentInt("PUID", 1000),
		PGID:                 environmentInt("PGID", 1000),
		Timezone:             localEnvironmentTimezone(),
		Architecture:         runtime.GOARCH,
		PortRange: deployment.PortRange{
			Start:        current.AllocPortStart,
			End:          current.AllocPortEnd,
			AutoAllocate: current.AllowAutoAllocPort,
		},
		HostAddresses:    localHostAddresses(current),
		AllowedBindRoots: []string{hostProjectRoot},
		Devices:          detectedLocalDevices(),
		NetworkPolicy: deployment.NetworkPolicy{
			DefaultNetwork: "bridge",
			AllowHost:      true,
		},
	}
	return deployment.ResolveProfile(database.LocalEnvironmentID, profile, deployment.ProfileOverrides{})
}

func localEnvironmentTimezone() string {
	if timezone := strings.TrimSpace(os.Getenv("TZ")); timezone != "" {
		return timezone
	}
	return "Asia/Shanghai"
}

func environmentInt(key string, fallback int) int {
	value, err := strconv.Atoi(strings.TrimSpace(os.Getenv(key)))
	if err != nil || value < 0 {
		return fallback
	}
	return value
}

func localHostAddresses(current settings.Settings) []string {
	addresses := make([]string, 0, 2)
	seen := map[string]struct{}{}
	for _, raw := range []string{current.LanUrl, current.WanUrl} {
		address := hostFromConfiguredURL(raw)
		if address == "" {
			continue
		}
		if _, exists := seen[address]; exists {
			continue
		}
		seen[address] = struct{}{}
		addresses = append(addresses, address)
	}
	return addresses
}

func hostFromConfiguredURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err == nil && parsed.Hostname() != "" {
		return parsed.Hostname()
	}
	parsed, err = url.Parse("//" + raw)
	if err == nil {
		return parsed.Hostname()
	}
	return ""
}

func detectedLocalDevices() []string {
	devices := make([]string, 0)
	for _, path := range []string{"/dev/dri", "/dev/net/tun"} {
		if _, err := os.Stat(path); err == nil {
			devices = append(devices, path)
		}
	}
	for _, pattern := range []string{"/dev/ttyUSB*", "/dev/ttyACM*"} {
		matches, _ := filepath.Glob(pattern)
		devices = append(devices, matches...)
	}
	sort.Strings(devices)
	return devices
}

type composePreflightTargetContextKey struct{}

func composePreflightUsedPorts(containers []types.Container, target composeOperationTarget) map[string]struct{} {
	used := make(map[string]struct{})
	for _, container := range containers {
		labels := container.Labels
		workingDir := strings.TrimSpace(labels["com.docker.compose.project.working_dir"])
		if workingDir == "" {
			if configFiles := strings.TrimSpace(labels["com.docker.compose.project.config_files"]); configFiles != "" {
				workingDir = filepath.Dir(strings.Split(configFiles, ",")[0])
			}
		}
		owned := target.ComposeProjectName != "" && target.ProjectDir != "" && workingDir != "" &&
			labels["com.docker.compose.project"] == target.ComposeProjectName &&
			filepath.Clean(canonicalComposeObservedProjectPath(workingDir, target.ComposeProjectName)) == filepath.Clean(target.ProjectDir)
		if owned {
			continue
		}
		for _, port := range container.Ports {
			if port.PublicPort > 0 {
				used[portKey(int(port.PublicPort), port.Type)] = struct{}{}
			}
		}
	}
	return used
}

func defaultEnvironmentPreflight(ctx context.Context, intent deployment.Intent, profile deployment.NASProfile) deployment.PreflightResult {
	cli, clientErr := tradisdocker.NewDockerClient()
	if clientErr == nil {
		defer cli.Close()
	}
	usedPorts := make(map[string]struct{})
	target, _ := ctx.Value(composePreflightTargetContextKey{}).(composeOperationTarget)
	if intent.EnvironmentID != database.LocalEnvironmentID || intent.ProjectName != target.ComposeProjectName || filepath.Clean(intent.BuildContext) != filepath.Clean(target.ProjectDir) {
		target = composeOperationTarget{}
	}
	var ownershipErr error
	networks := make(map[string]struct{})
	if cli != nil {
		if containers, err := cli.ContainerList(ctx, types.ContainerListOptions{All: true}); err == nil {
			usedPorts = composePreflightUsedPorts(containers, target)
		} else if target.ComposeProjectName != "" {
			ownershipErr = err
		}
		if items, err := cli.NetworkList(ctx, types.NetworkListOptions{}); err == nil {
			for _, item := range items {
				networks[item.Name] = struct{}{}
				networks[item.ID] = struct{}{}
			}
		}
	}

	deps := deployment.PreflightDependencies{
		DockerInfo: func(callCtx context.Context) (deployment.DockerRuntimeInfo, error) {
			if clientErr != nil {
				return deployment.DockerRuntimeInfo{}, clientErr
			}
			ping, err := cli.Ping(callCtx)
			if err != nil {
				return deployment.DockerRuntimeInfo{}, err
			}
			version, err := cli.ServerVersion(callCtx)
			if err != nil {
				return deployment.DockerRuntimeInfo{}, err
			}
			return deployment.DockerRuntimeInfo{APIVersion: ping.APIVersion, Architecture: version.Arch}, nil
		},
		PortAvailable: func(_ context.Context, port int, protocol string) (bool, error) {
			if ownershipErr != nil {
				return false, fmt.Errorf("无法核验现有项目端口归属: %w", ownershipErr)
			}
			_, used := usedPorts[portKey(port, protocol)]
			return !used, nil
		},
		NetworkExists: func(_ context.Context, name string) (bool, error) {
			if clientErr != nil {
				return true, nil
			}
			_, exists := networks[name]
			return exists, nil
		},
		ImageReachable: func(callCtx context.Context, image string) (bool, error) {
			if cli != nil {
				if _, _, err := cli.ImageInspectWithRaw(callCtx, image); err == nil {
					return true, nil
				}
			}
			host := imageRegistryHost(image)
			_, err := net.DefaultResolver.LookupHost(callCtx, host)
			return err == nil, err
		},
		ComposeConfig: validateExpandedCompose,
	}
	deps.PlannedConfigFiles, _ = ctx.Value(composePreflightFilesContextKey{}).(map[string]bool)
	return deployment.Preflight(ctx, intent, profile, deps)
}

type composePreflightFilesContextKey struct{}

func validateExpandedCompose(ctx context.Context, intent deployment.Intent, sourceDir string) error {
	projectDir := os.TempDir()
	if info, err := os.Stat(sourceDir); err == nil && info.IsDir() {
		projectDir = sourceDir
	}
	args := []string{
		"compose",
		"--project-directory", projectDir,
		"--file", "-",
		"config",
		"--quiet",
	}
	command := exec.CommandContext(ctx, "docker", args...)
	command.Dir = projectDir
	command.Stdin = strings.NewReader(intent.ComposeYAML)
	command.Env = append(os.Environ(), "COMPOSE_PROJECT_NAME=tradis-preflight")
	parameterNames := make([]string, 0, len(intent.Parameters))
	for name := range intent.Parameters {
		if isLikelyEnvKey(name) {
			parameterNames = append(parameterNames, name)
		}
	}
	sort.Strings(parameterNames)
	for _, name := range parameterNames {
		value := intent.Parameters[name]
		if value == nil {
			continue
		}
		command.Env = append(command.Env, name+"="+fmt.Sprint(value))
	}
	if _, err := command.Output(); err != nil {
		return fmt.Errorf("%s", describeExecError(err))
	}
	return nil
}

func portKey(port int, protocol string) string {
	protocol = strings.ToLower(strings.TrimSpace(protocol))
	if protocol == "" {
		protocol = "tcp"
	}
	return strconv.Itoa(port) + "/" + protocol
}

func imageRegistryHost(image string) string {
	first := strings.Split(strings.TrimSpace(image), "/")[0]
	if !strings.Contains(first, ".") && !strings.Contains(first, ":") && first != "localhost" {
		return "registry-1.docker.io"
	}
	return first
}
