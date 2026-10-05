package cloudflare

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestShouldTriggerForCDNChange(t *testing.T) {
	tests := []struct {
		name     string
		previous string
		current  string
		want     bool
	}{
		{name: "empty remains empty", previous: "", current: "", want: false},
		{name: "new CDN", previous: "", current: "https://cdn.example.com", want: true},
		{name: "changed CDN", previous: "https://old.example.com", current: "https://new.example.com", want: true},
		{name: "same CDN", previous: "https://cdn.example.com", current: "https://cdn.example.com", want: false},
		{name: "ignore spaces and trailing slash", previous: " https://cdn.example.com/ ", current: "https://cdn.example.com", want: false},
		{name: "removed CDN", previous: "https://cdn.example.com", current: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldTriggerForCDNChange(tt.previous, tt.current); got != tt.want {
				t.Fatalf("shouldTriggerForCDNChange(%q, %q) = %v, want %v", tt.previous, tt.current, got, tt.want)
			}
		})
	}
}

func TestSpeedTestArchiveArch(t *testing.T) {
	tests := []struct {
		goarch  string
		want    string
		wantErr bool
	}{
		{goarch: "amd64", want: "amd64"},
		{goarch: "386", want: "386"},
		{goarch: "arm64", want: "arm64"},
		{goarch: "arm", want: "armv7"},
		{goarch: "riscv64", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.goarch, func(t *testing.T) {
			got, err := speedTestArchiveArch(tt.goarch)
			if (err != nil) != tt.wantErr {
				t.Fatalf("speedTestArchiveArch(%q) error = %v, wantErr %v", tt.goarch, err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("speedTestArchiveArch(%q) = %q, want %q", tt.goarch, got, tt.want)
			}
		})
	}
}

func TestLoadCachedResultUsesFileModificationTime(t *testing.T) {
	t.Setenv("CLOUDFLARE_SPEED_TEST", "true")
	oldBinDir := binDir
	oldBestIPs := GetBestIPs()
	oldLastTestTime := lastSpeedTestTime
	t.Cleanup(func() {
		binDir = oldBinDir
		bestIPsMu.Lock()
		bestIPs = oldBestIPs
		lastSpeedTestTime = oldLastTestTime
		bestIPsMu.Unlock()
	})

	binDir = t.TempDir()
	resultPath := filepath.Join(binDir, DefaultResultFile)
	if err := os.WriteFile(resultPath, []byte("IP,Sent,Received\n1.1.1.1,5,5\n"), 0644); err != nil {
		t.Fatal(err)
	}
	wantTime := time.Now().Add(-6 * time.Hour).Truncate(time.Second)
	if err := os.Chtimes(resultPath, wantTime, wantTime); err != nil {
		t.Fatal(err)
	}

	if err := LoadCachedResult(); err != nil {
		t.Fatal(err)
	}
	if got := GetBestIP(); got != "1.1.1.1" {
		t.Fatalf("GetBestIP() = %q, want 1.1.1.1", got)
	}
	if !lastSpeedTestTime.Equal(wantTime) {
		t.Fatalf("lastSpeedTestTime = %v, want %v", lastSpeedTestTime, wantTime)
	}
}

func TestReportIPFailureRemovesCandidateAndCache(t *testing.T) {
	t.Setenv("CLOUDFLARE_SPEED_TEST", "true")
	oldBinDir := binDir
	oldBestIPs := GetBestIPs()
	oldLastError := lastSpeedTestError
	t.Cleanup(func() {
		binDir = oldBinDir
		bestIPsMu.Lock()
		bestIPs = oldBestIPs
		lastSpeedTestError = oldLastError
		bestIPsMu.Unlock()
	})

	binDir = t.TempDir()
	resultPath := filepath.Join(binDir, DefaultResultFile)
	if err := os.WriteFile(resultPath, []byte("IP\n1.1.1.1\n2.2.2.2\n"), 0644); err != nil {
		t.Fatal(err)
	}
	bestIPsMu.Lock()
	bestIPs = []string{"1.1.1.1", "2.2.2.2"}
	bestIPsMu.Unlock()

	ReportIPFailure("1.1.1.1")

	if got := GetBestIP(); got != "2.2.2.2" {
		t.Fatalf("GetBestIP() = %q, want 2.2.2.2", got)
	}
	if _, err := os.Stat(resultPath); !os.IsNotExist(err) {
		t.Fatalf("result cache should be removed, stat error = %v", err)
	}
}
