package locking

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"github.com/alex/assistent/internal/platform/config"
)

var ErrApplicationAlreadyRunning = errors.New("ApplicationAlreadyRunning")

func NewProcessLock(cfg config.DatabaseConfig) *ProcessLock {
	path, err := filepath.Abs(cfg.SQLitePath)
	if err != nil {
		path = filepath.Clean(cfg.SQLitePath)
	}
	return &ProcessLock{path: path + ".lock"}
}

type ProcessLock struct {
	path string
	file *os.File
}

func (l *ProcessLock) Acquire() error {
	if err := os.MkdirAll(filepath.Dir(l.path), 0700); err != nil {
		return fmt.Errorf("create process lock directory: %w", err)
	}
	file, err := os.OpenFile(l.path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return fmt.Errorf("open process lock: %w", err)
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = file.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return ErrApplicationAlreadyRunning
		}
		return fmt.Errorf("acquire process lock: %w", err)
	}
	l.file = file
	return nil
}

func (l *ProcessLock) Release() error {
	if l.file == nil {
		return nil
	}
	if err := syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN); err != nil {
		_ = l.file.Close()
		l.file = nil
		return fmt.Errorf("release process lock: %w", err)
	}
	err := l.file.Close()
	l.file = nil
	return err
}
