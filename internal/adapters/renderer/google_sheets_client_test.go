package renderer

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/alex/assistent/internal/platform/config"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Google Sheets client", func() {
	It("writes through the Sheets API", func() {
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
		Expect(err).NotTo(HaveOccurred())
		Expect(id).To(Equal("sheet-1"))
		Expect(client.AddSheets(context.Background(), id, []string{"Appendices", "Evidence"})).To(Succeed())
		Expect(client.WriteValues(context.Background(), id, "Appendices", [][]any{{"A1", 1}})).To(Succeed())
		Expect(paths).To(HaveLen(3))
		Expect(paths[0]).To(Equal("POST /spreadsheets"))
		Expect(paths[1]).To(ContainSubstring(":batchUpdate"))
		Expect(paths[2]).To(ContainSubstring("/values:batchUpdate"))
		Expect(client.WriteRanges(context.Background(), id, []sheetValueRange{{rangeName: "Аркуш1!C13", values: [][]any{{"/"}}}})).To(Succeed())
		Expect(client.ClearRanges(context.Background(), id, []string{"Аркуш1!C12"})).To(Succeed())
	})

	It("reports API and decode errors", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte("upstream failure"))
		}))
		defer server.Close()
		client := &googleSheetsClient{baseURL: server.URL, client: server.Client()}
		_, err := client.CreateSpreadsheet(context.Background(), "Report")
		Expect(err).To(MatchError(ContainSubstring("502")))

		decodeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("not-json")) }))
		defer decodeServer.Close()
		client.baseURL = decodeServer.URL
		_, err = client.CreateSpreadsheet(context.Background(), "Report")
		Expect(err).To(MatchError(ContainSubstring("decode")))
	})

	It("rejects a missing spreadsheet ID", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"kind":"sheets"}`))
		}))
		defer server.Close()
		client := &googleSheetsClient{baseURL: server.URL, client: server.Client()}
		_, err := client.CreateSpreadsheet(context.Background(), "Report")
		Expect(err).To(MatchError(ContainSubstring("spreadsheetId")))
	})

	It("validates configuration", func() {
		_, err := newGoogleSheetsClient(config.SheetsConfig{BaseURL: "://bad"})
		Expect(err).To(HaveOccurred())
		_, err = newGoogleSheetsClient(config.SheetsConfig{})
		Expect(err).To(HaveOccurred())
		_, err = newGoogleSheetsClient(config.SheetsConfig{ClientID: "id", ClientSecret: "secret"})
		Expect(err).To(HaveOccurred())
		for _, baseURL := range []string{"https://user:pass@example.com/v4", "https://example.com/v4?x=1", "ftp://example.com/v4"} {
			_, err := newGoogleSheetsClient(config.SheetsConfig{BaseURL: baseURL, ClientID: "id", ClientSecret: "secret", RefreshToken: "refresh"})
			Expect(err).To(HaveOccurred())
		}
		client, err := newGoogleSheetsClient(config.SheetsConfig{BaseURL: "http://localhost:1234/v4", ClientID: "id", ClientSecret: "secret", RefreshToken: "refresh"})
		Expect(err).NotTo(HaveOccurred())
		Expect(client).NotTo(BeNil())
	})
})
