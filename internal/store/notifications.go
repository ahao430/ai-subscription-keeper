package store

import (
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
)

// Notification channel types.
const (
	NotifyTypeDingTalk = "dingtalk"
	NotifyTypeWebhook  = "webhook"
	NotifyTypeFeishu    = "feishu"
	NotifyTypeTelegram  = "telegram"
	NotifyTypeWeCom     = "wecom"
	NotifyTypeNtfy      = "ntfy"
	NotifyTypeBark      = "bark"
	NotifyTypeGotify    = "gotify"
	NotifyTypeEmail     = "email"
)

type NotificationChannel struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Type      string    `json:"type"`
	Config    string    `json:"-"` // encrypted blob
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

const channelCols = `id, name, type, config, enabled, created_at, updated_at`

func scanChannel(row interface{ Scan(...any) error }) (*NotificationChannel, error) {
	var c NotificationChannel
	var enabled int
	var createdAt, updatedAt string
	if err := row.Scan(&c.ID, &c.Name, &c.Type, &c.Config, &enabled, &createdAt, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	c.Enabled = enabled == 1
	c.CreatedAt, c.UpdatedAt = parseTime(createdAt), parseTime(updatedAt)
	return &c, nil
}

func (s *Store) ListNotificationChannels() ([]*NotificationChannel, error) {
	rows, err := s.db.Query(`SELECT ` + channelCols + ` FROM notification_channels ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*NotificationChannel
	for rows.Next() {
		c, err := scanChannel(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) GetNotificationChannel(id string) (*NotificationChannel, error) {
	row := s.db.QueryRow(`SELECT `+channelCols+` FROM notification_channels WHERE id=?`, id)
	return scanChannel(row)
}

func (s *Store) CreateNotificationChannel(c *NotificationChannel) error {
	now := time.Now().UTC()
	if c.ID == "" {
		c.ID = uuid.NewString()
	}
	c.CreatedAt, c.UpdatedAt = now, now
	_, err := s.db.Exec(`INSERT INTO notification_channels (`+channelCols+`) VALUES (?,?,?,?,?,?,?)`,
		c.ID, c.Name, c.Type, c.Config, boolInt(c.Enabled), c.CreatedAt.Format(time.RFC3339), c.UpdatedAt.Format(time.RFC3339))
	return err
}

func (s *Store) UpdateNotificationChannel(c *NotificationChannel) error {
	c.UpdatedAt = time.Now().UTC()
	res, err := s.db.Exec(`UPDATE notification_channels SET name=?, type=?, config=?, enabled=?, updated_at=? WHERE id=?`,
		c.Name, c.Type, c.Config, boolInt(c.Enabled), c.UpdatedAt.Format(time.RFC3339), c.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeleteNotificationChannel(id string) error {
	_, err := s.db.Exec(`DELETE FROM notification_channels WHERE id=?`, id)
	return err
}
