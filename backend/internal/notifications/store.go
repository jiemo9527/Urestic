package notifications

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/urestic/urestic/backend/internal/ids"
)

var (
	ErrDuplicateName = errors.New("notification channel name already exists")
	ErrNotFound      = errors.New("notification channel not found")
)

type Channel struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Type         string            `json:"type"`
	Enabled      bool              `json:"-"`
	Settings     map[string]string `json:"settings"`
	SecretFields []string          `json:"secretFields"`
	CreatedAt    time.Time         `json:"createdAt"`
	UpdatedAt    time.Time         `json:"updatedAt"`
}

type Params struct {
	Name         string
	Type         string
	Settings     map[string]string
	SecretFields []string
}

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

func (s *Store) List(ctx context.Context) ([]Channel, error) {
	items, err := s.list(ctx)
	if err != nil {
		return nil, err
	}
	for index := range items {
		items[index] = items[index].Public()
	}
	return items, nil
}

func (s *Store) ListPrivate(ctx context.Context) ([]Channel, error) {
	return s.list(ctx)
}

func (s *Store) list(ctx context.Context) ([]Channel, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, type, enabled, settings_json, secret_fields_json, created_at, updated_at FROM notification_channels ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []Channel{}
	for rows.Next() {
		item, err := scanChannel(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) GetPrivate(ctx context.Context, id string) (Channel, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, name, type, enabled, settings_json, secret_fields_json, created_at, updated_at FROM notification_channels WHERE id = ?`, id)
	item, err := scanChannel(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Channel{}, ErrNotFound
	}
	return item, err
}

func (s *Store) Create(ctx context.Context, params Params) (Channel, error) {
	exists, err := s.nameExists(ctx, params.Name, "")
	if err != nil {
		return Channel{}, err
	}
	if exists {
		return Channel{}, ErrDuplicateName
	}

	id, err := ids.New()
	if err != nil {
		return Channel{}, err
	}
	settingsJSON, err := json.Marshal(params.Settings)
	if err != nil {
		return Channel{}, err
	}
	secretFieldsJSON, err := json.Marshal(params.SecretFields)
	if err != nil {
		return Channel{}, err
	}
	now := time.Now().UTC()
	_, err = s.db.ExecContext(ctx, `INSERT INTO notification_channels (id, name, type, enabled, settings_json, secret_fields_json, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		id, params.Name, params.Type, 1, string(settingsJSON), string(secretFieldsJSON), now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))
	if err != nil {
		return Channel{}, err
	}

	item, err := s.GetPrivate(ctx, id)
	if err != nil {
		return Channel{}, err
	}
	return item.Public(), nil
}

func (s *Store) Update(ctx context.Context, id string, params Params) (Channel, error) {
	if _, err := s.GetPrivate(ctx, id); err != nil {
		return Channel{}, err
	}
	exists, err := s.nameExists(ctx, params.Name, id)
	if err != nil {
		return Channel{}, err
	}
	if exists {
		return Channel{}, ErrDuplicateName
	}
	settingsJSON, err := json.Marshal(params.Settings)
	if err != nil {
		return Channel{}, err
	}
	secretFieldsJSON, err := json.Marshal(params.SecretFields)
	if err != nil {
		return Channel{}, err
	}
	result, err := s.db.ExecContext(ctx, `UPDATE notification_channels SET name = ?, type = ?, settings_json = ?, secret_fields_json = ?, updated_at = ? WHERE id = ?`,
		params.Name, params.Type, string(settingsJSON), string(secretFieldsJSON), time.Now().UTC().Format(time.RFC3339Nano), id)
	if err != nil {
		return Channel{}, err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return Channel{}, err
	}
	if rowsAffected == 0 {
		return Channel{}, ErrNotFound
	}
	item, err := s.GetPrivate(ctx, id)
	if err != nil {
		return Channel{}, err
	}
	return item.Public(), nil
}

func (s *Store) Delete(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM notification_channels WHERE id = ?`, id)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (c Channel) Public() Channel {
	public := c
	for _, field := range public.SecretFields {
		if _, ok := public.Settings[field]; ok {
			public.Settings[field] = "********"
		}
	}
	return public
}

func (s *Store) nameExists(ctx context.Context, name string, excludeID string) (bool, error) {
	var count int
	var err error
	if excludeID == "" {
		err = s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM notification_channels WHERE name = ?`, name).Scan(&count)
	} else {
		err = s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM notification_channels WHERE name = ? AND id <> ?`, name, excludeID).Scan(&count)
	}
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanChannel(scanner scanner) (Channel, error) {
	var item Channel
	var enabled int
	var settingsJSON string
	var secretFieldsJSON string
	var createdAt string
	var updatedAt string

	err := scanner.Scan(&item.ID, &item.Name, &item.Type, &enabled, &settingsJSON, &secretFieldsJSON, &createdAt, &updatedAt)
	if err != nil {
		return Channel{}, err
	}
	item.Enabled = enabled == 1
	if err := json.Unmarshal([]byte(settingsJSON), &item.Settings); err != nil {
		return Channel{}, err
	}
	if err := json.Unmarshal([]byte(secretFieldsJSON), &item.SecretFields); err != nil {
		return Channel{}, err
	}
	item.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return Channel{}, err
	}
	item.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		return Channel{}, err
	}
	if item.Settings == nil {
		item.Settings = map[string]string{}
	}
	return item, nil
}
