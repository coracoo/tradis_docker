package scheduler

import (
	"context"
	"strings"
	"sync"
	"testing"

	"dockerpanel/backend/pkg/database"

	"github.com/robfig/cron/v3"
)

func newSchedulerForTest() *Scheduler {
	return &Scheduler{
		cron:     cron.New(),
		entryMap: make(map[int64]cron.EntryID),
		running:  make(map[int64]bool),
	}
}

func TestContainerActionGuardRejectsBeforeDockerAccess(t *testing.T) {
	original := containerActionGuard
	containerActionGuard = func(context.Context, string) error {
		return context.Canceled
	}
	t.Cleanup(func() { containerActionGuard = original })

	seq := int64(0)
	err := runContainerAction(context.Background(), database.ScheduledJob{
		TaskType: TaskContainerStop,
		Target:   "self-container",
	}, "unused-task", &seq)
	if err == nil || !strings.Contains(err.Error(), "禁止") {
		t.Fatalf("guard error = %v, want protected-container rejection", err)
	}
}

func TestUpdateJobInvalidCronKeepsExistingEntry(t *testing.T) {
	scheduler := newSchedulerForTest()
	job := database.ScheduledJob{ID: 7, Name: "test", CronExpr: "0 * * * *", Enabled: true}
	if err := scheduler.AddJob(job); err != nil {
		t.Fatal(err)
	}
	originalEntry := scheduler.entryMap[job.ID]

	job.CronExpr = "not-a-cron"
	if err := scheduler.UpdateJob(job); err == nil {
		t.Fatal("expected invalid cron error")
	}
	if got := scheduler.entryMap[job.ID]; got != originalEntry {
		t.Fatalf("invalid update replaced or removed the active entry: got %d, want %d", got, originalEntry)
	}
}

func TestNextRunReadsRegisteredEntry(t *testing.T) {
	scheduler := newSchedulerForTest()
	job := database.ScheduledJob{ID: 8, Name: "test", CronExpr: "0 * * * *", Enabled: true}
	if err := scheduler.AddJob(job); err != nil {
		t.Fatal(err)
	}
	scheduler.cron.Start()
	defer scheduler.cron.Stop()

	if next := scheduler.NextRun(job.ID); next == nil {
		t.Fatal("expected next run for registered job")
	}
}

func TestConcurrentJobReplacementLeavesOneEntry(t *testing.T) {
	s := newSchedulerForTest()
	job := database.ScheduledJob{ID: 9, CronExpr: "0 * * * *", Enabled: true}
	if err := s.AddJob(job); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 20 {
				if err := s.UpdateJob(job); err != nil {
					t.Error(err)
				}
				_ = s.NextRun(job.ID)
			}
		}()
	}
	wg.Wait()
	if entries := s.cron.Entries(); len(entries) != 1 {
		t.Fatalf("replacement left %d cron entries, want 1", len(entries))
	}
}
