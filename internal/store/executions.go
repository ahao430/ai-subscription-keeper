package store

import (
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
)

// Execution statuses.
const (
	ExecStatusRunning = "running"
	ExecStatusSuccess = "success"
	ExecStatusFailed  = "failed"
)

type Execution struct {
	ID           string     `json:"id"`
	TaskID       string     `json:"task_id"`
	Status       string     `json:"status"`
	StartedAt    time.Time  `json:"started_at"`
	FinishedAt   *time.Time `json:"finished_at,omitempty"`
	AttemptCount int        `json:"attempt_count"`
	Result       string     `json:"result"`
	Error        string     `json:"error"`
	CreatedAt    time.Time  `json:"created_at"`
}

type ExecutionAttempt struct {
	ID          string     `json:"id"`
	ExecutionID string     `json:"execution_id"`
	Attempt     int        `json:"attempt"`
	StartedAt   time.Time  `json:"started_at"`
	FinishedAt  *time.Time `json:"finished_at,omitempty"`
	ModelStatus string     `json:"model_status"`
	QuotaStatus string     `json:"quota_status"`
	HTTPStatus  int        `json:"http_status"`
	Result      string     `json:"result"`
	Error       string     `json:"error"`
}

const executionCols = `id, task_id, status, started_at, finished_at, attempt_count, result, error, created_at`

func scanExecution(row interface{ Scan(...any) error }) (*Execution, error) {
	var e Execution
	var startedAt, finishedAt, createdAt string
	if err := row.Scan(&e.ID, &e.TaskID, &e.Status, &startedAt, &finishedAt, &e.AttemptCount, &e.Result, &e.Error, &createdAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	e.StartedAt, e.CreatedAt = parseTime(startedAt), parseTime(createdAt)
	e.FinishedAt = parseTimePtr(finishedAt)
	return &e, nil
}

func (s *Store) CreateExecution(e *Execution) error {
	e.ID = uuid.NewString()
	e.CreatedAt = time.Now().UTC()
	if e.Status == "" {
		e.Status = ExecStatusRunning
	}
	e.StartedAt = e.CreatedAt
	_, err := s.db.Exec(`INSERT INTO executions (id, task_id, status, started_at, finished_at, attempt_count, result, error, created_at)
		VALUES (?,?,?,?,?,?,?,?,?)`,
		e.ID, e.TaskID, e.Status, e.StartedAt.Format(time.RFC3339), "", 0, e.Result, e.Error, e.CreatedAt.Format(time.RFC3339))
	return err
}

func (s *Store) FinishExecution(id, status, result, errMsg string, attempts int, finishedAt time.Time) error {
	_, err := s.db.Exec(`UPDATE executions SET status=?, finished_at=?, attempt_count=?, result=?, error=? WHERE id=?`,
		status, finishedAt.UTC().Format(time.RFC3339), attempts, result, errMsg, id)
	return err
}

func (s *Store) CreateExecutionAttempt(a *ExecutionAttempt) (string, error) {
	a.ID = uuid.NewString()
	a.StartedAt = time.Now().UTC()
	_, err := s.db.Exec(`INSERT INTO execution_attempts (id, execution_id, attempt, started_at, finished_at, model_status, quota_status, http_status, result, error)
		VALUES (?,?,?,?,?,?,?,?,?,?)`,
		a.ID, a.ExecutionID, a.Attempt, a.StartedAt.Format(time.RFC3339), "", a.ModelStatus, a.QuotaStatus, a.HTTPStatus, a.Result, a.Error)
	return a.ID, err
}

func (s *Store) FinishExecutionAttempt(id, modelStatus, quotaStatus string, httpStatus int, result, errMsg string) error {
	_, err := s.db.Exec(`UPDATE execution_attempts SET finished_at=?, model_status=?, quota_status=?, http_status=?, result=?, error=? WHERE id=?`,
		time.Now().UTC().Format(time.RFC3339), modelStatus, quotaStatus, httpStatus, result, errMsg, id)
	return err
}

func (s *Store) GetExecution(id string) (*Execution, error) {
	return scanExecution(s.db.QueryRow(`SELECT `+executionCols+` FROM executions WHERE id=?`, id))
}

// LatestExecution returns the most recent execution of a task (dashboard 列表用).
func (s *Store) LatestExecution(taskID string) (*Execution, error) {
	return scanExecution(s.db.QueryRow(
		`SELECT `+executionCols+` FROM executions WHERE task_id=? ORDER BY created_at DESC LIMIT 1`, taskID))
}

func (s *Store) ListExecutions(taskID string, limit int) ([]*Execution, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.db.Query(`SELECT `+executionCols+` FROM executions WHERE task_id=? ORDER BY created_at DESC LIMIT ?`, taskID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Execution
	for rows.Next() {
		e, err := scanExecution(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *Store) ListExecutionAttempts(executionID string) ([]*ExecutionAttempt, error) {
	rows, err := s.db.Query(`SELECT id, execution_id, attempt, started_at, finished_at, model_status, quota_status, http_status, result, error
		FROM execution_attempts WHERE execution_id=? ORDER BY attempt`, executionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*ExecutionAttempt
	for rows.Next() {
		var a ExecutionAttempt
		var startedAt, finishedAt string
		if err := rows.Scan(&a.ID, &a.ExecutionID, &a.Attempt, &startedAt, &finishedAt, &a.ModelStatus, &a.QuotaStatus, &a.HTTPStatus, &a.Result, &a.Error); err != nil {
			return nil, err
		}
		a.StartedAt = parseTime(startedAt)
		a.FinishedAt = parseTimePtr(finishedAt)
		out = append(out, &a)
	}
	return out, rows.Err()
}

// DeleteExecutions removes all executions and their attempts for a task.
func (s *Store) DeleteExecutions(taskID string) error {
	s.db.Exec(`DELETE FROM execution_attempts WHERE execution_id IN (SELECT id FROM executions WHERE task_id=?)`, taskID)
	_, err := s.db.Exec(`DELETE FROM executions WHERE task_id=?`, taskID)
	return err
}

// PruneExecutions deletes executions older than the given threshold.
func (s *Store) PruneExecutions(olderThan time.Time) (int64, error) {
	cutoff := olderThan.UTC().Format(time.RFC3339)
	// First delete orphaned attempts.
	s.db.Exec(`DELETE FROM execution_attempts WHERE execution_id IN (SELECT id FROM executions WHERE created_at < ?)`, cutoff)
	res, err := s.db.Exec(`DELETE FROM executions WHERE created_at < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}
