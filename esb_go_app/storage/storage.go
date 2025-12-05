package storage

import (
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// Store - абстракция хранилища данных, использующая SQLite.
type Store struct {
	db     *sql.DB
	logger *slog.Logger
}

// Application представляет приложение (клиента) в системе.
type Application struct {
	ID           string
	Name         string
	ClientSecret string
	IDToken      string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// Channel представляет канал, связанный с приложением.
type Channel struct {
	ID            string
	ApplicationID string
	Name          string
	Direction     string // "inbound" или "outbound"
	Destination   string
	CreatedAt     time.Time
}

// Route представляет маршрут между двумя каналами.
type Route struct {
	ID                   string
	SourceChannelID      string
	DestinationChannelID string
	CreatedAt            time.Time
}

// ChannelInfo - отображение информации о канале в UI.
type ChannelInfo struct {
	Channel
	ApplicationName string
}

// RouteInfo - отображение информации о маршруте в UI.
type RouteInfo struct {
	Route
	SourceChannelName      string
	SourceAppName          string
	SourceDirection        string
	SourceDestination      string
	DestinationChannelName string
	DestinationAppName     string
	DestinationDirection   string
	DestinationDestination string
}

// NewStore создает и возвращает новый экземпляр Store.
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

	logger.Info("database initialized and migrated successfully", "path", dbPath)
	return store, nil
}

// migrate создает необходимые таблицы.
func (s *Store) migrate() error {
	createApplicationsTable := `
	CREATE TABLE IF NOT EXISTS applications (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL UNIQUE,
		client_secret TEXT NOT NULL,
		id_token TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);`
	if _, err := s.db.Exec(createApplicationsTable); err != nil {
		return fmt.Errorf("failed to create applications table: %w", err)
	}

	createChannelsTable := `
	CREATE TABLE IF NOT EXISTS channels (
		id TEXT PRIMARY KEY,
		application_id TEXT NOT NULL,
		name TEXT NOT NULL,
		direction TEXT NOT NULL,
		destination TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (application_id) REFERENCES applications(id) ON DELETE CASCADE,
		UNIQUE(application_id, name)
	);`
	if _, err := s.db.Exec(createChannelsTable); err != nil {
		return fmt.Errorf("failed to create channels table: %w", err)
	}

	createRoutesTable := `
	CREATE TABLE IF NOT EXISTS routes (
		id TEXT PRIMARY KEY,
		source_channel_id TEXT NOT NULL,
		destination_channel_id TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (source_channel_id) REFERENCES channels(id) ON DELETE CASCADE,
		FOREIGN KEY (destination_channel_id) REFERENCES channels(id) ON DELETE CASCADE,
		UNIQUE(source_channel_id, destination_channel_id)
	);`
	if _, err := s.db.Exec(createRoutesTable); err != nil {
		return fmt.Errorf("failed to create routes table: %w", err)
	}

	s.logger.Info("database migration completed")
	return nil
}

// CreateApplication создает новое приложение.
func (s *Store) CreateApplication(app *Application) error {
	query := `INSERT INTO applications (id, name, client_secret, id_token) VALUES (?, ?, ?, ?)`
	_, err := s.db.Exec(query, app.ID, app.Name, app.ClientSecret, app.IDToken)
	if err != nil {
		return fmt.Errorf("failed to create application: %w", err)
	}
	return nil
}

// GetApplicationByName возвращает приложение по его имени.
func (s *Store) GetApplicationByName(name string) (*Application, error) {
	query := `SELECT id, name, client_secret, id_token, created_at, updated_at FROM applications WHERE name = ?`
	row := s.db.QueryRow(query, name)

	app := &Application{}
	err := row.Scan(&app.ID, &app.Name, &app.ClientSecret, &app.IDToken, &app.CreatedAt, &app.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil // Приложение не найдено
		}
		return nil, fmt.Errorf("failed to get application by name: %w", err)
	}
	return app, nil
}

// GetApplicationByID возвращает приложение по его ID.
func (s *Store) GetApplicationByID(id string) (*Application, error) {
	query := `SELECT id, name, client_secret, id_token, created_at, updated_at FROM applications WHERE id = ?`
	row := s.db.QueryRow(query, id)

	app := &Application{}
	err := row.Scan(&app.ID, &app.Name, &app.ClientSecret, &app.IDToken, &app.CreatedAt, &app.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil // Приложение не найдено
		}
		return nil, fmt.Errorf("failed to get application by id: %w", err)
	}
	return app, nil
}

// GetApplicationByIDToken возвращает приложение по его токену.
func (s *Store) GetApplicationByIDToken(token string) (*Application, error) {
	query := `SELECT id, name, client_secret, id_token, created_at, updated_at FROM applications WHERE id_token = ?`
	row := s.db.QueryRow(query, token)

	app := &Application{}
	err := row.Scan(&app.ID, &app.Name, &app.ClientSecret, &app.IDToken, &app.CreatedAt, &app.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil // Приложение не найдено
		}
		return nil, fmt.Errorf("failed to get application by token: %w", err)
	}
	return app, nil
}

// GetAllApplications возвращает все приложения.
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

