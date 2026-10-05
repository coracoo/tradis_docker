// Package scheduler 实现基于 cron 表达式的用户自定义定时任务系统。
//
// 设计要点：
//   - 调度规则存 database.scheduled_jobs 表（持久化），执行记录复用 tasks/task_logs 表。
//   - 用 robfig/cron/v3 管理下次触发时间，每个 job 一个 cron.Entry。
//   - per-job 防重入（running map），避免长任务被重复触发。
//   - image_update_check 动作通过 RegisterImageUpdateChecker 注入回调，避免循环依赖 api 包。
//
// 与现有 StartImageUpdateScheduler（api 包内置 ticker）的关系：
//
//	两者并存。内置 ticker 是"系统默认检测间隔"，本系统是"用户自定义任务"。
package scheduler

import (
	"fmt"
	"log"
	"sync"
	"time"

	"dockerpanel/backend/pkg/database"
	"github.com/robfig/cron/v3"
)

// Scheduler 管理所有定时任务的 cron 注册与触发。
type Scheduler struct {
	cron       *cron.Cron
	mu         sync.Mutex
	mutationMu sync.Mutex
	entryMap   map[int64]cron.EntryID // scheduled_jobs.id → cron entry id
	running    map[int64]bool         // 防 per-job 重入
}

// Default 是包级单例，由 Start 初始化，供 api 包调用。
var Default = &Scheduler{
	entryMap: make(map[int64]cron.EntryID),
	running:  make(map[int64]bool),
}

// Start 初始化 cron 实例，从 DB 加载所有 enabled job，并启动后台调度。
// 应在 main 中调用一次。
func Start() {
	c := cron.New(
		cron.WithLogger(cron.PrintfLogger(log.New(cronLogWriter{}, "[cron] ", log.LstdFlags))),
	)
	Default.cron = c

	jobs, err := database.ListEnabledScheduledJobs()
	if err != nil {
		log.Printf("[scheduler] 从数据库加载任务失败: %v", err)
	} else {
		for _, j := range jobs {
			if err := Default.addCronEntry(j); err != nil {
				log.Printf("[scheduler] 注册任务 %d (%s) 失败: %v", j.ID, j.Name, err)
			} else {
				log.Printf("[scheduler] 已加载任务 %d (%s) cron=%s", j.ID, j.Name, j.CronExpr)
			}
		}
	}

	c.Start()
	log.Printf("[scheduler] 已启动，共加载 %d 个任务", len(jobs))
}

// AddJob 为新创建的 job 注册 cron entry（仅当 enabled）。
func (s *Scheduler) AddJob(job database.ScheduledJob) error {
	if !job.Enabled {
		return nil
	}
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	return s.addCronEntry(job)
}

// UpdateJob 更新 cron entry：先移除旧的，再按新定义添加。
func (s *Scheduler) UpdateJob(job database.ScheduledJob) error {
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	if job.Enabled {
		if _, err := cron.ParseStandard(job.CronExpr); err != nil {
			return fmt.Errorf("cron 表达式无效 (%s): %w", job.CronExpr, err)
		}
	}
	s.removeCronEntry(job.ID)
	if !job.Enabled {
		return nil
	}
	return s.addCronEntry(job)
}

// RemoveJob 移除 job 的 cron entry。
func (s *Scheduler) RemoveJob(id int64) {
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	s.removeCronEntry(id)
}

// NextRun returns the next scheduled execution without exposing entryMap to
// callers. cron.Entry is concurrency-safe; the map lookup still needs the
// scheduler mutex because jobs can be replaced while a run is finishing.
func (s *Scheduler) NextRun(id int64) *time.Time {
	s.mu.Lock()
	entryID, ok := s.entryMap[id]
	c := s.cron
	s.mu.Unlock()
	if !ok || c == nil {
		return nil
	}
	entry := c.Entry(entryID)
	if entry.ID == 0 || entry.Next.IsZero() {
		return nil
	}
	next := entry.Next
	return &next
}

// RunNow 手动立即触发一次 job（不经 cron 调度，不受 enabled 限制）。
func (s *Scheduler) RunNow(job database.ScheduledJob) error {
	go func() {
		if err := executeJob(job, true); err != nil {
			log.Printf("[scheduler] 手动触发任务 %d (%s) 失败: %v", job.ID, job.Name, err)
		}
	}()
	return nil
}

func (s *Scheduler) addCronEntry(job database.ScheduledJob) error {
	wrapped := func() {
		// 每次 cron 触发都从 DB 重新读取最新定义，确保 target/payload/enabled 是最新值。
		latest := job
		if err := database.GetScheduledJob(job.ID, &latest); err != nil {
			log.Printf("[scheduler] 读取任务 %d 失败: %v", job.ID, err)
			return
		}
		if !latest.Enabled {
			return
		}
		if err := executeJob(latest, false); err != nil {
			log.Printf("[scheduler] 任务 %d (%s) 执行失败: %v", job.ID, job.Name, err)
		}
	}
	entryID, err := s.cron.AddFunc(job.CronExpr, wrapped)
	if err != nil {
		return fmt.Errorf("cron 表达式无效 (%s): %w", job.CronExpr, err)
	}
	s.mu.Lock()
	s.entryMap[job.ID] = entryID
	s.mu.Unlock()
	return nil
}

func (s *Scheduler) removeCronEntry(id int64) {
	s.mu.Lock()
	entryID, ok := s.entryMap[id]
	if ok {
		delete(s.entryMap, id)
	}
	s.mu.Unlock()
	if ok && s.cron != nil {
		s.cron.Remove(entryID)
	}
}

// cronLogWriter 把 cron 库的日志输出静默丢弃，避免污染主日志。
type cronLogWriter struct{}

func (cronLogWriter) Write(p []byte) (int, error) { return len(p), nil }
