package api

import (
	"testing"

	"dockerpanel/backend/pkg/database"
	"dockerpanel/backend/pkg/docker"
)

func TestResolveDockerNetworkConfigPrefersDaemonProxyAndRuntimeMirrors(t *testing.T) {
	resolved := resolveDockerNetworkConfig(
		&docker.DaemonConfig{
			Proxies: &docker.ProxyConfig{
				HTTPProxy:  "http://proxy.internal:7890",
				HTTPSProxy: "http://proxy.internal:7890",
				NoProxy:    "localhost,127.0.0.1",
			},
		},
		true,
		dockerRuntimeNetworkConfig{RegistryMirrors: []string{"https://runtime-mirror.example/"}},
		true,
		&database.DockerProxy{
			Enabled:         false,
			RegistryMirrors: database.MarshalRegistryMirrors([]string{"https://stale-db.example"}),
		},
	)

	if !resolved.Config.Enabled || resolved.Config.HTTPProxy != "http://proxy.internal:7890" {
		t.Fatalf("daemon proxy was not selected: %#v", resolved)
	}
	if resolved.ProxySource != dockerConfigSourceDaemon {
		t.Fatalf("proxy source = %q, want daemon", resolved.ProxySource)
	}
	if len(resolved.Config.RegistryMirrors) != 1 || resolved.Config.RegistryMirrors[0] != "https://runtime-mirror.example/" {
		t.Fatalf("runtime mirrors were not selected: %#v", resolved.Config.RegistryMirrors)
	}
	if resolved.MirrorSource != dockerConfigSourceRuntime {
		t.Fatalf("mirror source = %q, want runtime", resolved.MirrorSource)
	}
	if !resolved.PendingRestart {
		t.Fatal("daemon proxy differs from runtime and must require restart")
	}
}

func TestResolveDockerNetworkConfigClearsStaleDatabaseWhenHostHasNoProxy(t *testing.T) {
	resolved := resolveDockerNetworkConfig(
		&docker.DaemonConfig{},
		true,
		dockerRuntimeNetworkConfig{},
		true,
		&database.DockerProxy{
			Enabled:    true,
			HTTPProxy:  "http://stale-db.example",
			HTTPSProxy: "http://stale-db.example",
		},
	)

	if resolved.Config.Enabled || resolved.Config.HTTPProxy != "" || resolved.Config.HTTPSProxy != "" {
		t.Fatalf("stale database proxy survived host reconciliation: %#v", resolved.Config)
	}
	if resolved.ProxySource != dockerConfigSourceRuntime {
		t.Fatalf("proxy source = %q, want runtime", resolved.ProxySource)
	}
}

func TestResolveDockerNetworkConfigFallsBackToDatabaseWhenHostUnavailable(t *testing.T) {
	stored := &database.DockerProxy{
		Enabled:         true,
		HTTPProxy:       "http://db-fallback.example",
		HTTPSProxy:      "http://db-fallback.example",
		NoProxy:         "localhost",
		RegistryMirrors: database.MarshalRegistryMirrors([]string{"https://db-mirror.example"}),
	}
	resolved := resolveDockerNetworkConfig(nil, false, dockerRuntimeNetworkConfig{}, false, stored)

	if !resolved.Config.Enabled || resolved.Config.HTTPProxy != stored.HTTPProxy {
		t.Fatalf("database proxy fallback was lost: %#v", resolved.Config)
	}
	if len(resolved.Config.RegistryMirrors) != 1 || resolved.Config.RegistryMirrors[0] != "https://db-mirror.example" {
		t.Fatalf("database mirror fallback was lost: %#v", resolved.Config.RegistryMirrors)
	}
	if resolved.ProxySource != dockerConfigSourceDatabase || resolved.MirrorSource != dockerConfigSourceDatabase {
		t.Fatalf("unexpected fallback sources: proxy=%q mirror=%q", resolved.ProxySource, resolved.MirrorSource)
	}
}

func TestResolveDockerNetworkConfigNormalizesMirrorSlashForRestartComparison(t *testing.T) {
	resolved := resolveDockerNetworkConfig(
		&docker.DaemonConfig{RegistryMirrors: []string{"https://mirror.example"}},
		true,
		dockerRuntimeNetworkConfig{RegistryMirrors: []string{"https://mirror.example/"}},
		true,
		&database.DockerProxy{},
	)

	if resolved.PendingRestart {
		t.Fatal("equivalent mirror URLs must not require a Docker restart")
	}
}
