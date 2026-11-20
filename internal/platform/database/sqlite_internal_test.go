package database

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFileDSNRejectsDirectoryCreationBelowAFile(t *testing.T) {
	file := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(file, []byte("file"), 0600); err != nil {
		t.Fatal(err)
	}

	_, err := fileDSN(filepath.Join(file, "state.db"))
	if err == nil {
		t.Fatal("fileDSN returned nil error")
	}
}
