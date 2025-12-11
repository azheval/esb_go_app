package storage

import (
	"database/sql"
	"fmt"
)

// CreateCollector creates a new collector in the database.
func (s *Store) CreateCollector(c *Collector) error {
	query := `INSERT INTO collectors (id, name, schedule, engine, script, integration_id) VALUES (?, ?, ?, ?, ?, ?)`
	_, err := s.db.Exec(query, c.ID, c.Name, c.Schedule, c.Engine, c.Script, c.IntegrationID)
	if err != nil {
		return fmt.Errorf("failed to create collector: %w", err)
	}
	return nil
}

// GetCollectorByID retrieves a collector by its ID.
func (s *Store) GetCollectorByID(id string) (*Collector, error) {
	query := `SELECT id, name, schedule, engine, script, integration_id, created_at, updated_at FROM collectors WHERE id = ?`
	row := s.db.QueryRow(query, id)

	c := &Collector{}
	err := row.Scan(&c.ID, &c.Name, &c.Schedule, &c.Engine, &c.Script, &c.IntegrationID, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get collector by ID: %w", err)
	}
	return c, nil
}

// GetCollectorsByIntegrationID retrieves all collectors for a given integration ID.
func (s *Store) GetCollectorsByIntegrationID(integrationID string) ([]Collector, error) {
	query := `SELECT id, name, schedule, engine, script, integration_id, created_at, updated_at FROM collectors WHERE integration_id = ? ORDER BY created_at DESC`
	rows, err := s.db.Query(query, integrationID)
	if err != nil {
		return nil, fmt.Errorf("failed to get collectors by integration id: %w", err)
	}
	defer rows.Close()

	var collectors []Collector
	for rows.Next() {
		var c Collector
		if err := rows.Scan(&c.ID, &c.Name, &c.Schedule, &c.Engine, &c.Script, &c.IntegrationID, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan collector row: %w", err)
		}
		collectors = append(collectors, c)
	}
	return collectors, nil
}


// GetCollectorByName retrieves a collector by its name.
func (s *Store) GetCollectorByName(name string) (*Collector, error) {
	query := `SELECT id, name, schedule, engine, script, integration_id, created_at, updated_at FROM collectors WHERE name = ?`
	row := s.db.QueryRow(query, name)

	c := &Collector{}
	err := row.Scan(&c.ID, &c.Name, &c.Schedule, &c.Engine, &c.Script, &c.IntegrationID, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get collector by name: %w", err)
	}
	return c, nil
}

// GetAllCollectors retrieves all collectors from the database.
func (s *Store) GetAllCollectors() ([]Collector, error) {
	query := `SELECT id, name, schedule, engine, script, integration_id, created_at, updated_at FROM collectors ORDER BY created_at DESC`
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to get all collectors: %w", err)
	}
	defer rows.Close()

	var collectors []Collector
	for rows.Next() {
		var c Collector
		if err := rows.Scan(&c.ID, &c.Name, &c.Schedule, &c.Engine, &c.Script, &c.IntegrationID, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan collector row: %w", err)
		}
		collectors = append(collectors, c)
	}
	return collectors, nil
}

// UpdateCollector updates an existing collector in the database.
func (s *Store) UpdateCollector(c *Collector) error {
	query := `UPDATE collectors SET name = ?, schedule = ?, engine = ?, script = ?, integration_id = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`
	_, err := s.db.Exec(query, c.Name, c.Schedule, c.Engine, c.Script, c.IntegrationID, c.ID)
	if err != nil {
		return fmt.Errorf("failed to update collector: %w", err)
	}
	return nil
}

// DeleteCollector deletes a collector by its ID.
func (s *Store) DeleteCollector(id string) error {
	query := `DELETE FROM collectors WHERE id = ?`
	_, err := s.db.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to delete collector: %w", err)
	}
	return nil
}
