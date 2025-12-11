package storage

import (
	"database/sql"
	"fmt"
)

// CreateApplication.
func (s *Store) CreateApplication(app *Application) error {
	query := `INSERT INTO applications (id, name, client_secret, id_token) VALUES (?, ?, ?, ?)`
	_, err := s.db.Exec(query, app.ID, app.Name, app.ClientSecret, app.IDToken)
	if err != nil {
		return fmt.Errorf("failed to create application: %w", err)
	}
	return nil
}

// GetApplicationByName
func (s *Store) GetApplicationByName(name string) (*Application, error) {
	query := `SELECT id, name, client_secret, id_token, created_at, updated_at FROM applications WHERE name = ?`
	row := s.db.QueryRow(query, name)

	app := &Application{}
	err := row.Scan(&app.ID, &app.Name, &app.ClientSecret, &app.IDToken, &app.CreatedAt, &app.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get application by name: %w", err)
	}
	return app, nil
}

// GetApplicationByID
func (s *Store) GetApplicationByID(id string) (*Application, error) {
	query := `SELECT id, name, client_secret, id_token, created_at, updated_at FROM applications WHERE id = ?`
	row := s.db.QueryRow(query, id)

	app := &Application{}
	err := row.Scan(&app.ID, &app.Name, &app.ClientSecret, &app.IDToken, &app.CreatedAt, &app.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get application by id: %w", err)
	}
	return app, nil
}

// GetApplicationByIDToken
func (s *Store) GetApplicationByIDToken(token string) (*Application, error) {
	query := `SELECT id, name, client_secret, id_token, created_at, updated_at FROM applications WHERE id_token = ?`
	row := s.db.QueryRow(query, token)

	app := &Application{}
	err := row.Scan(&app.ID, &app.Name, &app.ClientSecret, &app.IDToken, &app.CreatedAt, &app.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get application by token: %w", err)
	}
	return app, nil
}

// GetAllApplications
func (s *Store) GetAllApplications() ([]Application, error) {
	query := `SELECT id, name, client_secret, id_token, created_at, updated_at FROM applications ORDER BY created_at DESC`
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to get all applications: %w", err)
	}
	defer rows.Close()

	var apps []Application
	for rows.Next() {
		var app Application
		if err := rows.Scan(&app.ID, &app.Name, &app.ClientSecret, &app.IDToken, &app.CreatedAt, &app.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan application row: %w", err)
		}
		apps = append(apps, app)
	}

	return apps, nil
}

// UpdateApplication
func (s *Store) UpdateApplication(app *Application) error {
	query := `UPDATE applications SET name = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`
	_, err := s.db.Exec(query, app.Name, app.ID)
	if err != nil {
		return fmt.Errorf("failed to update application: %w", err)
	}
	return nil
}

// DeleteApplication
func (s *Store) DeleteApplication(id string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	if _, err := tx.Exec("DELETE FROM channels WHERE application_id = ?", id); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("failed to delete associated channels: %w", err)
	}

	if _, err := tx.Exec("DELETE FROM applications WHERE id = ?", id); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("failed to delete application: %w", err)
	}

	return tx.Commit()
}
