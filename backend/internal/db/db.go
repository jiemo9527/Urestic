package db

import (
	"database/sql"
	"os"
	"path/filepath"

	"github.com/urestic/urestic/backend/internal/config"
	_ "modernc.org/sqlite"
)

func Open(cfg config.Config) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(cfg.DatabasePath), 0o700); err != nil {
		return nil, err
	}

	database, err := sql.Open("sqlite", cfg.DatabasePath)
	if err != nil {
		return nil, err
	}
	database.SetMaxOpenConns(1)

	if err := migrate(database); err != nil {
		database.Close()
		return nil, err
	}

	return database, nil
}

func migrate(database *sql.DB) error {
	statements := []string{
		`PRAGMA foreign_keys = ON`,
		`PRAGMA journal_mode = WAL`,
		`PRAGMA busy_timeout = 5000`,
		`CREATE TABLE IF NOT EXISTS backup_repositories (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,
			backend TEXT NOT NULL,
			repo_url TEXT NOT NULL,
			password_ciphertext TEXT NOT NULL,
			variables_json TEXT NOT NULL DEFAULT '{}',
			secret_fields_json TEXT NOT NULL DEFAULT '[]',
			description TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_backup_repositories_backend ON backup_repositories(backend)`,
		`CREATE TABLE IF NOT EXISTS notification_channels (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,
			type TEXT NOT NULL,
			enabled INTEGER NOT NULL DEFAULT 1,
			settings_json TEXT NOT NULL DEFAULT '{}',
			secret_fields_json TEXT NOT NULL DEFAULT '[]',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_notification_channels_type ON notification_channels(type)`,
		`CREATE TABLE IF NOT EXISTS app_settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			secret INTEGER NOT NULL DEFAULT 0,
			updated_at TEXT NOT NULL
		)`,
	}

	for _, statement := range statements {
		if _, err := database.Exec(statement); err != nil {
			return err
		}
	}

	return nil
}
