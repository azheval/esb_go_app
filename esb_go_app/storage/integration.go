package storage

import (
	"database/sql"
	"fmt"
)

// CreateIntegration creates a new integration in the database.
func (s *Store) CreateIntegration(i *Integration) error {
	query := `INSERT INTO integrations (id, name, description) VALUES (?, ?, ?)`
	_, err := s.db.Exec(query, i.ID, i.Name, i.Description)
	if err != nil {
		return fmt.Errorf("failed to create integration: %w", err)
	}
	return nil
}

// GetIntegrationByID retrieves an integration by its ID.
func (s *Store) GetIntegrationByID(id string) (*Integration, error) {
	query := `SELECT id, name, description, created_at, updated_at FROM integrations WHERE id = ?`
	row := s.db.QueryRow(query, id)

	i := &Integration{}
	err := row.Scan(&i.ID, &i.Name, &i.Description, &i.CreatedAt, &i.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get integration by ID: %w", err)
	}
	return i, nil
}

// GetAllIntegrations retrieves all integrations from the database.
func (s *Store) GetAllIntegrations() ([]Integration, error) {
	query := `SELECT id, name, description, created_at, updated_at FROM integrations ORDER BY name ASC`
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to get all integrations: %w", err)
	}
	defer rows.Close()

	var integrations []Integration
	for rows.Next() {
		var i Integration
		if err := rows.Scan(&i.ID, &i.Name, &i.Description, &i.CreatedAt, &i.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan integration row: %w", err)
		}
		integrations = append(integrations, i)
	}
	return integrations, nil
}

// UpdateIntegration updates an existing integration in the database.
func (s *Store) UpdateIntegration(i *Integration) error {
	query := `UPDATE integrations SET name = ?, description = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`
	_, err := s.db.Exec(query, i.Name, i.Description, i.ID)
	if err != nil {
		return fmt.Errorf("failed to update integration: %w", err)
	}
	return nil
}

// DeleteIntegration deletes an integration by its ID.
// Note: This does not delete associated routes or collectors, it just nullifies the foreign key.
func (s *Store) DeleteIntegration(id string) error {
	query := `DELETE FROM integrations WHERE id = ?`
	_, err := s.db.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to delete integration: %w", err)
	}
	return nil
}
