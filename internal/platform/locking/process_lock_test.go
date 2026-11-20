package locking_test

import (
	"errors"
	"os"
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

func TestProcessLockReleaseBeforeAcquireIsNoOp(t *testing.T) {
	lock := locking.NewProcessLock(config.DatabaseConfig{SQLitePath: filepath.Join(t.TempDir(), "state.db")})
	if err := lock.Release(); err != nil {
		t.Fatalf("release before acquire: %v", err)
	}
}

func TestProcessLockReportsLockDirectoryErrors(t *testing.T) {
	parent := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(parent, []byte("file"), 0600); err != nil {
		t.Fatalf("write parent file: %v", err)
	}
	lock := locking.NewProcessLock(config.DatabaseConfig{SQLitePath: filepath.Join(parent, "state.db")})
	if err := lock.Acquire(); err == nil {
		t.Fatal("Acquire() unexpectedly succeeded")
	}
}

func TestProcessLockReportsLockFileErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	if err := os.Mkdir(path+".lock", 0700); err != nil {
		t.Fatalf("create lock path: %v", err)
	}
	lock := locking.NewProcessLock(config.DatabaseConfig{SQLitePath: path})
	if err := lock.Acquire(); err == nil {
		t.Fatal("Acquire() unexpectedly succeeded")
	}
}
