package settings

import (
	"context"
	"database/sql"
	"time"
)

type Item struct {
	Key       string    `json:"key"`
	Value     string    `json:"value"`
	Secret    bool      `json:"secret"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

func (s *Store) List(ctx context.Context) ([]Item, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT key, value, secret, updated_at FROM app_settings ORDER BY key ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []Item{}
	for rows.Next() {
		var item Item
		var secret int
		var updatedAt string
		if err := rows.Scan(&item.Key, &item.Value, &secret, &updatedAt); err != nil {
			return nil, err
		}
		item.Secret = secret == 1
		parsed, err := time.Parse(time.RFC3339Nano, updatedAt)
		if err != nil {
			return nil, err
		}
		item.UpdatedAt = parsed
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) Upsert(ctx context.Context, key string, value string, secret bool) error {
	secretValue := 0
	if secret {
		secretValue = 1
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(ctx, `INSERT INTO app_settings (key, value, secret, updated_at) VALUES (?, ?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, secret = excluded.secret, updated_at = excluded.updated_at`, key, value, secretValue, now)
	return err
}

func (s *Store) Delete(ctx context.Context, key string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM app_settings WHERE key = ?`, key)
	return err
}
