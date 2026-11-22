package parser

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/alex/assistent/internal/domain/submission"
)

func TestLocalTextSignalsReportsCancellationAfterReading(t *testing.T) {
	path := filepath.Join(t.TempDir(), "text.txt")
	if err := os.WriteFile(path, []byte("publication evidence"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err := localTextSignals(ctx, submission.Material{Name: "text.txt", MIMEType: "text/plain", LocalPath: path})
	if err != context.Canceled {
		t.Fatalf("localTextSignals() error = %v", err)
	}
}

func TestLocalTextSignalsReportsUnexpectedFileErrors(t *testing.T) {
	if _, _, err := localTextSignals(context.Background(), submission.Material{Name: "text.txt", MIMEType: "text/plain", LocalPath: "bad\x00path"}); err == nil {
		t.Fatal("invalid path unexpectedly succeeded")
	}
	if _, _, err := localTextSignals(context.Background(), submission.Material{Name: "text.txt", MIMEType: "text/plain", LocalPath: t.TempDir()}); err == nil {
		t.Fatal("directory unexpectedly read as text")
	}
}
