package store

import (
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
)

var ErrNotFound = errors.New("record not found")

// ModelService is one concrete AI account bound to a provider type.
type ModelService struct {
	ID                string     `json:"id"`
	Name              string     `json:"name"`
	ProviderType      string     `json:"provider_type"`
	Credential        string     `json:"-"` // encrypted blob, never serialized to clients
	Config            string     `json:"config"`
	Models            string     `json:"models"`
	DefaultWarmupModel string    `json:"default_warmup_model"`
	DefaultTestModel  string     `json:"default_test_model"`
	DefaultTestPrompt string     `json:"default_test_prompt"`
	QuotaLabels       string     `json:"quota_labels"`
	BillingType       string     `json:"billing_type"` // "subscription" | "pay_as_you_go"
	SortOrder         int        `json:"sort_order"`
	Enabled           bool       `json:"enabled"`
	LastQuota         string     `json:"-"`
	LastQuotaAt       *time.Time `json:"last_quota_at,omitempty"`
	LastQuotaError    string     `json:"last_quota_error,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

const modelServiceCols = `id, name, provider_type, credential, config, models,
	default_warmup_model, default_test_model, default_test_prompt, quota_labels,
	billing_type, sort_order, enabled, last_quota, last_quota_at, last_quota_error, created_at, updated_at`

func scanModelService(row interface{ Scan(...any) error }) (*ModelService, error) {
	var ms ModelService
	var enabled int
	var lastQuota, lastQuotaAt, createdAt, updatedAt string
	if err := row.Scan(&ms.ID, &ms.Name, &ms.ProviderType, &ms.Credential, &ms.Config, &ms.Models,
		&ms.DefaultWarmupModel, &ms.DefaultTestModel, &ms.DefaultTestPrompt, &ms.QuotaLabels,
		&ms.BillingType, &ms.SortOrder, &enabled, &lastQuota, &lastQuotaAt, &ms.LastQuotaError,
		&createdAt, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	ms.Enabled = enabled == 1
	ms.LastQuota = lastQuota
	if ms.BillingType == "" {
		ms.BillingType = "subscription"
	}
	if lastQuotaAt != "" {
		if t, err := time.Parse(time.RFC3339, lastQuotaAt); err == nil {
			ms.LastQuotaAt = &t
		}
	}
	ms.CreatedAt, ms.UpdatedAt = parseTime(createdAt), parseTime(updatedAt)
	return &ms, nil
}

// parseTime parses RFC3339 text columns; zero value on failure. The sqlite
// driver returns TEXT columns as strings, never time.Time.
func parseTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

func parseTimePtr(s string) *time.Time {
	if s == "" {
		return nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return &t
	}
	return nil
}

func (s *Store) ListModelServices() ([]*ModelService, error) {
	rows, err := s.db.Query(`SELECT `+modelServiceCols+` FROM model_services ORDER BY sort_order, created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*ModelService
	for rows.Next() {
		ms, err := scanModelService(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, ms)
	}
	return out, rows.Err()
}

func (s *Store) GetModelService(id string) (*ModelService, error) {
	row := s.db.QueryRow(`SELECT `+modelServiceCols+` FROM model_services WHERE id = ?`, id)
	return scanModelService(row)
}

func (s *Store) CreateModelService(ms *ModelService) error {
	now := time.Now().UTC()
	ms.ID = uuid.NewString()
	ms.CreatedAt, ms.UpdatedAt = now, now
	if ms.DefaultTestPrompt == "" {
		ms.DefaultTestPrompt = "hi"
	}
	if ms.BillingType == "" {
		ms.BillingType = "subscription"
	}
	_, err := s.db.Exec(`INSERT INTO model_services (`+modelServiceCols+`) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		ms.ID, ms.Name, ms.ProviderType, ms.Credential, orDefault(ms.Config, "{}"), orDefault(ms.Models, "[]"),
		ms.DefaultWarmupModel, ms.DefaultTestModel, ms.DefaultTestPrompt, orDefault(ms.QuotaLabels, "{}"),
		ms.BillingType, ms.SortOrder, boolInt(ms.Enabled), "", "", "", ms.CreatedAt.Format(time.RFC3339), ms.UpdatedAt.Format(time.RFC3339))
	return err
}

func (s *Store) UpdateModelService(ms *ModelService) error {
	ms.UpdatedAt = time.Now().UTC()
	if ms.BillingType == "" {
		ms.BillingType = "subscription"
	}
	res, err := s.db.Exec(`UPDATE model_services SET name=?, provider_type=?, credential=?, config=?, models=?,
		default_warmup_model=?, default_test_model=?, default_test_prompt=?, quota_labels=?,
		billing_type=?, sort_order=?, enabled=?, updated_at=? WHERE id=?`,
		ms.Name, ms.ProviderType, ms.Credential, orDefault(ms.Config, "{}"), orDefault(ms.Models, "[]"),
		ms.DefaultWarmupModel, ms.DefaultTestModel, ms.DefaultTestPrompt, orDefault(ms.QuotaLabels, "{}"),
		ms.BillingType, ms.SortOrder, boolInt(ms.Enabled), ms.UpdatedAt.Format(time.RFC3339), ms.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeleteModelService(id string) error {
	// Cascade delete test logs before removing the service.
	s.db.Exec(`DELETE FROM test_logs WHERE service_id=?`, id)
	_, err := s.db.Exec(`DELETE FROM model_services WHERE id=?`, id)
	return err
}

// SaveQuotaResult persists the latest quota payload (wrapper JSON including the
// sanitized raw response) for the dashboard cache.
func (s *Store) SaveQuotaResult(id, payload string, queriedAt time.Time, errMsg string) error {
	at := ""
	if !queriedAt.IsZero() {
		at = queriedAt.UTC().Format(time.RFC3339)
	}
	_, err := s.db.Exec(`UPDATE model_services SET last_quota=?, last_quota_at=?, last_quota_error=?, updated_at=? WHERE id=?`,
		payload, at, errMsg, time.Now().UTC().Format(time.RFC3339), id)
	return err
}

func (s *Store) UpdateModelServiceModels(id, models string) error {
	_, err := s.db.Exec(`UPDATE model_services SET models=?, updated_at=? WHERE id=?`,
		models, time.Now().UTC().Format(time.RFC3339), id)
	return err
}

// ReorderModelServices assigns sequential sort_order values following the
// given id order (dashboard drag & drop).
func (s *Store) ReorderModelServices(ids []string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().UTC().Format(time.RFC3339)
	for i, id := range ids {
		if _, err := tx.Exec(`UPDATE model_services SET sort_order=?, updated_at=? WHERE id=?`, i, now, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
