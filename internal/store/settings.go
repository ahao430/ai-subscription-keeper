package store

import (
	"database/sql"
	"errors"
)

const (
	settingKeyProxy  = "proxy"
	settingKeyWebdav = "webdav"
)

// GetProxySetting returns the stored proxy JSON, or the zero-value default.
func (s *Store) GetProxySetting() (string, error) {
	var v string
	err := s.db.QueryRow(`SELECT value FROM settings WHERE key=?`, settingKeyProxy).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return `{"mode":"none","url":""}`, nil
	}
	return v, err
}

func (s *Store) SetProxySetting(value string) error {
	_, err := s.db.Exec(`INSERT INTO settings (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value`, settingKeyProxy, value)
	return err
}

// GetWebdavSetting returns the stored (encrypted) WebDAV config blob, or "".
func (s *Store) GetWebdavSetting() (string, error) {
	var v string
	err := s.db.QueryRow(`SELECT value FROM settings WHERE key=?`, settingKeyWebdav).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return v, err
}

func (s *Store) SetWebdavSetting(value string) error {
	_, err := s.db.Exec(`INSERT INTO settings (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value`, settingKeyWebdav, value)
	return err
}

// GetSetting / SetSetting are generic accessors for auxiliary keys
// (e.g. webdav_last_sync).
func (s *Store) GetSetting(key string) (string, error) {
	var v string
	err := s.db.QueryRow(`SELECT value FROM settings WHERE key=?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return v, err
}

func (s *Store) SetSetting(key, value string) error {
	_, err := s.db.Exec(`INSERT INTO settings (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
	return err
}
