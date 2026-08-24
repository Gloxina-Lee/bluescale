package app

import "database/sql"

func initializeDatabase(db *sql.DB) error {
	statements := []string{
		`PRAGMA journal_mode = WAL`,
		`PRAGMA foreign_keys = ON`,
		`PRAGMA busy_timeout = 5000`,
		`CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS administrators (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			display_name TEXT NOT NULL,
			username TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL,
			created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
		)`,
		`CREATE TABLE IF NOT EXISTS user_groups (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL UNIQUE COLLATE NOCASE,
			system_key TEXT UNIQUE,
			can_upload INTEGER NOT NULL DEFAULT 0 CHECK (can_upload IN (0, 1)),
			can_manage_images INTEGER NOT NULL DEFAULT 0 CHECK (can_manage_images IN (0, 1)),
			can_manage_users INTEGER NOT NULL DEFAULT 0 CHECK (can_manage_users IN (0, 1)),
			created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
			updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
		)`,
		`CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			display_name TEXT NOT NULL,
			username TEXT NOT NULL UNIQUE COLLATE NOCASE,
			password_hash TEXT NOT NULL,
			group_id INTEGER NOT NULL REFERENCES user_groups(id) ON DELETE RESTRICT,
			created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
			updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
		)`,
		`CREATE INDEX IF NOT EXISTS idx_users_group_id ON users(group_id)`,
		`CREATE TABLE IF NOT EXISTS images (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			original_name TEXT NOT NULL,
			storage_name TEXT NOT NULL UNIQUE,
			mime_type TEXT NOT NULL,
			size INTEGER NOT NULL,
			created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
		)`,
		`CREATE INDEX IF NOT EXISTS idx_images_created_at ON images(created_at DESC)`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			return err
		}
	}
	if _, err := db.Exec(`INSERT INTO user_groups (name, system_key, can_upload, can_manage_images, can_manage_users)
		VALUES ('Admin', 'admin', 1, 1, 1)
		ON CONFLICT(system_key) DO NOTHING`); err != nil {
		return err
	}
	if _, err := db.Exec(`INSERT INTO user_groups (name, system_key, can_upload, can_manage_images, can_manage_users)
		VALUES ('User', 'user', 1, 0, 0)
		ON CONFLICT(system_key) DO NOTHING`); err != nil {
		return err
	}
	// Existing single-user installations are migrated without changing their credentials.
	if _, err := db.Exec(`INSERT INTO users (display_name, username, password_hash, group_id, created_at, updated_at)
		SELECT a.display_name, a.username, a.password_hash, g.id, a.created_at, a.created_at
		FROM administrators a
		JOIN user_groups g ON g.system_key = 'admin'
		WHERE NOT EXISTS (SELECT 1 FROM users)`); err != nil {
		return err
	}
	return nil
}
