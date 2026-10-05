package deployment

import (
	"fmt"
	"path/filepath"
	"strings"
)

type PortRange struct {
	Start        int  `json:"start"`
	End          int  `json:"end"`
	AutoAllocate bool `json:"autoAllocate"`
}

type NetworkPolicy struct {
	DefaultNetwork string `json:"defaultNetwork"`
	AllowHost      bool   `json:"allowHost"`
}

type NASLibraryPaths struct {
	Media string `json:"media"`
	Novel string `json:"novel"`
	Comic string `json:"comic"`
	Music string `json:"music"`
	Photo string `json:"photo"`
}

type NASProfile struct {
	EnvironmentID        string          `json:"environmentId"`
	ProjectRoot          string          `json:"projectRoot"`
	ContainerProjectRoot string          `json:"containerProjectRoot"`
	PUID                 int             `json:"puid"`
	PGID                 int             `json:"pgid"`
	Timezone             string          `json:"timezone"`
	Architecture         string          `json:"architecture"`
	PortRange            PortRange       `json:"portRange"`
	HostAddresses        []string        `json:"hostAddresses"`
	AllowedBindRoots     []string        `json:"allowedBindRoots"`
	Devices              []string        `json:"devices"`
	BackupTargetID       string          `json:"backupTargetId,omitempty"`
	NetworkPolicy        NetworkPolicy   `json:"networkPolicy"`
	LibraryPaths         NASLibraryPaths `json:"libraryPaths"`
}

// ProfileOverrides uses pointers for scalar values so an application only
// replaces fields it explicitly owns. A non-nil empty slice intentionally
// clears an inherited list.
type ProfileOverrides struct {
	ProjectRoot          *string          `json:"projectRoot,omitempty"`
	ContainerProjectRoot *string          `json:"containerProjectRoot,omitempty"`
	PUID                 *int             `json:"puid,omitempty"`
	PGID                 *int             `json:"pgid,omitempty"`
	Timezone             *string          `json:"timezone,omitempty"`
	Architecture         *string          `json:"architecture,omitempty"`
	PortRange            *PortRange       `json:"portRange,omitempty"`
	HostAddresses        []string         `json:"hostAddresses,omitempty"`
	AllowedBindRoots     []string         `json:"allowedBindRoots,omitempty"`
	Devices              []string         `json:"devices,omitempty"`
	BackupTargetID       *string          `json:"backupTargetId,omitempty"`
	NetworkPolicy        *NetworkPolicy   `json:"networkPolicy,omitempty"`
	LibraryPaths         *NASLibraryPaths `json:"libraryPaths,omitempty"`
}

func ResolveProfile(environmentID string, base NASProfile, overrides ProfileOverrides) (NASProfile, error) {
	resolved := base
	resolved.EnvironmentID = strings.TrimSpace(environmentID)

	if overrides.ProjectRoot != nil {
		resolved.ProjectRoot = *overrides.ProjectRoot
	}
	if overrides.ContainerProjectRoot != nil {
		resolved.ContainerProjectRoot = *overrides.ContainerProjectRoot
	}
	if overrides.PUID != nil {
		resolved.PUID = *overrides.PUID
	}
	if overrides.PGID != nil {
		resolved.PGID = *overrides.PGID
	}
	if overrides.Timezone != nil {
		resolved.Timezone = *overrides.Timezone
	}
	if overrides.Architecture != nil {
		resolved.Architecture = *overrides.Architecture
	}
	if overrides.PortRange != nil {
		resolved.PortRange = *overrides.PortRange
	}
	if overrides.HostAddresses != nil {
		resolved.HostAddresses = cloneStringSlice(overrides.HostAddresses)
	}
	if overrides.AllowedBindRoots != nil {
		resolved.AllowedBindRoots = cloneStringSlice(overrides.AllowedBindRoots)
	}
	if overrides.Devices != nil {
		resolved.Devices = cloneStringSlice(overrides.Devices)
	}
	if overrides.BackupTargetID != nil {
		resolved.BackupTargetID = *overrides.BackupTargetID
	}
	if overrides.NetworkPolicy != nil {
		resolved.NetworkPolicy = *overrides.NetworkPolicy
	}
	if overrides.LibraryPaths != nil {
		resolved.LibraryPaths = *overrides.LibraryPaths
	}

	resolved = normalizeNASProfile(resolved)
	if err := validateNASProfile(resolved); err != nil {
		return NASProfile{}, err
	}
	return resolved, nil
}

