package renderer

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/alex/assistent/internal/platform/config"
	"golang.org/x/oauth2"
)

func TestGoogleDriveTemplateUploaderImportsWorkbook(t *testing.T) {
	templatePath := t.TempDir() + "/template.xlsx"
	if err := os.WriteFile(templatePath, []byte("xlsx bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.Contains(r.URL.RawQuery, "uploadType=multipart") {
			t.Fatalf("request = %s %s", r.Method, r.URL.String())
		}
		if got := r.Header.Get("Authorization"); got != "Bearer token" {
			t.Fatalf("authorization = %q", got)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil || !strings.Contains(string(body), "xlsx bytes") {
			t.Fatalf("multipart body did not contain template: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"id": "sheet-123"})
	}))
	defer server.Close()

	uploader := &googleDriveTemplateUploader{
		endpoint: server.URL,
		client:   oauth2.NewClient(context.Background(), oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "token"})),
	}
	id, err := uploader.UploadSpreadsheet(context.Background(), "Report", templatePath)
	if err != nil {
		t.Fatal(err)
	}
	if id != "sheet-123" {
		t.Fatalf("id = %q", id)
	}
}

func TestGoogleDriveTemplateUploaderReportsUploadFailures(t *testing.T) {
	uploader := &googleDriveTemplateUploader{endpoint: "http://127.0.0.1:1", client: http.DefaultClient}
	if _, err := uploader.UploadSpreadsheet(context.Background(), "Report", "/does/not/exist.xlsx"); err == nil || !strings.Contains(err.Error(), "open report template") {
		t.Fatalf("missing file error = %v", err)
	}
	templatePath := t.TempDir() + "/template.xlsx"
	if err := os.WriteFile(templatePath, []byte("xlsx"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := uploader.UploadSpreadsheet(context.Background(), "Report", templatePath); err == nil {
		t.Fatal("transport failure unexpectedly succeeded")
	}
	for name, handler := range map[string]http.Handler{
		"http error": http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte("bad gateway"))
		}),
		"invalid json": http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("not-json")) }),
		"empty id":     http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{}`)) }),
	} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(handler)
			defer server.Close()
			candidate := &googleDriveTemplateUploader{endpoint: server.URL, client: server.Client()}
			if _, err := candidate.UploadSpreadsheet(context.Background(), "Report", templatePath); err == nil {
				t.Fatal("upload unexpectedly succeeded")
			}
		})
	}
}

func TestNewGoogleDriveTemplateUploaderValidatesConfiguration(t *testing.T) {
	if uploader, err := newGoogleDriveTemplateUploader(config.DriveConfig{}); err != nil || uploader != nil {
		t.Fatalf("empty config = %#v, %v", uploader, err)
	}
	if _, err := newGoogleDriveTemplateUploader(config.DriveConfig{ClientID: "id"}); err == nil {
		t.Fatal("partial config accepted")
	}
	if _, err := newGoogleDriveTemplateUploader(config.DriveConfig{ClientID: "id", ClientSecret: "secret", RefreshToken: "refresh", BaseURL: "://bad"}); err == nil {
		t.Fatal("invalid URL accepted")
	}
	uploader, err := newGoogleDriveTemplateUploader(config.DriveConfig{ClientID: "id", ClientSecret: "secret", RefreshToken: "refresh", BaseURL: "http://localhost:1234/v3"})
	if err != nil || uploader == nil {
		t.Fatalf("valid config = %#v, %v", uploader, err)
	}
}
