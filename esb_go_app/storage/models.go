package storage

import "time"

// Application
type Application struct {
	ID           string
	Name         string
	ClientSecret string
	IDToken      string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// Channel
type Channel struct {
	ID            string
	ApplicationID string
	Name          string
	Direction     string // "inbound" или "outbound"
	Destination   string
	FanoutMode    bool // If true, allows multiple consumers (pub/sub). If false, one queue (competing consumers).
	CreatedAt     time.Time
}

// Route represents a message routing rule.
type Route struct {
	ID                   string
	Name                 string
	SourceChannelID      string
	DestinationChannelID *string // Nullable for transform routes
	RouteType            string  // "direct" or "transform"
	TransformationID     *string // Nullable, only for "transform" routes
	IntegrationID        *string // Nullable
	CreatedAt            time.Time
}

// Integration represents a logical grouping of ESB components.
type Integration struct {
	ID          string
	Name        string
	Description string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// ChannelInfo
type ChannelInfo struct {
	Channel
	ApplicationName string
}

// RouteInfo
type RouteInfo struct {
	ID                   string
	Name                 string
	SourceChannelID      string
	DestinationChannelID string
	RouteType            string
	TransformationID     string
	IntegrationID        string
	CreatedAt            time.Time

	SourceBaseName         string // The name used for the RabbitMQ source (queue or exchange)
	SourceChannelName      string
	SourceAppName          string
	SourceDirection        string
	SourceDestination      string
	DestinationChannelName string
	DestinationAppName     string
	DestinationDirection   string
	DestinationDestination string
	TransformationName     string // New field for UI display
	IntegrationName        string
}

// Transformation represents a script for message transformation.
type Transformation struct {
	ID        string
	Name      string
	Engine    string // "javascript" or "starlark"
	Script    string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Collector represents a scheduled job to fetch external data.
type Collector struct {
	ID            string
	Name          string
	Schedule      string // Cron string
	Engine        string // "javascript" or "starlark"
	Script        string
	IntegrationID *string // Nullable
	CreatedAt     time.Time
	UpdatedAt     time.Time
}
