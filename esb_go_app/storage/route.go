package storage

import (
	"database/sql"
	"fmt"
	"strings"
)

// CreateRoute creates a new route in the database.
func (s *Store) CreateRoute(route *Route) error {
	query := `INSERT INTO routes (id, name, source_channel_id, destination_channel_id, route_type, transformation_id, integration_id) VALUES (?, ?, ?, ?, ?, ?, ?)`
	_, err := s.db.Exec(query, route.ID, route.Name, route.SourceChannelID, route.DestinationChannelID, route.RouteType, route.TransformationID, route.IntegrationID)
	if err != nil {
		return fmt.Errorf("failed to create route: %w", err)
	}
	return nil
}

// UpdateRoute updates an existing route in the database.
func (s *Store) UpdateRoute(route *Route) error {
	query := `UPDATE routes SET name = ?, source_channel_id = ?, destination_channel_id = ?, route_type = ?, transformation_id = ?, integration_id = ? WHERE id = ?`
	_, err := s.db.Exec(query, route.Name, route.SourceChannelID, route.DestinationChannelID, route.RouteType, route.TransformationID, route.IntegrationID, route.ID)
	if err != nil {
		return fmt.Errorf("failed to update route: %w", err)
	}
	return nil
}

// DeleteRoute deletes a route by its ID.
func (s *Store) DeleteRoute(id string) error {
	query := "DELETE FROM routes WHERE id = ?"
	_, err := s.db.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to delete route: %w", err)
	}
	return nil
}

// buildRouteInfo manually builds the extended RouteInfo struct from a raw Route.
func (s *Store) BuildRouteInfo(route Route) (RouteInfo, error) {
	info := RouteInfo{
		ID:              route.ID,
		Name:            route.Name,
		SourceChannelID: route.SourceChannelID,
		RouteType:       route.RouteType,
		CreatedAt:       route.CreatedAt,
	}

	if route.DestinationChannelID != nil {
		info.DestinationChannelID = *route.DestinationChannelID
	}
	if route.TransformationID != nil {
		info.TransformationID = *route.TransformationID
	}
	if route.IntegrationID != nil {
		info.IntegrationID = *route.IntegrationID
	}

	// 1. Populate Source Info
	if strings.HasPrefix(route.SourceChannelID, "collector-output:") {
		collectorID := strings.TrimPrefix(route.SourceChannelID, "collector-output:")
		collector, err := s.GetCollectorByID(collectorID)
		if err == nil && collector != nil {
			info.SourceAppName = "Сборщик"
			info.SourceChannelName = collector.Name
			info.SourceBaseName = route.SourceChannelID
		}
	} else {
		sourceChannel, err := s.GetChannelByID(route.SourceChannelID)
		if err == nil && sourceChannel != nil {
			info.SourceBaseName = sourceChannel.Destination
			info.SourceDestination = sourceChannel.Destination
			info.SourceChannelName = sourceChannel.Name
			app, err := s.GetApplicationByID(sourceChannel.ApplicationID)
			if err == nil && app != nil {
				info.SourceAppName = app.Name
			}
		}
	}

	// 2. Populate Destination Info
	if route.DestinationChannelID != nil {
		destChannel, err := s.GetChannelByID(*route.DestinationChannelID)
		if err == nil && destChannel != nil {
			info.DestinationDestination = destChannel.Destination
			info.DestinationChannelName = destChannel.Name
			app, err := s.GetApplicationByID(destChannel.ApplicationID)
			if err == nil && app != nil {
				info.DestinationAppName = app.Name
			}
		}
	}

	// 3. Populate Transformation Info
	if route.TransformationID != nil {
		transform, err := s.GetTransformationByID(*route.TransformationID)
		if err == nil && transform != nil {
			info.TransformationName = transform.Name
		}
	}

	// 4. Populate Integration Info
	if route.IntegrationID != nil {
		integration, err := s.GetIntegrationByID(*route.IntegrationID)
		if err == nil && integration != nil {
			info.IntegrationName = integration.Name
		}
	}

	return info, nil
}

// processRoutesRows iterates over rows and builds a slice of RouteInfo.
func (s *Store) processRoutesRows(rows *sql.Rows) ([]RouteInfo, error) {
	var results []RouteInfo
	for rows.Next() {
		var route Route
		// Scan all fields from the routes table
		if err := rows.Scan(&route.ID, &route.Name, &route.CreatedAt, &route.RouteType, &route.TransformationID, &route.IntegrationID, &route.SourceChannelID, &route.DestinationChannelID); err != nil {
			return nil, fmt.Errorf("failed to scan raw route: %w", err)
		}
		info, err := s.BuildRouteInfo(route)
		if err != nil {
			s.logger.Warn("could not build full route info, skipping", "route_id", route.ID, "error", err)
			continue
		}
		results = append(results, info)
	}
	return results, nil
}

// GetAllRoutes retrieves all routes and enriches them with related info.
func (s *Store) GetAllRoutes() ([]RouteInfo, error) {
	query := `SELECT id, name, created_at, route_type, transformation_id, integration_id, source_channel_id, destination_channel_id FROM routes ORDER BY created_at DESC`
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to get all routes: %w", err)
	}
	defer rows.Close()

	return s.processRoutesRows(rows)
}

// GetRoutesByIntegrationID retrieves all routes for a given integration ID.
func (s *Store) GetRoutesByIntegrationID(integrationID string) ([]RouteInfo, error) {
	query := `SELECT id, name, created_at, route_type, transformation_id, integration_id, source_channel_id, destination_channel_id FROM routes WHERE integration_id = ? ORDER BY created_at DESC`
	rows, err := s.db.Query(query, integrationID)
	if err != nil {
		return nil, fmt.Errorf("failed to get routes by integration id: %w", err)
	}
	defer rows.Close()

	return s.processRoutesRows(rows)
}

// GetRouteByID retrieves a single route by its ID.
func (s *Store) GetRouteByID(id string) (*Route, error) {
	query := `SELECT id, name, source_channel_id, destination_channel_id, route_type, transformation_id, integration_id, created_at FROM routes WHERE id = ?`
	row := s.db.QueryRow(query, id)

	r := &Route{}
	err := row.Scan(&r.ID, &r.Name, &r.SourceChannelID, &r.DestinationChannelID, &r.RouteType, &r.TransformationID, &r.IntegrationID, &r.CreatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil // Not found is not an error
		}
		return nil, fmt.Errorf("failed to get route by id: %w", err)
	}
	return r, nil
}
