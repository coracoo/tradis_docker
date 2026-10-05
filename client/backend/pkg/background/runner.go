package background

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"dockerpanel/backend/pkg/logging"
	"dockerpanel/backend/pkg/secrets"
)

type Task func(context.Context) error

type Runner struct {
	ctx      context.Context
	cancel   context.CancelFunc
	mu       sync.Mutex
	running  map[string]struct{}
	stopping bool
	active   int
	idle     chan struct{}
	wg       sync.WaitGroup
}

func New(parent context.Context) *Runner {
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	idle := make(chan struct{})
	close(idle)
	return &Runner{
		ctx:     ctx,
		cancel:  cancel,
		running: make(map[string]struct{}),
		idle:    idle,
	}
}

func (r *Runner) Submit(key string, task Task) bool {
	key = strings.TrimSpace(key)
	if key == "" || task == nil {
		return false
	}

	r.mu.Lock()
	if r.stopping {
		r.mu.Unlock()
		return false
	}
	if _, exists := r.running[key]; exists {
		r.mu.Unlock()
		return false
	}
	// Under mu, active matches admitted tasks and idle closes exactly at zero.
	if r.active == 0 {
		r.idle = make(chan struct{})
	}
	r.running[key] = struct{}{}
	r.active++
	r.wg.Add(1)
	r.mu.Unlock()

	go r.run(key, task)
	return true
}

func (r *Runner) Shutdown(ctx context.Context) error {
	r.mu.Lock()
	r.stopping = true
	r.mu.Unlock()
	r.cancel()
	return r.Wait(ctx)
}

func (r *Runner) Wait(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	r.mu.Lock()
	idle := r.idle
	r.mu.Unlock()
	select {
	case <-idle:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (r *Runner) run(key string, task Task) {
	defer r.wg.Done()
	defer r.complete(key)
	defer func() {
		if recovered := recover(); recovered != nil {
			logging.Error("background task panicked", "key", key, "panic", redactLogValue(recovered))
		}
	}()

	if err := task(r.ctx); err != nil {
		logging.Error("background task failed", "key", key, "error", redactLogValue(err))
	}
}

func (r *Runner) complete(key string) {
	r.mu.Lock()
	delete(r.running, key)
	r.active--
	if r.active == 0 {
		close(r.idle)
	}
	r.mu.Unlock()
}

func redactLogValue(value any) string {
	return logging.RedactText(secrets.RedactString(fmt.Sprint(value)))
}
