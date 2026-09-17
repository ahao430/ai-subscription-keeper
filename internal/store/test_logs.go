package store

import (
	"database/sql"
	"time"

	"github.com/google/uuid"
)

const (
	TestLogStatusRunning = "running"
	TestLogStatusSuccess = "success"
	TestLogStatusFailed  = "failed"
)

// TestLog is one model-test execution record persisted from the SSE test endpoint.
type TestLog struct {
	ID        string    `json:"id"`
	ServiceID string    `json:"service_id"`
	Model     string    `json:"model"`
	Prompt    string    `json:"prompt"`
	Status    string    `json:"status"`
	Result    string    `json:"result"`
	Error     string    `json:"error"`
	StartedAt time.Time `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

const testLogCols = `id, service_id, model, prompt, status, result, error, started_at, finished_at, created_at`

func scanTestLog(row interface{ Scan(...any) error }) (*TestLog, error) {
	var l TestLog
	var startedAt, finishedAt, createdAt string
	if err := row.Scan(&l.ID, &l.ServiceID, &l.Model, &l.Prompt, &l.Status, &l.Result, &l.Error, &startedAt, &finishedAt, &createdAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}
	l.StartedAt = parseTime(startedAt)
	l.FinishedAt = parseTimePtr(finishedAt)
	l.CreatedAt = parseTime(createdAt)
	return &l, nil
}

func (s *Store) CreateTestLog(l *TestLog) error {
	if l.ID == "" {
		l.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	l.CreatedAt = now
	if l.StartedAt.IsZero() {
		l.StartedAt = now
	}
	_, err := s.db.Exec(`INSERT INTO test_logs (`+testLogCols+`) VALUES (?,?,?,?,?,?,?,?,?,?)`,
		l.ID, l.ServiceID, l.Model, l.Prompt, l.Status, l.Result, l.Error,
		l.StartedAt.Format(time.RFC3339), "", l.CreatedAt.Format(time.RFC3339))
	return err
}

func (s *Store) FinishTestLog(id, status, result, errMsg string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`UPDATE test_logs SET status=?, result=?, error=?, finished_at=? WHERE id=?`,
		status, result, errMsg, now, id)
	return err
}

func (s *Store) ListTestLogs(serviceID string, limit int) ([]*TestLog, error) {
	if limit < 1 || limit > 200 {
		limit = 50
	}
	rows, err := s.db.Query(`SELECT `+testLogCols+` FROM test_logs WHERE service_id=? ORDER BY created_at DESC LIMIT ?`, serviceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*TestLog
	for rows.Next() {
		l, err := scanTestLog(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func (s *Store) DeleteTestLogs(serviceID string) error {
	_, err := s.db.Exec(`DELETE FROM test_logs WHERE service_id=?`, serviceID)
	return err
}

func (s *Store) PruneTestLogs(olderThan time.Time) (int64, error) {
	cutoff := olderThan.UTC().Format(time.RFC3339)
	res, err := s.db.Exec(`DELETE FROM test_logs WHERE created_at < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}
