package docker

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestUpdateDaemonConfigAtSerializesConcurrentMerges(t *testing.T) {
	path := filepath.Join(t.TempDir(), "daemon.json")
	if err := os.WriteFile(path, []byte(`{"log-driver":"json-file"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		if err := updateDaemonConfigAt(path, &DaemonConfig{RegistryMirrors: []string{"https://mirror.example"}}); err != nil {
			t.Errorf("update mirrors: %v", err)
		}
	}()
	go func() {
		defer wg.Done()
		<-start
		if err := updateDaemonConfigAt(path, &DaemonConfig{Proxies: &ProxyConfig{HTTPProxy: "http://proxy.example"}}); err != nil {
			t.Errorf("update proxy: %v", err)
		}
	}()
	close(start)
	wg.Wait()

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var stored map[string]any
	if err := json.Unmarshal(raw, &stored); err != nil {
		t.Fatalf("daemon.json is not valid JSON: %v", err)
	}
	if stored["log-driver"] != "json-file" {
		t.Fatalf("unknown setting was lost: %#v", stored)
	}
	if _, ok := stored["registry-mirrors"]; !ok {
		t.Fatalf("registry mirrors update was lost: %#v", stored)
	}
	if _, ok := stored["proxies"]; !ok {
		t.Fatalf("proxy update was lost: %#v", stored)
	}
}
