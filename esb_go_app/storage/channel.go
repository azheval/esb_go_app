package storage

import (
	"database/sql"
	"fmt"
)

// CreateChannel
func (s *Store) CreateChannel(ch *Channel) error {
	query := `INSERT INTO channels (id, application_id, name, direction, destination, fanout_mode) VALUES (?, ?, ?, ?, ?, ?)`
	_, err := s.db.Exec(query, ch.ID, ch.ApplicationID, ch.Name, ch.Direction, ch.Destination, ch.FanoutMode)
	if err != nil {
		return fmt.Errorf("failed to create channel: %w", err)
	}
	return nil
}

// UpdateChannel
func (s *Store) UpdateChannel(ch *Channel) error {
	query := `UPDATE channels SET name = ?, direction = ?, destination = ?, fanout_mode = ? WHERE id = ?`
	_, err := s.db.Exec(query, ch.Name, ch.Direction, ch.Destination, ch.FanoutMode, ch.ID)
	if err != nil {
		return fmt.Errorf("failed to update channel: %w", err)
	}
	return nil
}

// GetChannelsByAppID
func (s *Store) GetChannelsByAppID(appID string) ([]Channel, error) {
	query := `SELECT id, application_id, name, direction, destination, fanout_mode, created_at FROM channels WHERE application_id = ?`
	rows, err := s.db.Query(query, appID)
	if err != nil {
		return nil, fmt.Errorf("failed to get channels by app id: %w", err)
	}
	defer rows.Close()

	var channels []Channel
	for rows.Next() {
		var ch Channel
		if err := rows.Scan(&ch.ID, &ch.ApplicationID, &ch.Name, &ch.Direction, &ch.Destination, &ch.FanoutMode, &ch.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan channel row: %w", err)
		}
		channels = append(channels, ch)
	}

	return channels, nil
}

// GetAllChannels
func (s *Store) GetAllChannels() ([]Channel, error) {
	query := `SELECT id, application_id, name, direction, destination, fanout_mode, created_at FROM channels`
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to get all channels: %w", err)
	}
	defer rows.Close()

	var channels []Channel
	for rows.Next() {
		var ch Channel
		if err := rows.Scan(&ch.ID, &ch.ApplicationID, &ch.Name, &ch.Direction, &ch.Destination, &ch.FanoutMode, &ch.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan channel row: %w", err)
		}
		channels = append(channels, ch)
	}

	return channels, nil
}

// GetChannelByID
func (s *Store) GetChannelByID(id string) (*Channel, error) {
	query := `SELECT id, application_id, name, direction, destination, fanout_mode, created_at FROM channels WHERE id = ?`
	row := s.db.QueryRow(query, id)

	ch := &Channel{}
	err := row.Scan(&ch.ID, &ch.ApplicationID, &ch.Name, &ch.Direction, &ch.Destination, &ch.FanoutMode, &ch.CreatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get channel by id: %w", err)
	}
	return ch, nil
}

// GetChannelsByName
func (s *Store) GetChannelsByName(name string) ([]Channel, error) {
	query := `SELECT id, application_id, name, direction, destination, fanout_mode, created_at FROM channels WHERE name = ?`
	rows, err := s.db.Query(query, name)
	if err != nil {
		return nil, fmt.Errorf("failed to get channels by name: %w", err)
	}
	defer rows.Close()

	var channels []Channel
	for rows.Next() {
		var ch Channel
		if err := rows.Scan(&ch.ID, &ch.ApplicationID, &ch.Name, &ch.Direction, &ch.Destination, &ch.FanoutMode, &ch.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan channel row: %w", err)
		}
		channels = append(channels, ch)
	}
	return channels, nil
}

// FindChannel
func (s *Store) FindChannel(identifier string) (*Channel, error) {
	// First, try to find by ID
	channel, err := s.GetChannelByID(identifier)
	if err != nil {
		// Ignore ErrNoRows, but return on other DB errors
		if err != sql.ErrNoRows {
			return nil, fmt.Errorf("error checking channel by ID: %w", err)
		}
	}
	if channel != nil {
		return channel, nil // Found by ID
	}

	// If not found by ID, try by name
	channels, err := s.GetChannelsByName(identifier)
	if err != nil {
		return nil, fmt.Errorf("error checking channel by name: %w", err)
	}

	if len(channels) == 0 {
		return nil, nil // Not found
	}

	if len(channels) > 1 {
		return nil, fmt.Errorf("ambiguous channel name: '%s' matches multiple channels", identifier)
	}

	return &channels[0], nil // Found unique match by name
}

// DeleteChannel
func (s *Store) DeleteChannel(id string) error {
	query := `DELETE FROM channels WHERE id = ?`
	_, err := s.db.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to delete channel: %w", err)
	}
	return nil
}

// DeleteOrphanedChannels
func (s *Store) DeleteOrphanedChannels() (int64, error) {
	query := `DELETE FROM channels WHERE application_id NOT IN (SELECT id FROM applications)`
	res, err := s.db.Exec(query)
	if err != nil {
		return 0, fmt.Errorf("failed to delete orphaned channels: %w", err)
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get affected rows: %w", err)
	}

	return rowsAffected, nil
}

// GetAllRoutableChannels
func (s *Store) GetAllRoutableChannels(direction string) ([]ChannelInfo, error) {
	query := `
		SELECT c.id, c.name, c.destination, c.fanout_mode, a.name
		FROM channels c
		JOIN applications a ON c.application_id = a.id
		WHERE c.direction = ?
		ORDER BY a.name, c.name
	`
	rows, err := s.db.Query(query, direction)
	if err != nil {
		return nil, fmt.Errorf("failed to get routable channels: %w", err)
	}
	defer rows.Close()

	var channels []ChannelInfo
	for rows.Next() {
		var ch ChannelInfo
		if err := rows.Scan(&ch.ID, &ch.Name, &ch.Destination, &ch.FanoutMode, &ch.ApplicationName); err != nil {
			return nil, fmt.Errorf("failed to scan routable channel row: %w", err)
		}
		channels = append(channels, ch)
	}
	return channels, nil
}
