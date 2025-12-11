package storage

import (
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

// Store
type Store struct {
	db     *sql.DB
	logger *slog.Logger
}

// NewStore
func NewStore(dbPath string, logger *slog.Logger) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, fmt.Errorf("failed to create storage directory: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath+"?_foreign_keys=on")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	store := &Store{
		db:     db,
		logger: logger,
	}

	if err := store.migrate(); err != nil {
		return nil, fmt.Errorf("failed to migrate database: %w", err)
	}

	// Close and re-open the database connection to ensure schema cache is refreshed
	if store.db != nil {
		store.db.Close()
	}
	db, err = sql.Open("sqlite", dbPath+"?_foreign_keys=on")
	if err != nil {
		return nil, fmt.Errorf("failed to re-open database after migration: %w", err)
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to re-connect to database after migration: %w", err)
	}
	store.db = db

	logger.Info("database initialized and migrated successfully", "path", dbPath)
	return store, nil
}

// migrate handles database schema setup and evolution.
func (s *Store) migrate() error {
	s.logger.Info("checking database schema...")

	// Create tables if they don't exist
	if err := s.createTablesIfNotExist(); err != nil {
		return err
	}

	// Perform alterations on existing tables
	if err := s.migrateCollectorsTable(); err != nil {
		return fmt.Errorf("failed to migrate collectors table: %w", err)
	}
	if err := s.migrateRoutesTable(); err != nil {
		return fmt.Errorf("failed to migrate routes table: %w", err)
	}
	if err := s.migrateChannelsTable(); err != nil {
		return fmt.Errorf("failed to migrate channels table: %w", err)
	}

	s.logger.Info("database schema is up to date.")
	return nil
}

// createTablesIfNotExist ensures all necessary tables are created.
func (s *Store) createTablesIfNotExist() error {
	// The order is important due to foreign key constraints
	tables := []string{
		`CREATE TABLE IF NOT EXISTS applications (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,
			client_secret TEXT NOT NULL,
			id_token TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS channels (
			id TEXT PRIMARY KEY,
			application_id TEXT NOT NULL,
			name TEXT NOT NULL,
			direction TEXT NOT NULL,
			destination TEXT NOT NULL,
			fanout_mode BOOLEAN NOT NULL DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (application_id) REFERENCES applications(id) ON DELETE CASCADE,
			UNIQUE(application_id, name)
		);`,
		`CREATE TABLE IF NOT EXISTS transformations (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,
			engine TEXT NOT NULL,
			script TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS integrations (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,
			description TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS collectors (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,
			schedule TEXT NOT NULL,
			engine TEXT NOT NULL,
			script TEXT NOT NULL,
			integration_id TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (integration_id) REFERENCES integrations(id) ON DELETE SET NULL
		);`,
		`CREATE TABLE IF NOT EXISTS routes (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			source_channel_id TEXT NOT NULL,
			destination_channel_id TEXT,
			route_type TEXT NOT NULL DEFAULT 'direct',
			transformation_id TEXT,
			integration_id TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (source_channel_id) REFERENCES channels(id) ON DELETE CASCADE,
			FOREIGN KEY (destination_channel_id) REFERENCES channels(id) ON DELETE SET NULL,
			FOREIGN KEY (transformation_id) REFERENCES transformations(id) ON DELETE SET NULL,
			FOREIGN KEY (integration_id) REFERENCES integrations(id) ON DELETE SET NULL
		);`,
	}

	for _, tableSQL := range tables {
		if _, err := s.db.Exec(tableSQL); err != nil {
			// Extract table name for better error message
			parts := strings.Fields(tableSQL)
			tableName := "unknown"
			if len(parts) > 5 {
				tableName = parts[5]
			}
			return fmt.Errorf("failed to create table %s: %w", tableName, err)
		}
	}
	return nil
}

// migrateChannelsTable handles adding the fanout_mode column to the `channels` table.
func (s *Store) migrateChannelsTable() error {
	rows, err := s.db.Query(`PRAGMA table_info(channels);`)
	if err != nil {
		return nil // Table might not exist on a fresh DB, which is fine.
	}
	defer rows.Close()

	var hasFanoutMode bool
	for rows.Next() {
		var cid, notnull, pk int
		var name, rtype string
		var dfltValue interface{}
		if err := rows.Scan(&cid, &name, &rtype, &notnull, &dfltValue, &pk); err != nil {
			return fmt.Errorf("failed to scan table_info for channels: %w", err)
		}
		if name == "fanout_mode" {
			hasFanoutMode = true
			break
		}
	}

	if !hasFanoutMode {
		s.logger.Info("migrating 'channels' table: adding fanout_mode column...")
		if _, err := s.db.Exec(`ALTER TABLE channels ADD COLUMN fanout_mode BOOLEAN NOT NULL DEFAULT 0`); err != nil {
			return fmt.Errorf("failed to add fanout_mode to channels table: %w", err)
		}
		s.logger.Info("'channels' table migrated successfully (fanout_mode).")
	}

	return nil
}


