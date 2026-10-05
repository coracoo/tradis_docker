package deployment

import (
	"errors"
	"testing"

	"gopkg.in/yaml.v3"
)

func localizeTestProfile(root string) NASProfile {
	return NASProfile{
		EnvironmentID:        "local",
		ProjectRoot:          root,
		ContainerProjectRoot: root,
		PUID:                 1000,
		PGID:                 1000,
		Timezone:             "Asia/Shanghai",
		Architecture:         "amd64",
		PortRange:            PortRange{Start: 50000, End: 50010, AutoAllocate: true},
		AllowedBindRoots:     []string{root, "/volume1/media"},
		NetworkPolicy:        NetworkPolicy{DefaultNetwork: "bridge"},
	}
}

func decodeLocalizedRoot(t *testing.T, raw string) map[string]any {
	t.Helper()
	root := map[string]any{}
	if err := yaml.Unmarshal([]byte(raw), &root); err != nil {
		t.Fatalf("decode localized Compose: %v\n%s", err, raw)
	}
	return root
}

func localizedService(t *testing.T, raw string, name string) map[string]any {
	t.Helper()
	root := decodeLocalizedRoot(t, raw)
	services, ok := root["services"].(map[string]any)
	if !ok {
		t.Fatalf("localized services missing: %#v", root)
	}
	service, ok := services[name].(map[string]any)
	if !ok {
		t.Fatalf("localized service %q missing: %#v", name, services)
	}
	return service
}

func TestLocalizeComposePreservesNamedVolumesAndMapsRelativeBinds(t *testing.T) {
	raw := `services:
  app:
    image: example/app:latest
    volumes:
      - app-data:/data
      - ./config:/config:ro
      - type: volume
        source: cache-data
        target: /cache
      - type: bind
        source: ./media
        target: /media
volumes:
  app-data: {}
  cache-data: {}
`
	runtimeYAML, changes, err := LocalizeCompose(raw, localizeTestProfile("/volume1/docker"), ComposeLocalizationOverrides{
		ProjectName: "demo",
	})
	if err != nil {
		t.Fatalf("LocalizeCompose: %v", err)
	}

	service := localizedService(t, runtimeYAML, "app")
	volumes, ok := service["volumes"].([]any)
	if !ok || len(volumes) != 4 {
		t.Fatalf("unexpected volumes: %#v", service["volumes"])
	}
	if volumes[0] != "app-data:/data" {
		t.Fatalf("short named volume was changed: %#v", volumes[0])
	}
	if volumes[1] != "/volume1/docker/demo/config:/config:ro" {
		t.Fatalf("relative short bind was not localized: %#v", volumes[1])
	}
	longVolume, _ := volumes[2].(map[string]any)
	if longVolume["type"] != "volume" || longVolume["source"] != "cache-data" {
		t.Fatalf("long named volume was changed: %#v", longVolume)
	}
	longBind, _ := volumes[3].(map[string]any)
	if longBind["source"] != "/volume1/docker/demo/media" {
		t.Fatalf("relative long bind was not localized: %#v", longBind)
	}
	if len(changes) != 2 {
		t.Fatalf("expected two field changes, got %#v", changes)
	}
	for _, change := range changes {
		if change.Type != "bind" || change.ReasonCode != "relative_bind_to_project" {
			t.Fatalf("unexpected bind change: %#v", change)
		}
	}
}

func TestLocalizeComposeMapsRelativeBindsToHostProjectRoot(t *testing.T) {
	profile := localizeTestProfile("/workspace/client/backend/project")
	profile.ProjectRoot = "/volume1/docker"
	profile.AllowedBindRoots = []string{profile.ProjectRoot}

	runtimeYAML, changes, err := LocalizeCompose(`services:
  app:
    image: example/app:latest
    volumes:
      - ./data:/data
`, profile, ComposeLocalizationOverrides{ProjectName: "demo"})
	if err != nil {
		t.Fatalf("LocalizeCompose: %v", err)
	}

	volumes := localizedService(t, runtimeYAML, "app")["volumes"].([]any)
	if volumes[0] != "/volume1/docker/demo/data:/data" {
		t.Fatalf("relative bind must use the Docker host project root: %#v", volumes[0])
	}
	if len(changes) != 1 || changes[0].After != "/volume1/docker/demo/data" {
		t.Fatalf("localized change must expose the host path: %#v", changes)
	}
}

