package storage

import (
	"database/sql"
	"fmt"
)

// GetSetting retrieves a setting value by its key.
func (s *Store) GetSetting(key string) (string, error) {
	var value string
	query := `SELECT value FROM settings WHERE key = ?`
	err := s.db.QueryRow(query, key).Scan(&value)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", nil // Not found is not an error, just means no value is set
		}
		return "", fmt.Errorf("failed to get setting %s: %w", key, err)
	}
	return value, nil
}

// SetSetting creates or updates a setting value.
func (s *Store) SetSetting(key, value string) error {
	query := `INSERT INTO settings (key, value) VALUES (?, ?)
			  ON CONFLICT(key) DO UPDATE SET value = excluded.value`
	_, err := s.db.Exec(query, key, value)
	if err != nil {
		return fmt.Errorf("failed to set setting %s: %w", key, err)
	}
	return nil
}