// migrateCollectorsTable handles the migration for the 'collectors' table.
// It transitions from the old schema with `destination_channel_id` to the new one without it.
func (s *Store) migrateCollectorsTable() error {
	rows, err := s.db.Query(`PRAGMA table_info(collectors);`)
	if err != nil {
		// This can happen on a fresh DB, which is fine.
		return nil
	}
	defer rows.Close()

	hasDestinationID := false
	for rows.Next() {
		var cid, notnull, pk int
		var name, rtype string
		var dfltValue interface{}
		if err := rows.Scan(&cid, &name, &rtype, &notnull, &dfltValue, &pk); err != nil {
			return fmt.Errorf("failed to scan table_info for collectors: %w", err)
		}
		if name == "destination_channel_id" {
			hasDestinationID = true
			break
		}
	}

	// If the old column exists, we need to migrate the table.
	if hasDestinationID {
		s.logger.Info("migrating 'collectors' table: removing destination_channel_id...")
		tx, err := s.db.Begin()
		if err != nil { return err }

		if _, err := tx.Exec(`ALTER TABLE collectors RENAME TO old_collectors;`); err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to rename collectors to old_collectors: %w", err)
		}

		// Create new table with final schema
		createCollectorsTable := `
			CREATE TABLE collectors (
				id TEXT PRIMARY KEY,
				name TEXT NOT NULL UNIQUE,
				schedule TEXT NOT NULL,
				engine TEXT NOT NULL,
				script TEXT NOT NULL,
				integration_id TEXT,
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				FOREIGN KEY (integration_id) REFERENCES integrations(id) ON DELETE SET NULL
			);`
		if _, err := tx.Exec(createCollectorsTable); err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to create new collectors table during migration: %w", err)
		}

		// Copy data, omitting the old destination_channel_id
		copySQL := `INSERT INTO collectors (id, name, schedule, engine, script, created_at, updated_at)
					SELECT id, name, schedule, engine, script, created_at, updated_at FROM old_collectors;`
		if _, err := tx.Exec(copySQL); err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to copy data to new collectors table: %w", err)
		}

		if _, err := tx.Exec(`DROP TABLE old_collectors;`); err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to drop old_collectors table: %w", err)
		}
		s.logger.Info("'collectors' table migrated successfully.")
		return tx.Commit()
	}

	return nil // No migration needed
}

// migrateRoutesTable handles adding new columns to the `routes` table if they are missing.
func (s *Store) migrateRoutesTable() error {
	rows, err := s.db.Query(`PRAGMA table_info(routes);`)
	if err != nil {
		// Table might not exist on a fresh DB, which is fine.
		return nil
	}
	defer rows.Close()

	var hasName, hasSourceChannelID, hasDestinationChannelID, hasRouteType, hasTransformationID, hasIntegrationID, hasCreatedAt bool
	for rows.Next() {
		var cid, notnull, pk int
		var name, rtype string
		var dfltValue interface{}
		if err := rows.Scan(&cid, &name, &rtype, &notnull, &dfltValue, &pk); err != nil {
			return fmt.Errorf("failed to scan table_info for routes: %w", err)
		}
		switch name {
		case "name":
			hasName = true
		case "source_channel_id":
			hasSourceChannelID = true
		case "destination_channel_id":
			hasDestinationChannelID = true
		case "route_type":
			hasRouteType = true
		case "transformation_id":
			hasTransformationID = true
		case "integration_id":
			hasIntegrationID = true
		case "created_at":
			hasCreatedAt = true
		}
	}

	if !hasName {
		s.logger.Info("migrating 'routes' table: adding name column...")
		if _, err := s.db.Exec(`ALTER TABLE routes ADD COLUMN name TEXT NOT NULL DEFAULT ''`); err != nil {
			return fmt.Errorf("failed to add name to routes table: %w", err)
		}
		s.logger.Info("'routes' table migrated successfully (name).")
	}

	if !hasSourceChannelID {
		s.logger.Info("migrating 'routes' table: adding source_channel_id column...")
		if _, err := s.db.Exec(`ALTER TABLE routes ADD COLUMN source_channel_id TEXT NOT NULL DEFAULT ''`); err != nil {
			return fmt.Errorf("failed to add source_channel_id to routes table: %w", err)
		}
		s.logger.Info("'routes' table migrated successfully (source_channel_id).")
	}

	if !hasDestinationChannelID {
		s.logger.Info("migrating 'routes' table: adding destination_channel_id column...")
		if _, err := s.db.Exec(`ALTER TABLE routes ADD COLUMN destination_channel_id TEXT`); err != nil {
			return fmt.Errorf("failed to add destination_channel_id to routes table: %w", err)
		}
		s.logger.Info("'routes' table migrated successfully (destination_channel_id).")
	}

	if !hasRouteType {
		s.logger.Info("migrating 'routes' table: adding route_type column...")
		if _, err := s.db.Exec(`ALTER TABLE routes ADD COLUMN route_type TEXT NOT NULL DEFAULT 'direct'`); err != nil {
			return fmt.Errorf("failed to add route_type to routes table: %w", err)
		}
		s.logger.Info("'routes' table migrated successfully (route_type).")
	}

	if !hasTransformationID {
		s.logger.Info("migrating 'routes' table: adding transformation_id column...")
		if _, err := s.db.Exec(`ALTER TABLE routes ADD COLUMN transformation_id TEXT`); err != nil {
			return fmt.Errorf("failed to add transformation_id to routes table: %w", err)
		}
		s.logger.Info("'routes' table migrated successfully (transformation_id).")
	}

	if !hasIntegrationID {
		s.logger.Info("migrating 'routes' table: adding integration_id column...")
		if _, err := s.db.Exec(`ALTER TABLE routes ADD COLUMN integration_id TEXT`); err != nil {
			return fmt.Errorf("failed to add integration_id to routes table: %w", err)
		}
		s.logger.Info("'routes' table migrated successfully (integration_id).")
	}

	if !hasCreatedAt {
		s.logger.Info("migrating 'routes' table: adding created_at column...")
		if _, err := s.db.Exec(`ALTER TABLE routes ADD COLUMN created_at DATETIME DEFAULT CURRENT_TIMESTAMP`); err != nil {
			return fmt.Errorf("failed to add created_at to routes table: %w", err)
		}
		s.logger.Info("'routes' table migrated successfully (created_at).")
	}

	return nil
}

// Close
func (s *Store) Close() error {
	return s.db.Close()
}