func TestLocalizeComposeKeepsAllowedAbsoluteBindAndBlocksUnsafePath(t *testing.T) {
	profile := localizeTestProfile("/volume1/docker")
	allowed := `services:
  app:
    image: example/app
    volumes:
      - /volume1/media:/media:ro
`
	runtimeYAML, changes, err := LocalizeCompose(allowed, profile, ComposeLocalizationOverrides{ProjectName: "demo"})
	if err != nil {
		t.Fatalf("allowed bind: %v", err)
	}
	volumes := localizedService(t, runtimeYAML, "app")["volumes"].([]any)
	if volumes[0] != "/volume1/media:/media:ro" || len(changes) != 0 {
		t.Fatalf("allowed absolute bind must remain unchanged: %#v %#v", volumes, changes)
	}

	unsafe := `services:
  app:
    image: example/app
    volumes:
      - /etc:/host-etc:ro
`
	_, _, err = LocalizeCompose(unsafe, profile, ComposeLocalizationOverrides{ProjectName: "demo"})
	var structured *StructuredError
	if !errors.As(err, &structured) || structured.Code != ErrorCodePreflightBlocked {
		t.Fatalf("unsafe absolute bind must return structured blocking error: %#v", err)
	}
	if structured.Details["reasonCode"] != "bind_root_not_allowed" {
		t.Fatalf("unexpected unsafe bind reason: %#v", structured.Details)
	}
}

func TestLocalizeComposeRewritesTCPUDPAndPreservesHostIP(t *testing.T) {
	profile := localizeTestProfile("/volume1/docker")
	profile.PortRange = PortRange{Start: 50000, End: 50002, AutoAllocate: true}
	raw := `services:
  app:
    image: example/app
    ports:
      - "127.0.0.1:8080:80/tcp"
      - "[::1]:5353:53/udp"
      - target: 443
        published: 8443
        host_ip: 0.0.0.0
        protocol: tcp
  host-service:
    image: example/host
    network_mode: host
    ports:
      - "9000:9000"
`
	runtimeYAML, changes, err := LocalizeCompose(raw, profile, ComposeLocalizationOverrides{
		ProjectName: "demo",
		PortAvailable: func(port int, protocol string) bool {
			return !(port == 50000 && protocol == "tcp")
		},
	})
	if err != nil {
		t.Fatalf("LocalizeCompose: %v", err)
	}

	ports := localizedService(t, runtimeYAML, "app")["ports"].([]any)
	if ports[0] != "127.0.0.1:50001:80/tcp" {
		t.Fatalf("IPv4 TCP mapping changed incorrectly: %#v", ports[0])
	}
	if ports[1] != "[::1]:50000:53/udp" {
		t.Fatalf("IPv6 UDP mapping changed incorrectly: %#v", ports[1])
	}
	longPort, _ := ports[2].(map[string]any)
	if longPort["published"] != 50002 || longPort["target"] != 443 || longPort["protocol"] != "tcp" || longPort["host_ip"] != "0.0.0.0" {
		t.Fatalf("long port mapping changed incorrectly: %#v", longPort)
	}
	hostPorts := localizedService(t, runtimeYAML, "host-service")["ports"].([]any)
	if hostPorts[0] != "9000:9000" {
		t.Fatalf("host network ports must not be rewritten: %#v", hostPorts)
	}
	if len(changes) != 3 {
		t.Fatalf("expected three port changes, got %#v", changes)
	}
}

func TestLocalizeComposeHonorsDisabledAutomaticPorts(t *testing.T) {
	auto := false
	raw := `services:
  app:
    image: example/app
    ports:
      - "8080:80/udp"
`
	runtimeYAML, changes, err := LocalizeCompose(raw, localizeTestProfile("/volume1/docker"), ComposeLocalizationOverrides{
		ProjectName:       "demo",
		AutoAllocatePorts: &auto,
	})
	if err != nil {
		t.Fatalf("LocalizeCompose: %v", err)
	}
	ports := localizedService(t, runtimeYAML, "app")["ports"].([]any)
	if ports[0] != "8080:80/udp" || len(changes) != 0 {
		t.Fatalf("disabled automatic ports must preserve mapping: %#v %#v", ports, changes)
	}
}

func TestLocalizeComposeReportsPortPoolExhaustion(t *testing.T) {
	profile := localizeTestProfile("/volume1/docker")
	profile.PortRange = PortRange{Start: 50000, End: 50000, AutoAllocate: true}
	_, _, err := LocalizeCompose(`services:
  app:
    image: example/app
    ports: ["8080:80"]
`, profile, ComposeLocalizationOverrides{
		ProjectName:   "demo",
		PortAvailable: func(int, string) bool { return false },
	})
	var structured *StructuredError
	if !errors.As(err, &structured) || structured.Code != ErrorCodePreflightBlocked {
		t.Fatalf("port exhaustion must return structured blocking error: %#v", err)
	}
	if structured.Details["reasonCode"] != "port_pool_exhausted" {
		t.Fatalf("unexpected port exhaustion details: %#v", structured.Details)
	}
}

func TestLocalizeComposeExpandsShortPortRangesDeterministically(t *testing.T) {
	profile := localizeTestProfile("/volume1/docker")
	profile.PortRange = PortRange{Start: 50000, End: 50001, AutoAllocate: true}
	runtimeYAML, changes, err := LocalizeCompose(`services:
  app:
    image: example/app
    ports: ["8080-8081:80-81/tcp"]
`, profile, ComposeLocalizationOverrides{ProjectName: "demo"})
	if err != nil {
		t.Fatalf("LocalizeCompose: %v", err)
	}
	ports := localizedService(t, runtimeYAML, "app")["ports"].([]any)
	if len(ports) != 2 || ports[0] != "50000:80/tcp" || ports[1] != "50001:81/tcp" {
		t.Fatalf("short port range was not expanded deterministically: %#v", ports)
	}
	if len(changes) != 2 {
		t.Fatalf("expanded range must record every published port change: %#v", changes)
	}
}

