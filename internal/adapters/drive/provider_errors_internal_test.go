package drive

import (
	"errors"
	"io"
	"strings"
	"testing"
)

type failingErrorReader struct{}

func (failingErrorReader) Read([]byte) (int, error) { return 0, errors.New("read failed") }

func TestIsQuotaErrorRejectsUnreadableAndOversizedBodies(t *testing.T) {
	if isQuotaError(failingErrorReader{}) {
		t.Fatal("unreadable error body classified as quota")
	}
	if isQuotaError(strings.NewReader(strings.Repeat("x", maxErrorBody+1))) {
		t.Fatal("oversized error body classified as quota")
	}
	if isQuotaError(io.LimitReader(strings.NewReader(`{"error":{"status":"RESOURCE_EXHAUSTED"}}`), maxErrorBody+1)) != true {
		t.Fatal("resource exhausted body was not classified as quota")
	}
}
