package locking

import (
	"path/filepath"
	"testing"

	"github.com/alex/assistent/internal/platform/config"
)

func TestProcessLockReportsReleaseFailure(t *testing.T) {
	lock := NewProcessLock(config.DatabaseConfig{SQLitePath: filepath.Join(t.TempDir(), "state.db")})
	if err := lock.Acquire(); err != nil {
		t.Fatal(err)
	}
	if err := lock.file.Close(); err != nil {
		t.Fatal(err)
	}
	if err := lock.Release(); err == nil {
		t.Fatal("Release() unexpectedly succeeded for a closed lock file")
	}
}
