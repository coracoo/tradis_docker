package api

import "testing"

func TestNormalizeTerminalUser(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "container default", input: "", want: ""},
		{name: "root", input: "root", want: "root"},
		{name: "nobody", input: "nobody", want: "nobody"},
		{name: "custom name", input: "  app-user  ", want: "app-user"},
		{name: "numeric uid and gid", input: "1000:1000", want: "1000:1000"},
		{name: "reject command syntax", input: "root;id", wantErr: true},
		{name: "reject whitespace", input: "app user", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeTerminalUser(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("normalizeTerminalUser(%q) expected error", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("normalizeTerminalUser(%q): %v", tt.input, err)
			}
			if got != tt.want {
				t.Fatalf("normalizeTerminalUser(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestBuildTerminalExecConfigAppliesShellAndUser(t *testing.T) {
	config, err := buildTerminalExecConfig("/bin/bash", "nobody")
	if err != nil {
		t.Fatalf("buildTerminalExecConfig returned error: %v", err)
	}
	if len(config.Cmd) != 1 || config.Cmd[0] != "/bin/bash" {
		t.Fatalf("unexpected shell command: %#v", config.Cmd)
	}
	if config.User != "nobody" {
		t.Fatalf("unexpected user: %q", config.User)
	}
}

func TestBuildTerminalExecConfigUsesPortableDefaults(t *testing.T) {
	config, err := buildTerminalExecConfig("", "")
	if err != nil {
		t.Fatalf("buildTerminalExecConfig returned error: %v", err)
	}
	if len(config.Cmd) != 1 || config.Cmd[0] != "/bin/sh" {
		t.Fatalf("unexpected default shell: %#v", config.Cmd)
	}
	if config.User != "" {
		t.Fatalf("container default user should be empty, got %q", config.User)
	}
}
