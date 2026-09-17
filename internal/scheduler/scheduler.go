// Package scheduler drives task execution on cron schedules. Timezone is part
// of each task's schedule (CRON_TZ prefix), defaulting to Asia/Shanghai.
package scheduler

import (
	"fmt"
	"sync"
	"time"

	"github.com/robfig/cron/v3"

	"ai-subscription-keeper/internal/store"
	"ai-subscription-keeper/internal/task"
)

type Scheduler struct {
	cron *cron.Cron
	st   *store.Store
	exec *task.Executor

	mu      sync.Mutex
	entries map[string]cron.EntryID
}

func New(st *store.Store, exec *task.Executor) *Scheduler {
	return &Scheduler{
		// Recover panicking jobs so one bad task never kills the loop.
		cron:    cron.New(cron.WithChain(cron.Recover(cron.DefaultLogger))),
		st:      st,
		exec:    exec,
		entries: map[string]cron.EntryID{},
	}
}

// Start loads every enabled task from the store and schedules it.
func (s *Scheduler) Start() error {
	tasks, err := s.st.ListTasks()
	if err != nil {
		return err
	}
	for _, t := range tasks {
		if t.Enabled {
			if err := s.Schedule(t); err != nil {
				fmt.Printf("[scheduler] 任务 %s 调度失败: %v\n", t.Name, err)
			}
		}
	}
	s.cron.Start()
	return nil
}

func (s *Scheduler) Stop() {
	ctx := s.cron.Stop()
	<-ctx.Done()
}

// Schedule (re)registers a task; it replaces any previous entry.
func (s *Scheduler) Schedule(t *store.Task) error {
	if t.Cron == "" {
		return nil
	}
	tz := t.Timezone
	if tz == "" {
		tz = "Asia/Shanghai"
	}
	spec := fmt.Sprintf("CRON_TZ=%s %s", tz, t.Cron)

	s.mu.Lock()
	defer s.mu.Unlock()
	if id, ok := s.entries[t.ID]; ok {
		s.cron.Remove(id)
		delete(s.entries, t.ID)
	}
	id, err := s.cron.AddFunc(spec, func() {
		// Re-read the task so cron fires use the latest config.
		latest, err := s.st.GetTask(t.ID)
		if err != nil {
			fmt.Printf("[scheduler] 任务不存在: %s\n", t.ID)
			return
		}
		if !latest.Enabled {
			return
		}
		if err := s.exec.RunAsync(latest); err != nil {
			fmt.Printf("[scheduler] 任务 %s 跳过: %v\n", latest.Name, err)
		}
	})
	if err != nil {
		return fmt.Errorf("无效的 Cron 表达式: %w", err)
	}
	s.entries[t.ID] = id
	return nil
}

func (s *Scheduler) Unschedule(taskID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if id, ok := s.entries[taskID]; ok {
		s.cron.Remove(id)
		delete(s.entries, taskID)
	}
}

// NextRun reports the next fire time of a scheduled task.
func (s *Scheduler) NextRun(taskID string) (time.Time, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := s.entries[taskID]
	if !ok {
		return time.Time{}, false
	}
	for _, e := range s.cron.Entries() {
		if e.ID == id {
			return e.Next, true
		}
	}
	return time.Time{}, false
}
