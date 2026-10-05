package system

import (
	"context"
	"errors"
	"math"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
)

type CPUSnapshot struct {
	Percent   float64
	SampledAt time.Time
	Window    time.Duration
	Ready     bool
	Stale     bool
}

type CPUTimesReader interface {
	Read(context.Context) (cpu.TimesStat, error)
}

type gopsutilCPUTimesReader struct{}

func (gopsutilCPUTimesReader) Read(ctx context.Context) (cpu.TimesStat, error) {
	select {
	case <-ctx.Done():
		return cpu.TimesStat{}, ctx.Err()
	default:
	}
	times, err := cpu.Times(false)
	if err != nil {
		return cpu.TimesStat{}, err
	}
	if len(times) == 0 {
		return cpu.TimesStat{}, errors.New("CPU times are unavailable")
	}
	return times[0], nil
}

type cpuSample struct {
	at    time.Time
	times cpu.TimesStat
}

type CPUSampler struct {
	reader   CPUTimesReader
	interval time.Duration
	window   time.Duration

	sampleMu sync.Mutex
	mu       sync.RWMutex
	samples  []cpuSample
	snapshot CPUSnapshot
}

func NewCPUSampler(reader CPUTimesReader, interval, window time.Duration) *CPUSampler {
	if reader == nil {
		reader = gopsutilCPUTimesReader{}
	}
	if interval <= 0 {
		interval = time.Second
	}
	if window <= 0 {
		window = 5 * time.Second
	}
	if window < interval {
		window = interval
	}
	return &CPUSampler{reader: reader, interval: interval, window: window}
}

func (s *CPUSampler) Current() CPUSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.snapshot
}

func (s *CPUSampler) Run(ctx context.Context) error {
	s.sample(ctx, time.Now())
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			s.sample(ctx, time.Now())
		}
	}
}

func (s *CPUSampler) sample(ctx context.Context, sampledAt time.Time) {
	s.sampleMu.Lock()
	defer s.sampleMu.Unlock()

	times, err := s.reader.Read(ctx)
	if err != nil {
		s.recordFailure()
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.samples = append(s.samples, cpuSample{at: sampledAt, times: times})
	s.snapshot.Stale = false

	baselineIndex := -1
	for i := range s.samples {
		if sampledAt.Sub(s.samples[i].at) >= s.window {
			baselineIndex = i
			continue
		}
		break
	}
	if baselineIndex < 0 {
		return
	}

	baseline := s.samples[baselineIndex]
	totalDelta := cpuTotal(times) - cpuTotal(baseline.times)
	idleDelta := (times.Idle + times.Iowait) - (baseline.times.Idle + baseline.times.Iowait)
	if totalDelta <= 0 {
		s.snapshot.Stale = true
		return
	}
	busyDelta := math.Max(0, math.Min(totalDelta, totalDelta-idleDelta))
	s.snapshot = CPUSnapshot{
		Percent:   busyDelta / totalDelta * 100,
		SampledAt: sampledAt,
		Window:    sampledAt.Sub(baseline.at),
		Ready:     true,
		Stale:     false,
	}
	if baselineIndex > 0 {
		s.samples = append([]cpuSample(nil), s.samples[baselineIndex:]...)
	}
}

func (s *CPUSampler) recordFailure() {
	s.mu.Lock()
	s.snapshot.Stale = true
	s.mu.Unlock()
}

func cpuTotal(times cpu.TimesStat) float64 {
	return times.User + times.System + times.Idle + times.Nice + times.Iowait +
		times.Irq + times.Softirq + times.Steal
}