func cloneStringSlice(values []string) []string {
	if values == nil {
		return nil
	}
	out := make([]string, len(values))
	copy(out, values)
	return out
}

func normalizeNASProfile(profile NASProfile) NASProfile {
	profile.EnvironmentID = strings.TrimSpace(profile.EnvironmentID)
	profile.ProjectRoot = cleanAbsolutePath(profile.ProjectRoot)
	profile.ContainerProjectRoot = cleanAbsolutePath(profile.ContainerProjectRoot)
	profile.Timezone = strings.TrimSpace(profile.Timezone)
	if profile.Timezone == "" {
		profile.Timezone = "Asia/Shanghai"
	}
	profile.Architecture = normalizeArchitecture(profile.Architecture)
	profile.HostAddresses = normalizeStringList(profile.HostAddresses, false)
	profile.AllowedBindRoots = normalizeStringList(profile.AllowedBindRoots, true)
	profile.Devices = normalizeStringList(profile.Devices, true)
	profile.BackupTargetID = strings.TrimSpace(profile.BackupTargetID)
	profile.NetworkPolicy.DefaultNetwork = strings.TrimSpace(profile.NetworkPolicy.DefaultNetwork)
	if profile.NetworkPolicy.DefaultNetwork == "" {
		profile.NetworkPolicy.DefaultNetwork = "bridge"
	}
	profile.LibraryPaths = normalizeNASLibraryPaths(profile.LibraryPaths)
	return profile
}

func validateNASProfile(profile NASProfile) error {
	if profile.EnvironmentID == "" {
		return fmt.Errorf("environment id is required")
	}
	if profile.ProjectRoot == "" || !filepath.IsAbs(profile.ProjectRoot) {
		return fmt.Errorf("project_root must be an absolute path")
	}
	if profile.ContainerProjectRoot == "" || !filepath.IsAbs(profile.ContainerProjectRoot) {
		return fmt.Errorf("container_project_root must be an absolute path")
	}
	if profile.PUID < 0 || profile.PGID < 0 {
		return fmt.Errorf("puid and pgid must not be negative")
	}
	if profile.Architecture == "" {
		return fmt.Errorf("architecture is required")
	}
	if profile.PortRange.Start < 1 || profile.PortRange.End > 65535 || profile.PortRange.End < profile.PortRange.Start {
		return fmt.Errorf("port_range must be within 1-65535 and ordered")
	}
	for _, root := range profile.AllowedBindRoots {
		if !filepath.IsAbs(root) {
			return fmt.Errorf("allowed bind root %q must be absolute", root)
		}
	}
	for _, device := range profile.Devices {
		if !filepath.IsAbs(device) {
			return fmt.Errorf("device %q must be absolute", device)
		}
	}
	for label, path := range map[string]string{
		"media": profile.LibraryPaths.Media,
		"novel": profile.LibraryPaths.Novel,
		"comic": profile.LibraryPaths.Comic,
		"music": profile.LibraryPaths.Music,
		"photo": profile.LibraryPaths.Photo,
	} {
		if path != "" && !filepath.IsAbs(path) {
			return fmt.Errorf("%s library path %q must be absolute", label, path)
		}
	}
	return nil
}

func normalizeNASLibraryPaths(paths NASLibraryPaths) NASLibraryPaths {
	paths.Media = cleanAbsolutePath(paths.Media)
	paths.Novel = cleanAbsolutePath(paths.Novel)
	paths.Comic = cleanAbsolutePath(paths.Comic)
	paths.Music = cleanAbsolutePath(paths.Music)
	paths.Photo = cleanAbsolutePath(paths.Photo)
	return paths
}

func normalizeArchitecture(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "x86_64", "x86-64", "amd64":
		return "amd64"
	case "aarch64", "arm64", "arm64/v8":
		return "arm64"
	case "arm", "arm32", "armhf", "armv7", "armv7l", "arm/v7":
		return "arm/v7"
	case "armv6", "armv6l", "arm/v6":
		return "arm/v6"
	case "386", "i386", "i686":
		return "386"
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

func cleanAbsolutePath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return filepath.Clean(value)
}

func normalizeStringList(values []string, cleanPaths bool) []string {
	if values == nil {
		return nil
	}
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if cleanPaths {
			value = filepath.Clean(value)
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
