package api

import (
	"os"
	"strings"
	"testing"
)

func TestVolumeCreateDoesNotAcceptDeprecatedOptionsAlias(t *testing.T) {
	source, err := os.ReadFile("volume.go")
	if err != nil {
		t.Fatalf("read volume source: %v", err)
	}
	if strings.Contains(string(source), "`json:\"options\"`") {
		t.Fatal("deprecated volume options alias is still accepted")
	}
}
