package api

import (
	"testing"
	"time"

	"dockerpanel/backend/pkg/system"
)

func TestSystemCPUResponseFieldsUseTheSharedSnapshot(t *testing.T) {
	sampledAt := time.Unix(1_700_000_000, 0)
	want := system.CPUSnapshot{
		Percent:   37.5,
		SampledAt: sampledAt,
		Window:    5 * time.Second,
		Ready:     true,
		Stale:     false,
	}
	SetHostCPUSnapshotProvider(func() system.CPUSnapshot { return want })
	t.Cleanup(func() { SetHostCPUSnapshotProvider(nil) })

	snapshot := currentHostCPUSnapshot()
	info := cpuInfoResponseFields(snapshot)
	stats := cpuStatsResponseFields(snapshot)

	if info["CpuUsage"] != 37.5 || stats["cpu_percent"] != 37.5 {
		t.Fatalf("CPU fields do not share one snapshot: info=%v stats=%v", info, stats)
	}
	if info["CpuSampleWindowMs"] != int64(5000) || stats["cpu_sample_window_ms"] != int64(5000) {
		t.Fatalf("unexpected CPU sample window: info=%v stats=%v", info, stats)
	}
	if info["CpuSampledAt"] != sampledAt.UnixMilli() || stats["cpu_sampled_at"] != sampledAt.UnixMilli() {
		t.Fatalf("unexpected CPU sample timestamp: info=%v stats=%v", info, stats)
	}
}

func TestSystemCPUResponseFieldsExposeNotReadyState(t *testing.T) {
	info := cpuInfoResponseFields(system.CPUSnapshot{})
	stats := cpuStatsResponseFields(system.CPUSnapshot{})
	if info["CpuSampleReady"] != false || stats["cpu_sample_ready"] != false {
		t.Fatalf("not-ready snapshot was reported ready: info=%v stats=%v", info, stats)
	}
}
