package drive

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"golang.org/x/oauth2"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alex/assistent/internal/domain/submission"
)

type failingRoundTripper struct{}

func (failingRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("transport failed")
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("read failed") }

func TestContentHelpersSelectExportsAndSafeExtensions(t *testing.T) {
	for _, item := range []struct {
		material submission.Material
		path     string
		ext      string
	}{
		{submission.Material{MIMEType: googleMIME + "document"}, "/export", ".docx"},
		{submission.Material{MIMEType: googleMIME + "spreadsheet"}, "/export", ".xlsx"},
		{submission.Material{MIMEType: "application/pdf"}, "", ".pdf"},
		{submission.Material{Name: "notes.TXT", MIMEType: "application/octet-stream"}, "", ".txt"},
		{submission.Material{Name: "notes.longextension", MIMEType: "application/octet-stream"}, "", ".bin"},
	} {
		path, _, ext := contentRequest(item.material)
		if path != item.path || ext != item.ext {
			t.Fatalf("contentRequest(%#v) = %q, %q", item.material, path, ext)
		}
	}
	if path, query, ext := contentRequest(submission.Material{MIMEType: googleMIME + "form"}); path != "" || query != nil || ext != "" {
		t.Fatalf("unsupported Google export = %q %v %q", path, query, ext)
	}
}

func TestSaveFilePublishesDigestAndRejectsOversizedOrCanceledInput(t *testing.T) {
	dir := t.TempDir()
	path, err := saveFile(context.Background(), dir, ".txt", strings.NewReader("content"))
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(path) != dir {
		t.Fatalf("saved path = %q", path)
	}
	if contents, err := os.ReadFile(path); err != nil || string(contents) != "content" {
		t.Fatalf("saved content = %q, %v", contents, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := saveFile(ctx, dir, ".txt", bytes.NewReader([]byte("content"))); err == nil {
		t.Fatal("canceled save unexpectedly succeeded")
	}
	if _, err := saveFile(context.Background(), dir, ".txt", io.MultiReader(strings.NewReader(strings.Repeat("x", maxFileBytes)), strings.NewReader("x"))); err == nil {
		t.Fatal("oversized save unexpectedly succeeded")
	}
	if _, err := saveFile(context.Background(), dir, ".txt", failingReader{}); err == nil {
		t.Fatal("reader failure unexpectedly succeeded")
	}
	digest := sha256.Sum256([]byte("directory"))
	if err := os.Mkdir(filepath.Join(dir, fmt.Sprintf("%x.txt", digest)), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := saveFile(context.Background(), dir, ".txt", strings.NewReader("directory")); err == nil {
		t.Fatal("directory artifact unexpectedly accepted")
	}
}

func TestMaterialRetrieverReportsRequestAndTransportFailures(t *testing.T) {
	retriever := &materialRetriever{baseURL: "://bad", client: http.DefaultClient, tokens: oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "token"})}
	if _, status := retriever.get(context.Background(), "file", nil, ""); status != submission.Failed {
		t.Fatalf("malformed request status = %s", status)
	}
	retriever.baseURL = "http://example.test"
	retriever.client = &http.Client{Transport: failingRoundTripper{}}
	if _, status := retriever.get(context.Background(), "file", nil, ""); status != submission.Failed {
		t.Fatalf("transport failure status = %s", status)
	}
}
