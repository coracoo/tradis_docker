package api

import (
	"os"
	"testing"
)

func TestCompareSemver(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"v0.6.10", "v0.6.9", 1},
		{"v0.8.0", "v0.7.10", 1},
		{"v1.0.0", "v1.0.0-beta.1", 1},
		{"v1.0.0-beta.1", "v1.0.0", -1},
		{"0.6.5", "v0.6.5", 0},
	}
	for _, test := range tests {
		if got := compareSemver(test.a, test.b); got != test.want {
			t.Fatalf("compareSemver(%q, %q) = %d, want %d", test.a, test.b, got, test.want)
		}
	}
}

func TestResolveLocalClientVersionUsesEnvironmentFirst(t *testing.T) {
	previousDefault := DefaultClientVersion
	DefaultClientVersion = "v0.6.6"
	t.Cleanup(func() {
		DefaultClientVersion = previousDefault
	})
	if err := os.Setenv("CLIENT_VERSION", "0.8.0"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Unsetenv("CLIENT_VERSION")
	})
	if got := resolveLocalClientVersion(); got != "v0.8.0" {
		t.Fatalf("resolveLocalClientVersion() = %q, want v0.8.0", got)
	}
}

func TestResolveLocalClientVersionUsesBuildDefault(t *testing.T) {
	previousDefault := DefaultClientVersion
	DefaultClientVersion = "0.6.6"
	t.Cleanup(func() {
		DefaultClientVersion = previousDefault
	})
	_ = os.Unsetenv("CLIENT_VERSION")
	_ = os.Unsetenv("DOCKPIER_CLIENT_VERSION")
	if got := resolveLocalClientVersion(); got != "v0.6.6" {
		t.Fatalf("resolveLocalClientVersion() = %q, want v0.6.6", got)
	}
}
