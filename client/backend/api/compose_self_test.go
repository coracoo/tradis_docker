package api

import "testing"

func TestIsProtectedImage(t *testing.T) {
	tests := []struct {
		name  string
		image string
		want  bool
	}{
		{name: "client dev", image: "coracoo/tradis:dev", want: true},
		{name: "client version", image: "coracoo/tradis:0.2.3", want: true},
		{name: "client digest", image: "coracoo/tradis@sha256:abc", want: true},
		{name: "server backend legacy tag", image: "coracoo/tradis:server_b_0.2.2", want: false},
		{name: "server frontend legacy tag", image: "coracoo/tradis:server_f_0.2.2", want: false},
		{name: "server merged tag", image: "coracoo/tradis:server-merged-0.2.2", want: false},
		{name: "dedicated server repo", image: "coracoo/tradis-server:dev", want: false},
		{name: "unrelated image", image: "nginx:alpine", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isProtectedImage(tt.image); got != tt.want {
				t.Fatalf("isProtectedImage(%q) = %v, want %v", tt.image, got, tt.want)
			}
		})
	}
}
