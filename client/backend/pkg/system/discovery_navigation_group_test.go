package system

import (
	"dockerpanel/backend/pkg/settings"
	"testing"

	"github.com/docker/docker/api/types"
)

func TestSelectBestNavigationPortChoice_ComposePrefersWebService(t *testing.T) {
	choice, ok := selectBestNavigationPortChoice([]navigationContainerCandidate{
		{
			ContainerID: "db",
			Service:     "postgres",
			Name:        "demo-postgres",
			Image:       "postgres:16",
			Ports:       []int{5432},
		},
		{
			ContainerID: "web",
			Service:     "web",
			Name:        "demo-web",
			Image:       "nginx:latest",
			Ports:       []int{8080},
		},
	}, settings.Settings{})
	if !ok {
		t.Fatal("expected a selected port")
	}
	if choice.Candidate.ContainerID != "web" || choice.Port != 8080 {
		t.Fatalf("expected web:8080, got %s:%d", choice.Candidate.ContainerID, choice.Port)
	}
}

func TestSelectBestNavigationPortChoice_ExplicitPortLabelWins(t *testing.T) {
	choice, ok := selectBestNavigationPortChoice([]navigationContainerCandidate{
		{
			ContainerID: "app",
			Service:     "app",
			Name:        "demo-app",
			Ports:       []int{8080, 9001},
			Labels: map[string]string{
				"tradis.navigation.port": "9001",
			},
		},
	}, settings.Settings{})
	if !ok {
		t.Fatal("expected a selected port")
	}
	if choice.Port != 9001 {
		t.Fatalf("expected explicit port 9001, got %d", choice.Port)
	}
}

func TestBuildValidNavigationSourceKeys_IncludesComposeProject(t *testing.T) {
	keys := buildValidNavigationSourceKeys([]types.Container{
		{
			ID: "ctr1",
			Ports: []types.Port{
				{Type: "tcp", PublicPort: 8080},
			},
			Labels: map[string]string{
				"com.docker.compose.project": "demo",
			},
		},
	})
	if _, ok := keys["ctr1"]; !ok {
		t.Fatal("expected container id key")
	}
	if _, ok := keys["compose:demo"]; !ok {
		t.Fatal("expected compose source key")
	}
}

func TestBuildValidNavigationSourceKeys_KeepsStoppedComposeProject(t *testing.T) {
	keys := buildValidNavigationSourceKeys([]types.Container{
		{
			ID:    "stopped-ctr",
			State: "exited",
			Labels: map[string]string{
				"com.docker.compose.project": "stopped-demo",
			},
		},
	})
	if _, ok := keys["stopped-ctr"]; !ok {
		t.Fatal("expected stopped container id key")
	}
	if _, ok := keys["compose:stopped-demo"]; !ok {
		t.Fatal("expected stopped compose project source key")
	}
}

func TestComposeServiceNavigationKeys(t *testing.T) {
	base := navigationComposeSourceKey("aitoearn")
	if base != "compose:aitoearn" {
		t.Fatalf("base key = %q", base)
	}
	serviceKey := composeServiceNavigationSourceKey("aitoearn", "web")
	if serviceKey != "compose:aitoearn:web" {
		t.Fatalf("service key = %q", serviceKey)
	}
	if composeServiceNavigationSourceKey("aitoearn", "") != base {
		t.Fatalf("empty service must fall back to base key")
	}
	label := composeServiceNavigationLabel(navigationContainerCandidate{Service: "server"})
	if label != "server" {
		t.Fatalf("label = %q", label)
	}
	label = composeServiceNavigationLabel(navigationContainerCandidate{Name: "aitoearn-ai-1"})
	if label != "aitoearn-ai-1" {
		t.Fatalf("fallback label = %q", label)
	}
}

func TestBuildValidNavigationSourceKeys_IncludesComposeServiceKeys(t *testing.T) {
	containers := []types.Container{{
		ID: "ctr-1",
		Labels: map[string]string{
			"com.docker.compose.project": "aitoearn",
			"com.docker.compose.service": "web",
		},
	}}
	keys := buildValidNavigationSourceKeys(containers)
	for _, want := range []string{"ctr-1", "compose:aitoearn", "compose:aitoearn:web"} {
		if _, ok := keys[want]; !ok {
			t.Fatalf("missing key %q in %#v", want, keys)
		}
	}
}

func TestSelectServiceWebPortChoice_SkipsNonWebServiceWithoutProbe(t *testing.T) {
	// 探测关闭（无 LanUrl/WanUrl）：数据库类服务评分为负，不逐服务注册。
	_, ok := selectServiceWebPortChoice(navigationContainerCandidate{
		ContainerID: "db",
		Service:     "postgres",
		Name:        "demo-postgres",
		Image:       "postgres:16",
		Ports:       []int{5432},
	}, settings.Settings{})
	if ok {
		t.Fatal("expected non-web service to be rejected without probe")
	}
}

func TestSelectServiceWebPortChoice_KeepsWebServiceWithoutProbe(t *testing.T) {
	choice, ok := selectServiceWebPortChoice(navigationContainerCandidate{
		ContainerID: "web",
		Service:     "web",
		Name:        "demo-web",
		Image:       "nginx:latest",
		Ports:       []int{8080},
	}, settings.Settings{})
	if !ok || choice.Port != 8080 {
		t.Fatalf("expected web:8080, got %+v ok=%v", choice, ok)
	}
}

func TestSelectServiceWebPortChoice_ExplicitLabelWinsWithoutProbe(t *testing.T) {
	choice, ok := selectServiceWebPortChoice(navigationContainerCandidate{
		ContainerID: "db",
		Service:     "postgres",
		Name:        "demo-postgres",
		Ports:       []int{5432},
		Labels:      map[string]string{"tradis.navigation.port": "5432"},
	}, settings.Settings{})
	if !ok || choice.Port != 5432 {
		t.Fatalf("expected explicit label port 5432, got %+v ok=%v", choice, ok)
	}
}

func TestEscapeSQLLikePattern(t *testing.T) {
	if got := escapeSQLLikePattern(`compose:my_proj%`); got != `compose:my\_proj\%` {
		t.Fatalf("escaped = %q", got)
	}
	if got := escapeSQLLikePattern("compose:plain"); got != "compose:plain" {
		t.Fatalf("plain = %q", got)
	}
}

func TestExplicitNavigationLabelPort(t *testing.T) {
	if got := explicitNavigationLabelPort(map[string]string{"tradis.navigation.port": "9001"}); got != 9001 {
		t.Fatalf("label port = %d", got)
	}
	if got := explicitNavigationLabelPort(map[string]string{"trabis.navigation.port": "8443"}); got != 8443 {
		t.Fatalf("legacy label port = %d", got)
	}
	if got := explicitNavigationLabelPort(nil); got != 0 {
		t.Fatalf("nil labels = %d", got)
	}
	if got := explicitNavigationLabelPort(map[string]string{"tradis.navigation.port": "abc"}); got != 0 {
		t.Fatalf("invalid label = %d", got)
	}
}