// DeleteApplication удаляет приложение по ID, а также все связанные каналы благодаря каскадному удалению.
func (s *Store) DeleteApplication(id string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	// 1. Удаляем связанные каналы
	if _, err := tx.Exec("DELETE FROM channels WHERE application_id = ?", id); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("failed to delete associated channels: %w", err)
	}

	// 2. Удаляем само приложение
	if _, err := tx.Exec("DELETE FROM applications WHERE id = ?", id); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("failed to delete application: %w", err)
	}

	return tx.Commit()
}

// CreateChannel создает новый канал для приложения.
func (s *Store) CreateChannel(ch *Channel) error {
	query := `INSERT INTO channels (id, application_id, name, direction, destination) VALUES (?, ?, ?, ?, ?)`
	_, err := s.db.Exec(query, ch.ID, ch.ApplicationID, ch.Name, ch.Direction, ch.Destination)
	if err != nil {
		return fmt.Errorf("failed to create channel: %w", err)
	}
	return nil
}

// GetChannelsByAppID возвращает все каналы для заданного ID приложения.
func (s *Store) GetChannelsByAppID(appID string) ([]Channel, error) {
	query := `SELECT id, application_id, name, direction, destination, created_at FROM channels WHERE application_id = ?`
	rows, err := s.db.Query(query, appID)
	if err != nil {
		return nil, fmt.Errorf("failed to get channels by app id: %w", err)
	}
	defer rows.Close()

	var channels []Channel
	for rows.Next() {
		var ch Channel
		if err := rows.Scan(&ch.ID, &ch.ApplicationID, &ch.Name, &ch.Direction, &ch.Destination, &ch.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan channel row: %w", err)
		}
		channels = append(channels, ch)
	}

	return channels, nil
}

// GetChannelByID возвращает канал по его ID.
func (s *Store) GetChannelByID(id string) (*Channel, error) {
	query := `SELECT id, application_id, name, direction, destination, created_at FROM channels WHERE id = ?`
	row := s.db.QueryRow(query, id)

	ch := &Channel{}
	err := row.Scan(&ch.ID, &ch.ApplicationID, &ch.Name, &ch.Direction, &ch.Destination, &ch.CreatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil // Канал не найден
		}
		return nil, fmt.Errorf("failed to get channel by id: %w", err)
	}
	return ch, nil
}

// DeleteChannel удаляет канал по ID.
func (s *Store) DeleteChannel(id string) error {
	query := `DELETE FROM channels WHERE id = ?`
	_, err := s.db.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to delete channel: %w", err)
	}
	return nil
}

// DeleteOrphanedChannels удаляет каналы, ссылающиеся на несуществующие приложения.
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

// --- Методы для Маршрутов (Routes) ---

// CreateRoute создает новый маршрут.
func (s *Store) CreateRoute(route *Route) error {
	query := `INSERT INTO routes (id, source_channel_id, destination_channel_id) VALUES (?, ?, ?)`
	_, err := s.db.Exec(query, route.ID, route.SourceChannelID, route.DestinationChannelID)
	if err != nil {
		return fmt.Errorf("failed to create route: %w", err)
	}
	return nil
}

// DeleteRoute удаляет маршрут по ID.
func (s *Store) DeleteRoute(id string) error {
	query := "DELETE FROM routes WHERE id = ?"
	_, err := s.db.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to delete route: %w", err)
	}
	return nil
}

// GetAllRoutes возвращает все маршруты с детальной информацией.
func (s *Store) GetAllRoutes() ([]RouteInfo, error) {
	query := `
		SELECT
			r.id,
			r.created_at,
			sc.id, sc.name, sc.direction, sc.destination, sa.name,
			dc.id, dc.name, dc.direction, dc.destination, da.name
		FROM routes r
		JOIN channels sc ON r.source_channel_id = sc.id
		JOIN applications sa ON sc.application_id = sa.id
		JOIN channels dc ON r.destination_channel_id = dc.id
		JOIN applications da ON dc.application_id = da.id
		ORDER BY r.created_at DESC
	`
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to get all routes: %w", err)
	}
	defer rows.Close()

	var routes []RouteInfo
	for rows.Next() {
		var r RouteInfo
		if err := rows.Scan(
			&r.ID, &r.CreatedAt,
			&r.SourceChannelID, &r.SourceChannelName, &r.SourceDirection, &r.SourceDestination, &r.SourceAppName,
			&r.DestinationChannelID, &r.DestinationChannelName, &r.DestinationDirection, &r.DestinationDestination, &r.DestinationAppName,
		); err != nil {
			return nil, fmt.Errorf("failed to scan route row: %w", err)
		}
		routes = append(routes, r)
	}
	return routes, nil
}

// GetAllRoutableChannels возвращает все каналы, которые могут быть использованы в маршрутизации.
func (s *Store) GetAllRoutableChannels(direction string) ([]ChannelInfo, error) {
	query := `
		SELECT c.id, c.name, c.destination, a.name
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
		if err := rows.Scan(&ch.ID, &ch.Name, &ch.Destination, &ch.ApplicationName); err != nil {
			return nil, fmt.Errorf("failed to scan routable channel row: %w", err)
		}
		channels = append(channels, ch)
	}
	return channels, nil
}

// Close закрывает соединение с базой данных.
func (s *Store) Close() error {
	return s.db.Close()
}