func TestLocalizeComposeAcceptsNumericContainerPort(t *testing.T) {
	profile := localizeTestProfile("/volume1/docker")
	profile.PortRange = PortRange{Start: 50000, End: 50000, AutoAllocate: true}
	runtimeYAML, changes, err := LocalizeCompose(`services:
  app:
    image: example/app
    ports: [80]
`, profile, ComposeLocalizationOverrides{ProjectName: "demo"})
	if err != nil {
		t.Fatalf("LocalizeCompose: %v", err)
	}
	ports := localizedService(t, runtimeYAML, "app")["ports"].([]any)
	if len(ports) != 1 || ports[0] != "50000:80" || len(changes) != 1 {
		t.Fatalf("numeric container port was not localized: %#v %#v", ports, changes)
	}
}

func TestLocalizeComposeExpandsVariablePortDefaultsBeforeAllocation(t *testing.T) {
	profile := localizeTestProfile("/volume1/docker")
	profile.PortRange = PortRange{Start: 50000, End: 50000, AutoAllocate: true}
	runtimeYAML, changes, err := LocalizeCompose(`services:
  neo4j:
    image: neo4j:5.26.2
    ports:
      - "${NEO4J_PORT:-7687}:${NEO4J_PORT:-7687}"
`, profile, ComposeLocalizationOverrides{ProjectName: "graphiti"})
	if err != nil {
		t.Fatalf("LocalizeCompose: %v", err)
	}
	ports := localizedService(t, runtimeYAML, "neo4j")["ports"].([]any)
	if len(ports) != 1 || ports[0] != "50000:7687" {
		t.Fatalf("variable port default was not localized: %#v", ports)
	}
	if len(changes) != 1 || changes[0].Before != 7687 || changes[0].After != 50000 {
		t.Fatalf("unexpected variable port changes: %#v", changes)
	}
}

func TestLocalizeComposeExpandsNestedVariablePortDefaultsBeforeAllocation(t *testing.T) {
	runtimeYAML, changes, err := LocalizeCompose(`services:
  api-gw:
    image: envoyproxy/envoy:latest
    ports:
      - "${API_GW_HTTP_PORT:-${KONG_HTTP_PORT:-8000}}:8000/tcp"
`, localizeTestProfile("/volume1/docker"), ComposeLocalizationOverrides{
		ProjectName: "supabase",
	})
	if err != nil {
		t.Fatalf("LocalizeCompose nested port default: %v", err)
	}
	ports := localizedService(t, runtimeYAML, "api-gw")["ports"].([]any)
	if len(ports) != 1 || ports[0] != "50000:8000/tcp" {
		t.Fatalf("nested variable port default was not localized: %#v", ports)
	}
	if len(changes) != 1 || changes[0].Before != 8000 {
		t.Fatalf("unexpected nested variable port change: %#v", changes)
	}

	runtimeYAML, _, err = LocalizeCompose(`services:
  api-gw:
    image: envoyproxy/envoy:latest
    ports:
      - "${API_GW_HTTP_PORT:-${KONG_HTTP_PORT:-8000}}:8000/tcp"
`, localizeTestProfile("/volume1/docker"), ComposeLocalizationOverrides{
		ProjectName: "supabase",
		InterpolationValues: map[string]string{
			"KONG_HTTP_PORT": "9000",
		},
	})
	if err != nil {
		t.Fatalf("LocalizeCompose configured nested port default: %v", err)
	}
	ports = localizedService(t, runtimeYAML, "api-gw")["ports"].([]any)
	if len(ports) != 1 || ports[0] != "50000:8000/tcp" {
		t.Fatalf("configured nested published port was not localized: %#v", ports)
	}
}

func TestLocalizeComposeExpandsConfiguredVariablePortBeforeAllocation(t *testing.T) {
	profile := localizeTestProfile("/volume1/docker")
	profile.PortRange = PortRange{Start: 50000, End: 50000, AutoAllocate: true}
	runtimeYAML, _, err := LocalizeCompose(`services:
  neo4j:
    image: neo4j:5.26.2
    ports:
      - "${NEO4J_PORT:-7687}:${NEO4J_PORT:-7687}"
`, profile, ComposeLocalizationOverrides{
		ProjectName:         "graphiti",
		InterpolationValues: map[string]string{"NEO4J_PORT": "8687"},
	})
	if err != nil {
		t.Fatalf("LocalizeCompose: %v", err)
	}
	ports := localizedService(t, runtimeYAML, "neo4j")["ports"].([]any)
	if len(ports) != 1 || ports[0] != "50000:8687" {
		t.Fatalf("configured variable port was not localized: %#v", ports)
	}
}
