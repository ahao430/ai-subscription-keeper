package store

import (
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Task types.
const (
	TaskTypeWarmup  = "warmup"
	TaskTypeWebhook = "webhook"
)

// Task is a scheduled warmup or webhook job.
type Task struct {
	ID                   string    `json:"id"`
	Name                 string    `json:"name"`
	Type                 string    `json:"type"`
	ModelServiceID       string    `json:"model_service_id"`
	Model                string    `json:"model"`
	Prompt               string    `json:"prompt"`
	Cron                 string    `json:"cron"`
	Timezone             string    `json:"timezone"`
	WebhookConfig        string    `json:"webhook_config"`
	RetryCount           int       `json:"retry_count"`
	RetryIntervalMinutes int       `json:"retry_interval_min"`
	// NotificationChannelIDs 支持多个通知渠道；DB 列中以逗号分隔存储（兼容历史单值）。
	NotificationChannelIDs []string  `json:"notification_channel_ids"`
	Enabled                bool      `json:"enabled"`
	CreatedAt              time.Time `json:"created_at"`
	UpdatedAt              time.Time `json:"updated_at"`
}

// splitChannelIDs 解析 DB 中的逗号分隔渠道 ID。
func splitChannelIDs(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func joinChannelIDs(ids []string) string {
	return strings.Join(ids, ",")
}

const taskCols = `id, name, type, model_service_id, model, prompt, cron, timezone,
	webhook_config, retry_count, retry_interval_min, notification_channel_id, enabled, created_at, updated_at`

func scanTask(row interface{ Scan(...any) error }) (*Task, error) {
	var t Task
	var enabled int
	var channelIDs string
	var createdAt, updatedAt string
	if err := row.Scan(&t.ID, &t.Name, &t.Type, &t.ModelServiceID, &t.Model, &t.Prompt, &t.Cron, &t.Timezone,
		&t.WebhookConfig, &t.RetryCount, &t.RetryIntervalMinutes, &channelIDs, &enabled,
		&createdAt, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	t.NotificationChannelIDs = splitChannelIDs(channelIDs)
	t.Enabled = enabled == 1
	t.CreatedAt, t.UpdatedAt = parseTime(createdAt), parseTime(updatedAt)
	return &t, nil
}

func (s *Store) ListTasks() ([]*Task, error) {
	rows, err := s.db.Query(`SELECT ` + taskCols + ` FROM tasks ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Task
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) GetTask(id string) (*Task, error) {
	row := s.db.QueryRow(`SELECT `+taskCols+` FROM tasks WHERE id=?`, id)
	return scanTask(row)
}

func (s *Store) CreateTask(t *Task) error {
	now := time.Now().UTC()
	t.ID = uuid.NewString()
	t.CreatedAt, t.UpdatedAt = now, now
	if t.Timezone == "" {
		t.Timezone = "Asia/Shanghai"
	}
	if t.Prompt == "" {
		t.Prompt = "hi"
	}
	_, err := s.db.Exec(`INSERT INTO tasks (`+taskCols+`) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		t.ID, t.Name, t.Type, t.ModelServiceID, t.Model, t.Prompt, t.Cron, t.Timezone,
		orDefault(t.WebhookConfig, "{}"), t.RetryCount, t.RetryIntervalMinutes, joinChannelIDs(t.NotificationChannelIDs),
		boolInt(t.Enabled), t.CreatedAt.Format(time.RFC3339), t.UpdatedAt.Format(time.RFC3339))
	return err
}

func (s *Store) UpdateTask(t *Task) error {
	t.UpdatedAt = time.Now().UTC()
	if t.Timezone == "" {
		t.Timezone = "Asia/Shanghai"
	}
	res, err := s.db.Exec(`UPDATE tasks SET name=?, type=?, model_service_id=?, model=?, prompt=?, cron=?, timezone=?,
		webhook_config=?, retry_count=?, retry_interval_min=?, notification_channel_id=?, enabled=?, updated_at=? WHERE id=?`,
		t.Name, t.Type, t.ModelServiceID, t.Model, t.Prompt, t.Cron, t.Timezone,
		orDefault(t.WebhookConfig, "{}"), t.RetryCount, t.RetryIntervalMinutes, joinChannelIDs(t.NotificationChannelIDs),
		boolInt(t.Enabled), t.UpdatedAt.Format(time.RFC3339), t.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeleteTask(id string) error {
	// Cascade delete executions and their attempts before removing the task.
	s.db.Exec(`DELETE FROM execution_attempts WHERE execution_id IN (SELECT id FROM executions WHERE task_id=?)`, id)
	s.db.Exec(`DELETE FROM executions WHERE task_id=?`, id)
	_, err := s.db.Exec(`DELETE FROM tasks WHERE id=?`, id)
	return err
}
