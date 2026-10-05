package system

import (
	"sync"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/load"
	"github.com/shirou/gopsutil/v3/mem"
)

type LoadLevel string

const (
	LoadLevelNormal   LoadLevel = "normal"
	LoadLevelHigh     LoadLevel = "high"
	LoadLevelCritical LoadLevel = "critical"
)

type LoadSnapshot struct {
	CPUPercent     float64
	MemUsedPercent float64
	Load1          float64
	Load5          float64
	Load15         float64
}

var loadSnapshotCache struct {
	mu      sync.Mutex
	expires time.Time
	snap    LoadSnapshot
	level   LoadLevel
}

func GetLoadSnapshot() LoadSnapshot {
	loadSnapshotCache.mu.Lock()
	defer loadSnapshotCache.mu.Unlock()

	now := time.Now()
	if now.Before(loadSnapshotCache.expires) {
		return loadSnapshotCache.snap
	}

	var snap LoadSnapshot
	if vm, err := mem.VirtualMemory(); err == nil && vm != nil {
		snap.MemUsedPercent = vm.UsedPercent
	}
	if pct, err := cpu.Percent(0, false); err == nil && len(pct) > 0 {
		snap.CPUPercent = pct[0]
	}
	if avg, err := load.Avg(); err == nil && avg != nil {
		snap.Load1 = avg.Load1
		snap.Load5 = avg.Load5
		snap.Load15 = avg.Load15
	}

	loadSnapshotCache.snap = snap
	loadSnapshotCache.level = DetermineLoadLevel(snap)
	loadSnapshotCache.expires = now.Add(1 * time.Second)
	return snap
}

func DetermineLoadLevel(s LoadSnapshot) LoadLevel {
	if s.MemUsedPercent >= 92 || s.CPUPercent >= 95 {
		return LoadLevelCritical
	}
	if s.MemUsedPercent >= 85 || s.CPUPercent >= 88 {
		return LoadLevelHigh
	}
	return LoadLevelNormal
}

func CurrentLoadLevel() LoadLevel {
	GetLoadSnapshot()
	loadSnapshotCache.mu.Lock()
	defer loadSnapshotCache.mu.Unlock()
	return loadSnapshotCache.level
}

func SuggestedRefreshSeconds(level LoadLevel) int {
	switch level {
	case LoadLevelCritical:
		return 20
	case LoadLevelHigh:
		return 10
	default:
		return 5
	}
}

