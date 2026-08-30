package app

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/crypto/bcrypt"
)

var ErrAdministratorNotConfigured = errors.New("BlueScale administrator is not configured")

type AdministratorRecovery struct {
	db           *sql.DB
	instanceLock *instanceLock
	databasePath string
}

func OpenAdministratorRecovery(dataDir string) (*AdministratorRecovery, error) {
	if dataDir == "" {
		return nil, errors.New("data directory is required")
	}
	absoluteDataDir, err := filepath.Abs(dataDir)
	if err != nil {
		return nil, err
	}
	databasePath := filepath.Join(absoluteDataDir, "bluescale.db")
	info, err := os.Stat(databasePath)
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("BlueScale database does not exist at %s", databasePath)
	}
	if err != nil {
		return nil, fmt.Errorf("inspect BlueScale database: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("BlueScale database path is not a regular file: %s", databasePath)
	}

	instanceLock, err := acquireInstanceLock(absoluteDataDir)
	if err != nil {
		return nil, err
	}
	releaseLock := true
	defer func() {
		if releaseLock {
			_ = instanceLock.Close()
		}
	}()
	if err := os.Chmod(absoluteDataDir, 0o700); err != nil {
		return nil, fmt.Errorf("secure data directory: %w", err)
	}
	if err := os.Chmod(databasePath, 0o600); err != nil {
		return nil, fmt.Errorf("secure BlueScale database: %w", err)
	}

	db, err := sql.Open("sqlite", databasePath)
	if err != nil {
		return nil, fmt.Errorf("open BlueScale database: %w", err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`PRAGMA busy_timeout = 0`); err != nil {
		db.Close()
		return nil, fmt.Errorf("configure BlueScale database: %w", err)
	}
	recovery := &AdministratorRecovery{db: db, instanceLock: instanceLock, databasePath: databasePath}
	if _, err := recovery.Username(); err != nil {
		db.Close()
		return nil, err
	}
	releaseLock = false
	return recovery, nil
}

func (r *AdministratorRecovery) Close() error {
	return errors.Join(r.db.Close(), r.instanceLock.Close())
}

func (r *AdministratorRecovery) DatabasePath() string {
	return r.databasePath
}

func (r *AdministratorRecovery) Username() (string, error) {
	var username string
	err := r.db.QueryRow(`SELECT username FROM administrators WHERE id = 1`).Scan(&username)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrAdministratorNotConfigured
	}
	if err != nil {
		return "", fmt.Errorf("read administrator: %w", err)
	}
	return username, nil
}

func (r *AdministratorRecovery) ResetUsername(value string) (string, error) {
	username, err := normalizeAdministratorUsername(value)
	if err != nil {
		return "", err
	}
	tx, err := r.db.Begin()
	if err != nil {
		return "", fmt.Errorf("start administrator reset: %w", err)
	}
	defer tx.Rollback()
	result, err := tx.Exec(`UPDATE administrators SET username = ? WHERE id = 1`, username)
	if err != nil {
		return "", fmt.Errorf("reset administrator username: %w", err)
	}
	if err := requireUpdatedAdministrator(result); err != nil {
		return "", err
	}
	if _, err := tx.Exec(`DELETE FROM sessions`); err != nil {
		return "", fmt.Errorf("revoke login sessions: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("commit administrator reset: %w", err)
	}
	return username, nil
}

func (r *AdministratorRecovery) ResetPassword(password []byte, revokeAPITokens bool) error {
	if err := validateAdministratorPassword(password); err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword(password, bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash administrator password: %w", err)
	}
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("start administrator reset: %w", err)
	}
	defer tx.Rollback()
	result, err := tx.Exec(`UPDATE administrators SET password_hash = ? WHERE id = 1`, string(hash))
	if err != nil {
		return fmt.Errorf("reset administrator password: %w", err)
	}
	if err := requireUpdatedAdministrator(result); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM sessions`); err != nil {
		return fmt.Errorf("revoke login sessions: %w", err)
	}
	if revokeAPITokens {
		if _, err := tx.Exec(`DELETE FROM api_tokens`); err != nil {
			return fmt.Errorf("revoke API tokens: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit administrator reset: %w", err)
	}
	return nil
}

func requireUpdatedAdministrator(result sql.Result) error {
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("verify administrator reset: %w", err)
	}
	if rowsAffected != 1 {
		return ErrAdministratorNotConfigured
	}
	return nil
}
