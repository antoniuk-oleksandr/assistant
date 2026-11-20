package locking_test

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/alex/assistent/internal/platform/config"
	"github.com/alex/assistent/internal/platform/locking"
)

func TestProcessLockRejectsDuplicateAndAllowsAcquireAfterRelease(t *testing.T) {
	cfg := config.DatabaseConfig{SQLitePath: filepath.Join(t.TempDir(), "state.db")}
	first := locking.NewProcessLock(cfg)
	second := locking.NewProcessLock(cfg)

	if err := first.Acquire(); err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	if err := second.Acquire(); !errors.Is(err, locking.ErrApplicationAlreadyRunning) {
		t.Fatalf("second acquire error = %v, want %v", err, locking.ErrApplicationAlreadyRunning)
	}
	if err := first.Release(); err != nil {
		t.Fatalf("first release: %v", err)
	}
	if err := second.Acquire(); err != nil {
		t.Fatalf("acquire after release: %v", err)
	}
	if err := second.Release(); err != nil {
		t.Fatalf("second release: %v", err)
	}
}
