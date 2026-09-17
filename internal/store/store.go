package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

// Store wraps the SQLite database holding all persistent state.
type Store struct {
	db *sql.DB
}

func Open(dataDir string) (*Store, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	dsn := "file:" + filepath.Join(dataDir, "keeper.db") + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// SQLite handles one writer at a time; keep the pool small to avoid
	// SQLITE_BUSY churn under concurrent refreshes.
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS provider_types (
			code TEXT PRIMARY KEY,
			name TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS model_services (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			provider_type TEXT NOT NULL,
			credential TEXT NOT NULL DEFAULT '',
			config TEXT NOT NULL DEFAULT '{}',
			models TEXT NOT NULL DEFAULT '[]',
			default_warmup_model TEXT NOT NULL DEFAULT '',
			default_test_model TEXT NOT NULL DEFAULT '',
			default_test_prompt TEXT NOT NULL DEFAULT 'hi',
			quota_labels TEXT NOT NULL DEFAULT '{}',
			sort_order INTEGER NOT NULL DEFAULT 0,
			enabled INTEGER NOT NULL DEFAULT 1,
			last_quota TEXT NOT NULL DEFAULT '',
			last_quota_at TEXT NOT NULL DEFAULT '',
			last_quota_error TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS tasks (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			type TEXT NOT NULL,
			model_service_id TEXT NOT NULL DEFAULT '',
			model TEXT NOT NULL DEFAULT '',
			prompt TEXT NOT NULL DEFAULT 'hi',
			cron TEXT NOT NULL DEFAULT '',
			timezone TEXT NOT NULL DEFAULT 'Asia/Shanghai',
			webhook_config TEXT NOT NULL DEFAULT '{}',
			retry_count INTEGER NOT NULL DEFAULT 0,
			retry_interval_min INTEGER NOT NULL DEFAULT 5,
			notification_channel_id TEXT NOT NULL DEFAULT '',
			enabled INTEGER NOT NULL DEFAULT 1,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS executions (
			id TEXT PRIMARY KEY,
			task_id TEXT NOT NULL,
			status TEXT NOT NULL,
			started_at TEXT NOT NULL,
			finished_at TEXT NOT NULL DEFAULT '',
			attempt_count INTEGER NOT NULL DEFAULT 0,
			result TEXT NOT NULL DEFAULT '',
			error TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS execution_attempts (
			id TEXT PRIMARY KEY,
			execution_id TEXT NOT NULL,
			attempt INTEGER NOT NULL,
			started_at TEXT NOT NULL,
			finished_at TEXT NOT NULL DEFAULT '',
			model_status TEXT NOT NULL DEFAULT '',
			quota_status TEXT NOT NULL DEFAULT '',
			http_status INTEGER NOT NULL DEFAULT 0,
			result TEXT NOT NULL DEFAULT '',
			error TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS notification_channels (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			type TEXT NOT NULL,
			config TEXT NOT NULL DEFAULT '',
			enabled INTEGER NOT NULL DEFAULT 1,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS test_logs (
			id TEXT PRIMARY KEY,
			service_id TEXT NOT NULL,
			model TEXT NOT NULL DEFAULT '',
			prompt TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL,
			result TEXT NOT NULL DEFAULT '',
			error TEXT NOT NULL DEFAULT '',
			started_at TEXT NOT NULL,
			finished_at TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_executions_task ON executions(task_id, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_attempts_exec ON execution_attempts(execution_id, attempt)`,
		`CREATE INDEX IF NOT EXISTS idx_test_logs_service ON test_logs(service_id, created_at DESC)`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
	}
	// Add billing_type column to model_services (idempotent — ignore "duplicate column" error).
	if _, err := s.db.Exec(`ALTER TABLE model_services ADD COLUMN billing_type TEXT NOT NULL DEFAULT 'subscription'`); err != nil {
		if !strings.Contains(err.Error(), "duplicate column") {
			return fmt.Errorf("migrate billing_type: %w", err)
		}
	}
	return nil
}
