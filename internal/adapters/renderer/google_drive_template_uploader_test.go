package renderer

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"

	"github.com/alex/assistent/internal/platform/config"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"golang.org/x/oauth2"
)

var _ = Describe("Google Drive template uploader", func() {
	It("imports a workbook", func() {
		templatePath := GinkgoT().TempDir() + "/template.xlsx"
		Expect(os.WriteFile(templatePath, []byte("xlsx bytes"), 0600)).To(Succeed())
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost || !strings.Contains(r.URL.RawQuery, "uploadType=multipart") {
				Expect(r.Method).To(Equal(http.MethodPost))
				Expect(r.URL.RawQuery).To(ContainSubstring("uploadType=multipart"))
			}
			if got := r.Header.Get("Authorization"); got != "Bearer token" {
				Expect(got).To(Equal("Bearer token"))
			}
			body, err := io.ReadAll(r.Body)
			if err != nil || !strings.Contains(string(body), "xlsx bytes") {
				Expect(err).NotTo(HaveOccurred())
				Expect(string(body)).To(ContainSubstring("xlsx bytes"))
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
		Expect(err).NotTo(HaveOccurred())
		Expect(id).To(Equal("sheet-123"))
	})

	It("reports upload failures", func() {
		uploader := &googleDriveTemplateUploader{endpoint: "http://127.0.0.1:1", client: http.DefaultClient}
		_, err := uploader.UploadSpreadsheet(context.Background(), "Report", "/does/not/exist.xlsx")
		Expect(err).To(MatchError(ContainSubstring("open report template")))
		templatePath := GinkgoT().TempDir() + "/template.xlsx"
		Expect(os.WriteFile(templatePath, []byte("xlsx"), 0600)).To(Succeed())
		_, err = uploader.UploadSpreadsheet(context.Background(), "Report", templatePath)
		Expect(err).To(HaveOccurred())
		for name, handler := range map[string]http.Handler{
			"http error": http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusBadGateway)
				_, _ = w.Write([]byte("bad gateway"))
			}),
			"invalid json": http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("not-json")) }),
			"empty id":     http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{}`)) }),
		} {
			By(name)
			server := httptest.NewServer(handler)
			defer server.Close()
			candidate := &googleDriveTemplateUploader{endpoint: server.URL, client: server.Client()}
			_, err := candidate.UploadSpreadsheet(context.Background(), "Report", templatePath)
			Expect(err).To(HaveOccurred())
		}
	})

	It("validates configuration", func() {
		uploader, err := newGoogleDriveTemplateUploader(config.DriveConfig{})
		Expect(err).NotTo(HaveOccurred())
		Expect(uploader).To(BeNil())
		_, err = newGoogleDriveTemplateUploader(config.DriveConfig{ClientID: "id"})
		Expect(err).To(HaveOccurred())
		_, err = newGoogleDriveTemplateUploader(config.DriveConfig{ClientID: "id", ClientSecret: "secret", RefreshToken: "refresh", BaseURL: "://bad"})
		Expect(err).To(HaveOccurred())
		uploader, err = newGoogleDriveTemplateUploader(config.DriveConfig{ClientID: "id", ClientSecret: "secret", RefreshToken: "refresh", BaseURL: "http://localhost:1234/v3"})
		Expect(err).NotTo(HaveOccurred())
		Expect(uploader).NotTo(BeNil())
	})
})
