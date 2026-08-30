package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

var ErrInstanceRunning = errors.New("another BlueScale process is using this data directory")

type instanceLock struct {
	file *os.File
}

func acquireInstanceLock(dataDir string) (*instanceLock, error) {
	lockPath := filepath.Join(dataDir, ".bluescale.lock")
	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open instance lock: %w", err)
	}
	if err := os.Chmod(lockPath, 0o600); err != nil {
		file.Close()
		return nil, fmt.Errorf("secure instance lock: %w", err)
	}
	if err := tryLockFile(file); err != nil {
		file.Close()
		return nil, err
	}
	return &instanceLock{file: file}, nil
}

func (l *instanceLock) Close() error {
	if l == nil || l.file == nil {
		return nil
	}
	file := l.file
	l.file = nil
	return errors.Join(unlockFile(file), file.Close())
}
