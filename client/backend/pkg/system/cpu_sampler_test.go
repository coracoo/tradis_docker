package system

import (
	"context"
	"errors"
	"math"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
)

type sequenceCPUReader struct {
	mu      sync.Mutex
	samples []cpu.TimesStat
	errs    []error
	calls   atomic.Int64
}

func (r *sequenceCPUReader) Read(context.Context) (cpu.TimesStat, error) {
	r.calls.Add(1)
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.errs) > 0 {
		err := r.errs[0]
		r.errs = r.errs[1:]
		if err != nil {
			return cpu.TimesStat{}, err
		}
	}
	if len(r.samples) == 0 {
		return cpu.TimesStat{}, errors.New("no CPU sample")
	}
	sample := r.samples[0]
	r.samples = r.samples[1:]
	return sample, nil
}

func TestCPUSamplerCalculatesFixedWindowBusyPercent(t *testing.T) {
	reader := &sequenceCPUReader{samples: []cpu.TimesStat{
		{User: 100, System: 50, Idle: 850},
		{User: 120, System: 60, Idle: 920},
	}}
	sampler := NewCPUSampler(reader, time.Second, 5*time.Second)
	sampler.sample(context.Background(), time.Unix(0, 0))
	sampler.sample(context.Background(), time.Unix(5, 0))

	snapshot := sampler.Current()
	if !snapshot.Ready || math.Abs(snapshot.Percent-30) > 0.001 {
		t.Fatalf("snapshot = %#v, want ready 30%%", snapshot)
	}
	if snapshot.Window != 5*time.Second {
		t.Fatalf("Window = %s, want 5s", snapshot.Window)
	}
}

func TestCPUSamplerConcurrentReadersDoNotMoveBaseline(t *testing.T) {
	reader := &sequenceCPUReader{}
	sampler := NewCPUSampler(reader, time.Second, 5*time.Second)
	sampler.snapshot = CPUSnapshot{Percent: 42.5, Ready: true}

	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if got := sampler.Current().Percent; got != 42.5 {
				t.Errorf("Percent = %v, want 42.5", got)
			}
		}()
	}
	wg.Wait()
	if got := reader.calls.Load(); got != 0 {
		t.Fatalf("Current triggered %d reader calls", got)
	}
}

func TestCPUSamplerIsNotReadyBeforeWindowExists(t *testing.T) {
	reader := &sequenceCPUReader{samples: []cpu.TimesStat{{User: 10, Idle: 90}}}
	sampler := NewCPUSampler(reader, time.Second, 5*time.Second)
	sampler.sample(context.Background(), time.Unix(0, 0))
	if sampler.Current().Ready {
		t.Fatal("sampler became ready before a complete window")
	}
}

func TestCPUSamplerRetainsLastValueAndMarksStaleAfterReadFailure(t *testing.T) {
	reader := &sequenceCPUReader{
		samples: []cpu.TimesStat{
			{User: 10, Idle: 90},
			{User: 20, Idle: 180},
		},
		errs: []error{nil, nil, errors.New("read /proc/stat")},
	}
	sampler := NewCPUSampler(reader, time.Second, 5*time.Second)
	sampler.sample(context.Background(), time.Unix(0, 0))
	sampler.sample(context.Background(), time.Unix(5, 0))
	want := sampler.Current().Percent
	sampler.sample(context.Background(), time.Unix(6, 0))

	got := sampler.Current()
	if got.Percent != want || !got.Stale || !got.Ready {
		t.Fatalf("snapshot = %#v, want retained ready stale value", got)
	}
}
