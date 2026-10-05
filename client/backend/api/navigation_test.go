package api

import "testing"

func TestBrowserSafeNavigationIcon(t *testing.T) {
	tests := []struct {
		name string
		icon string
		want string
	}{
		{name: "loopback vite svg", icon: "http://localhost:50000/vite.svg", want: "container"},
		{name: "remote svg with query", icon: "https://example.com/logo.svg?v=2", want: "container"},
		{name: "remote raster", icon: "https://example.com/logo.png", want: "https://example.com/logo.png"},
		{name: "cached raster", icon: "/data/pic/logo.png", want: "/data/pic/logo.png"},
		{name: "dynamic icon", icon: "lucide:box", want: "lucide:box"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := browserSafeNavigationIcon(tt.icon); got != tt.want {
				t.Fatalf("browserSafeNavigationIcon(%q) = %q, want %q", tt.icon, got, tt.want)
			}
		})
	}
}
