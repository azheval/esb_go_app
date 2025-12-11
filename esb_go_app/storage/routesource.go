package storage

import (
	"fmt"
	"sort"
)

// RouteSource represents a generic source for a route, which can be an outbound channel or a collector.
type RouteSource struct {
	ID   string // For a channel, this is the channel ID. For a collector, it's 'collector-output:<collector_id>'
	Name string // A user-friendly name for display in the UI
}

// GetAllRouteSources fetches all possible sources for routes (all channels and all collectors)
// and returns them as a unified list.
func (s *Store) GetAllRouteSources() ([]RouteSource, error) {
	var sources []RouteSource

	// 1. Get all outbound channels
	outboundChannels, err := s.GetAllRoutableChannels("outbound")
	if err != nil {
		return nil, fmt.Errorf("failed to get outbound channels for route sources: %w", err)
	}
	for _, ch := range outboundChannels {
		sources = append(sources, RouteSource{
			ID:   ch.ID,
			Name: fmt.Sprintf("Источник (внешний): %s / %s", ch.ApplicationName, ch.Name),
		})
	}

	// 2. Get all inbound channels (so routes can be chained)
	inboundChannels, err := s.GetAllRoutableChannels("inbound")
	if err != nil {
		return nil, fmt.Errorf("failed to get inbound channels for route sources: %w", err)
	}
	for _, ch := range inboundChannels {
		sources = append(sources, RouteSource{
			ID:   ch.ID,
			Name: fmt.Sprintf("Источник (внутренний): %s / %s", ch.ApplicationName, ch.Name),
		})
	}

	// 3. Get all collectors
	collectors, err := s.GetAllCollectors()
	if err != nil {
		return nil, fmt.Errorf("failed to get collectors for route sources: %w", err)
	}
	for _, c := range collectors {
		sources = append(sources, RouteSource{
			ID:   fmt.Sprintf("collector-output:%s", c.ID),
			Name: fmt.Sprintf("Сборщик: %s", c.Name),
		})
	}

	// 4. Sort for consistent ordering in the UI
	sort.Slice(sources, func(i, j int) bool {
		return sources[i].Name < sources[j].Name
	})

	return sources, nil
}
