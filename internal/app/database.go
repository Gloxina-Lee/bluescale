package app

import (
	"database/sql"
	"errors"
)

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
			username TEXT NOT NULL UNIQUE COLLATE NOCASE,
			password_hash TEXT NOT NULL,
			created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
		)`,
		`CREATE TABLE IF NOT EXISTS images (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			original_name TEXT NOT NULL,
			storage_name TEXT NOT NULL UNIQUE,
			mime_type TEXT NOT NULL,
			size INTEGER NOT NULL,
			is_public INTEGER NOT NULL DEFAULT 0 CHECK (is_public IN (0, 1)),
			created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
		)`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			return err
		}
	}
	if err := migrateAdministratorFromMultiUser(db); err != nil {
		return err
	}
	return migrateAdministratorUsernameOnly(db)
}

func migrateAdministratorFromMultiUser(db *sql.DB) error {
	hasUsers, err := tableExists(db, "users")
	if err != nil || !hasUsers {
		return err
	}
	hasGroups, err := tableExists(db, "user_groups")
	if err != nil {
		return err
	}

	var displayName, username, passwordHash, createdAt string
	if hasGroups {
		err = db.QueryRow(`SELECT u.display_name, u.username, u.password_hash, u.created_at
			FROM users u LEFT JOIN user_groups g ON g.id = u.group_id
			ORDER BY CASE
				WHEN g.system_key = 'admin' THEN 0
				WHEN g.can_manage_users = 1 THEN 1
				ELSE 2
			END, u.id
			LIMIT 1`).Scan(&displayName, &username, &passwordHash, &createdAt)
	} else {
		err = db.QueryRow(`SELECT display_name, username, password_hash, created_at
			FROM users ORDER BY id LIMIT 1`).Scan(&displayName, &username, &passwordHash, &createdAt)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	hasDisplayName, err := tableHasColumn(db, "administrators", "display_name")
	if err != nil {
		return err
	}
	if hasDisplayName {
		_, err = db.Exec(`INSERT INTO administrators (id, display_name, username, password_hash, created_at)
			VALUES (1, ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET
				display_name = excluded.display_name,
				username = excluded.username,
				password_hash = excluded.password_hash,
				created_at = excluded.created_at`, displayName, username, passwordHash, createdAt)
		return err
	}
	_, err = db.Exec(`INSERT INTO administrators (id, username, password_hash, created_at)
		VALUES (1, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			username = excluded.username,
			password_hash = excluded.password_hash,
			created_at = excluded.created_at`, username, passwordHash, createdAt)
	return err
}

func migrateAdministratorUsernameOnly(db *sql.DB) error {
	hasDisplayName, err := tableHasColumn(db, "administrators", "display_name")
	if err != nil || !hasDisplayName {
		return err
	}
	_, err = db.Exec(`ALTER TABLE administrators DROP COLUMN display_name`)
	return err
}

func finalizeSingleUserDatabase(db *sql.DB) (finalErr error) {
	if _, err := db.Exec(`PRAGMA foreign_keys = OFF`); err != nil {
		return err
	}
	defer func() {
		if _, err := db.Exec(`PRAGMA foreign_keys = ON`); finalErr == nil && err != nil {
			finalErr = err
		}
	}()

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	hasImageOwner, err := tableHasColumn(tx, "images", "user_id")
	if err != nil {
		return err
	}
	if hasImageOwner {
		statements := []string{
			`DROP INDEX IF EXISTS idx_images_user_id_created_at`,
			`CREATE TABLE images_single_user (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				original_name TEXT NOT NULL,
				storage_name TEXT NOT NULL UNIQUE,
				mime_type TEXT NOT NULL,
				size INTEGER NOT NULL,
				is_public INTEGER NOT NULL DEFAULT 0 CHECK (is_public IN (0, 1)),
				created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
			)`,
			`INSERT INTO images_single_user (id, original_name, storage_name, mime_type, size, is_public, created_at)
				SELECT id, original_name, storage_name, mime_type, size, 0, created_at FROM images`,
			`DROP TABLE images`,
			`ALTER TABLE images_single_user RENAME TO images`,
		}
		for _, statement := range statements {
			if _, err := tx.Exec(statement); err != nil {
				return err
			}
		}
	}

	hasSessionOwner, err := tableHasColumn(tx, "sessions", "user_id")
	if err != nil {
		return err
	}
	if hasSessionOwner {
		if _, err := tx.Exec(`DROP TABLE sessions`); err != nil {
			return err
		}
	}
	statements := []string{
		`CREATE TABLE IF NOT EXISTS sessions (
			token_hash TEXT PRIMARY KEY,
			expires_at INTEGER NOT NULL,
			created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
		)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON sessions(expires_at)`,
		`CREATE INDEX IF NOT EXISTS idx_images_created_at ON images(created_at DESC)`,
		`DROP TABLE IF EXISTS users`,
		`DROP TABLE IF EXISTS user_groups`,
	}
	for _, statement := range statements {
		if _, err := tx.Exec(statement); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func initializeImageVisibilityDatabase(db *sql.DB) error {
	hasVisibility, err := tableHasColumn(db, "images", "is_public")
	if err != nil {
		return err
	}
	if !hasVisibility {
		if _, err := db.Exec(`ALTER TABLE images ADD COLUMN is_public INTEGER NOT NULL DEFAULT 0 CHECK (is_public IN (0, 1))`); err != nil {
			return err
		}
	}
	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_images_public_created_at ON images(is_public, created_at DESC, id DESC)`)
	return err
}

type queryer interface {
	QueryRow(query string, args ...any) *sql.Row
}

func tableExists(db queryer, name string) (bool, error) {
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, name).Scan(&count)
	return count > 0, err
}

func tableHasColumn(db interface {
	Query(query string, args ...any) (*sql.Rows, error)
}, table, column string) (bool, error) {
	exists, err := tableExistsFromQuery(db, table)
	if err != nil || !exists {
		return false, err
	}
	rows, err := db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, columnType string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
}

func tableExistsFromQuery(db interface {
	Query(query string, args ...any) (*sql.Rows, error)
}, name string) (bool, error) {
	rows, err := db.Query(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, name)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	if !rows.Next() {
		return false, rows.Err()
	}
	var count int
	if err := rows.Scan(&count); err != nil {
		return false, err
	}
	return count > 0, rows.Err()
}
