package deployment

import (
	"reflect"
	"testing"
)

func TestResolveProfileAppliesOnlyExplicitApplicationOverrides(t *testing.T) {
	base := NASProfile{
		EnvironmentID:        "local",
		ProjectRoot:          "/volume1/docker",
		ContainerProjectRoot: "/volume1/docker",
		PUID:                 1000,
		PGID:                 1000,
		Timezone:             "Asia/Shanghai",
		Architecture:         "amd64",
		PortRange:            PortRange{Start: 50000, End: 51000, AutoAllocate: true},
		HostAddresses:        []string{"192.168.1.10"},
		AllowedBindRoots:     []string{"/volume1/docker", "/volume1/media"},
		Devices:              []string{"/dev/dri"},
		BackupTargetID:       "local-backup",
		NetworkPolicy:        NetworkPolicy{DefaultNetwork: "bridge", AllowHost: false},
	}
	puid := 1026
	timezone := "Asia/Tokyo"
	devices := []string{}

	resolved, err := ResolveProfile("local", base, ProfileOverrides{
		PUID:     &puid,
		Timezone: &timezone,
		Devices:  devices,
	})
	if err != nil {
		t.Fatalf("ResolveProfile: %v", err)
	}

	if resolved.PUID != 1026 || resolved.PGID != 1000 || resolved.Timezone != "Asia/Tokyo" {
		t.Fatalf("unexpected scalar inheritance: %#v", resolved)
	}
	if !reflect.DeepEqual(resolved.Devices, []string{}) {
		t.Fatalf("explicit empty devices must clear the inherited list: %#v", resolved.Devices)
	}
	if !reflect.DeepEqual(resolved.AllowedBindRoots, base.AllowedBindRoots) || resolved.PortRange != base.PortRange {
		t.Fatalf("unrelated fields were not inherited: %#v", resolved)
	}
	if resolved.EnvironmentID != "local" {
		t.Fatalf("expected local environment, got %q", resolved.EnvironmentID)
	}
}

func TestResolveProfileNormalizesListsAndRejectsInvalidProfiles(t *testing.T) {
	base := NASProfile{
		ProjectRoot:          "/volume1/docker/",
		ContainerProjectRoot: "/volume1/docker/",
		PUID:                 1000,
		PGID:                 1000,
		Timezone:             "Asia/Shanghai",
		Architecture:         "x86_64",
		PortRange:            PortRange{Start: 50000, End: 51000},
		HostAddresses:        []string{" 192.168.1.10 ", "192.168.1.10", ""},
		AllowedBindRoots:     []string{"/volume1/docker/", "/volume1/docker"},
		Devices:              []string{" /dev/dri ", "/dev/dri"},
	}

	resolved, err := ResolveProfile("local", base, ProfileOverrides{})
	if err != nil {
		t.Fatalf("ResolveProfile: %v", err)
	}
	if resolved.Architecture != "amd64" {
		t.Fatalf("expected normalized architecture, got %q", resolved.Architecture)
	}
	if !reflect.DeepEqual(resolved.HostAddresses, []string{"192.168.1.10"}) ||
		!reflect.DeepEqual(resolved.AllowedBindRoots, []string{"/volume1/docker"}) ||
		!reflect.DeepEqual(resolved.Devices, []string{"/dev/dri"}) {
		t.Fatalf("profile lists were not normalized: %#v", resolved)
	}

	invalidRange := base
	invalidRange.PortRange = PortRange{Start: 51000, End: 50000}
	if _, err := ResolveProfile("local", invalidRange, ProfileOverrides{}); err == nil {
		t.Fatal("invalid port range must be rejected")
	}
	if _, err := ResolveProfile("", base, ProfileOverrides{}); err == nil {
		t.Fatal("empty environment id must be rejected")
	}
}

func TestResolveProfilePersistsStandardNASLibraryPaths(t *testing.T) {
	base := NASProfile{
		EnvironmentID: "local", ProjectRoot: "/srv/apps", ContainerProjectRoot: "/srv/apps",
		PUID: 1000, PGID: 1000, Timezone: "Asia/Shanghai", Architecture: "amd64",
		PortRange:        PortRange{Start: 50000, End: 50100},
		AllowedBindRoots: []string{"/srv/apps"},
		NetworkPolicy:    NetworkPolicy{DefaultNetwork: "bridge"},
	}
	paths := NASLibraryPaths{
		Media: "/volume1/media", Novel: "/volume1/novel", Comic: "/volume1/comic",
		Music: "/volume1/music", Photo: "/volume1/photo",
	}
	resolved, err := ResolveProfile("local", base, ProfileOverrides{LibraryPaths: &paths})
	if err != nil {
		t.Fatalf("ResolveProfile: %v", err)
	}
	if resolved.LibraryPaths != paths {
		t.Fatalf("library paths = %#v", resolved.LibraryPaths)
	}
}

func TestNormalizeArchitectureUsesDockerPlatformNames(t *testing.T) {
	tests := map[string]string{
		"x86_64":  "amd64",
		"aarch64": "arm64",
		"arm":     "arm/v7",
		"armv7l":  "arm/v7",
		"armv6l":  "arm/v6",
		"i686":    "386",
	}
	for input, expected := range tests {
		if actual := normalizeArchitecture(input); actual != expected {
			t.Errorf("normalizeArchitecture(%q) = %q, want %q", input, actual, expected)
		}
	}
}
