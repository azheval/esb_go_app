package storage

import (
	"database/sql"
	"fmt"
)

// CreateTransformation creates a new transformation in the database.
func (s *Store) CreateTransformation(t *Transformation) error {
	query := `INSERT INTO transformations (id, name, engine, script) VALUES (?, ?, ?, ?)`
	_, err := s.db.Exec(query, t.ID, t.Name, t.Engine, t.Script)
	if err != nil {
		return fmt.Errorf("failed to create transformation: %w", err)
	}
	return nil
}

// GetTransformationByID retrieves a transformation by its ID.
func (s *Store) GetTransformationByID(id string) (*Transformation, error) {
	query := `SELECT id, name, engine, script, created_at, updated_at FROM transformations WHERE id = ?`
	row := s.db.QueryRow(query, id)

	t := &Transformation{}
	err := row.Scan(&t.ID, &t.Name, &t.Engine, &t.Script, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get transformation by ID: %w", err)
	}
	return t, nil
}

// GetTransformationByName retrieves a transformation by its name.
func (s *Store) GetTransformationByName(name string) (*Transformation, error) {
	query := `SELECT id, name, engine, script, created_at, updated_at FROM transformations WHERE name = ?`
	row := s.db.QueryRow(query, name)

	t := &Transformation{}
	err := row.Scan(&t.ID, &t.Name, &t.Engine, &t.Script, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get transformation by name: %w", err)
	}
	return t, nil
}

// GetAllTransformations retrieves all transformations from the database.
func (s *Store) GetAllTransformations() ([]Transformation, error) {
	query := `SELECT id, name, engine, script, created_at, updated_at FROM transformations ORDER BY created_at DESC`
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to get all transformations: %w", err)
	}
	defer rows.Close()

	var transformations []Transformation
	for rows.Next() {
		var t Transformation
		if err := rows.Scan(&t.ID, &t.Name, &t.Engine, &t.Script, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan transformation row: %w", err)
		}
		transformations = append(transformations, t)
	}
	return transformations, nil
}

// UpdateTransformation updates an existing transformation in the database.
func (s *Store) UpdateTransformation(t *Transformation) error {
	query := `UPDATE transformations SET name = ?, engine = ?, script = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`
	_, err := s.db.Exec(query, t.Name, t.Engine, t.Script, t.ID)
	if err != nil {
		return fmt.Errorf("failed to update transformation: %w", err)
	}
	return nil
}

// DeleteTransformation deletes a transformation by its ID.
func (s *Store) DeleteTransformation(id string) error {
	query := `DELETE FROM transformations WHERE id = ?`
	_, err := s.db.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to delete transformation: %w", err)
	}
	return nil
}
