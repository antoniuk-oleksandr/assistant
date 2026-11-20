package renderer

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alex/assistent/internal/platform/config"
)

func TestGoogleSheetsClientWritesThroughSheetsAPI(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/spreadsheets"):
			_ = json.NewEncoder(w).Encode(map[string]string{"spreadsheetId": "sheet-1"})
		default:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	defer server.Close()
	client := &googleSheetsClient{baseURL: server.URL, client: server.Client()}
	id, err := client.CreateSpreadsheet(context.Background(), "Report")
	if err != nil || id != "sheet-1" {
		t.Fatalf("CreateSpreadsheet() = %q, %v", id, err)
	}
	if err := client.AddSheets(context.Background(), id, []string{"Appendices", "Evidence"}); err != nil {
		t.Fatal(err)
	}
	if err := client.WriteValues(context.Background(), id, "Appendices", [][]any{{"A1", 1}}); err != nil {
		t.Fatal(err)
	}
	if len(paths) != 3 || paths[0] != "POST /spreadsheets" || !strings.Contains(paths[1], ":batchUpdate") || !strings.Contains(paths[2], "/values:batchUpdate") {
		t.Fatalf("paths = %#v", paths)
	}
}

func TestGoogleSheetsClientReportsAPIAndDecodeErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("upstream failure"))
	}))
	defer server.Close()
	client := &googleSheetsClient{baseURL: server.URL, client: server.Client()}
	if _, err := client.CreateSpreadsheet(context.Background(), "Report"); err == nil || !strings.Contains(err.Error(), "502") {
		t.Fatalf("error = %v", err)
	}

	decodeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("not-json")) }))
	defer decodeServer.Close()
	client.baseURL = decodeServer.URL
	if _, err := client.CreateSpreadsheet(context.Background(), "Report"); err == nil || !strings.Contains(err.Error(), "decode") {
		t.Fatalf("decode error = %v", err)
	}
}

func TestNewGoogleSheetsClientValidatesConfiguration(t *testing.T) {
	if _, err := newGoogleSheetsClient(config.SheetsConfig{BaseURL: "://bad"}); err == nil {
		t.Fatal("invalid URL accepted")
	}
	if _, err := newGoogleSheetsClient(config.SheetsConfig{}); err == nil {
		t.Fatal("missing credentials accepted")
	}
	if _, err := newGoogleSheetsClient(config.SheetsConfig{ClientID: "id", ClientSecret: "secret"}); err == nil {
		t.Fatal("partial credentials accepted")
	}
	client, err := newGoogleSheetsClient(config.SheetsConfig{BaseURL: "http://localhost:1234/v4", ClientID: "id", ClientSecret: "secret", RefreshToken: "refresh"})
	if err != nil || client == nil {
		t.Fatalf("valid configuration = %v", err)
	}
}
