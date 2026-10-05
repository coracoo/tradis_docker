package api

import "testing"

func TestParseImageName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantName string
		wantTag  string
	}{
		{name: "plain image defaults latest", input: "nginx", wantName: "nginx", wantTag: "latest"},
		{name: "tagged docker hub image", input: "nginx:1.27", wantName: "nginx", wantTag: "1.27"},
		{name: "registry with port", input: "localhost:5000/team/app:v1", wantName: "localhost:5000/team/app", wantTag: "v1"},
		{name: "registry without tag", input: "registry.example.com/team/app", wantName: "registry.example.com/team/app", wantTag: "latest"},
		{name: "empty tag defaults latest", input: "repo/app:", wantName: "repo/app", wantTag: "latest"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotName, gotTag := parseImageName(tt.input)
			if gotName != tt.wantName || gotTag != tt.wantTag {
				t.Fatalf("parseImageName(%q) = (%q, %q), want (%q, %q)", tt.input, gotName, gotTag, tt.wantName, tt.wantTag)
			}
		})
	}
}
