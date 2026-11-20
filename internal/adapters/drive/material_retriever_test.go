package drive_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/alex/assistent/internal/adapters/drive"
	"github.com/alex/assistent/internal/domain/submission"
	"github.com/alex/assistent/internal/platform/config"
	. "github.com/onsi/gomega"
	"golang.org/x/oauth2"
)

func fixtureToken() oauth2.TokenSource {
	return oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "test-token"})
}

func TestRetrieveSavesDriveDownloadsAndWorkspaceExports(t *testing.T) {
	const docx = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	const xlsx = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	for _, tt := range []struct{ name, mime, exportMIME, extension string }{
		{"PDF", "application/pdf", "", ".pdf"},
		{"Word", docx, "", ".docx"},
		{"Excel", xlsx, "", ".xlsx"},
		{"JPEG", "image/jpeg", "", ".jpg"},
		{"PNG", "image/png", "", ".png"},
		{"Google Docs", "application/vnd.google-apps.document", docx, ".docx"},
		{"Google Sheets", "application/vnd.google-apps.spreadsheet", xlsx, ".xlsx"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			const source = "https://drive.google.com/file/d/file-1/view"
			const contents = "\x00\xfftest file"
			type request struct {
				method, path, token string
				query               url.Values
			}
			requests := make(chan request, 4)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests <- request{r.Method, r.URL.Path, r.Header.Get("Authorization"), r.URL.Query()}
				if r.URL.Query().Has("fields") {
					size := ""
					if tt.exportMIME == "" {
						size = fmt.Sprintf(`,"size":%q`, fmt.Sprint(len(contents)))
					}
					fmt.Fprintf(w, `{"id":"file-1","name":"../../provider-name","mimeType":%q,"webViewLink":%q,"capabilities":{"canDownload":true}%s}`, tt.mime, source, size)
					return
				}
				fmt.Fprint(w, contents)
			}))
			t.Cleanup(server.Close)
			dir := t.TempDir()
			retriever, err := drive.NewMaterialRetrieverWithTokenSource(config.DriveConfig{BaseURL: server.URL + "/drive/v3", MaterialDir: dir}, fixtureToken())
			g.Expect(err).NotTo(HaveOccurred())
			got := retriever.Retrieve(context.Background(), submission.Material{ExternalID: "file-1", Name: "classroom-name", Status: submission.Pending})
			g.Expect(got.Status).To(Equal(submission.Pending))
			g.Expect(got.ExternalID).To(Equal("file-1"))
			g.Expect(got.Name).To(Equal("../../provider-name"))
			g.Expect(got.MIMEType).To(Equal(tt.mime))
			g.Expect(got.SourceURI).To(Equal(source))
			if tt.exportMIME == "" {
				g.Expect(got.SizeBytes).To(HaveValue(Equal(int64(len(contents)))))
			} else {
				g.Expect(got.SizeBytes).To(BeNil())
			}
			g.Expect(filepath.Dir(got.LocalPath)).To(Equal(dir))
			g.Expect(filepath.Ext(got.LocalPath)).To(Equal(tt.extension))
			data, err := os.ReadFile(got.LocalPath)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(data).To(Equal([]byte(contents)))

			g.Expect(requests).To(HaveLen(2))
			metadata, content := <-requests, <-requests
			for _, req := range []request{metadata, content} {
				g.Expect(req.method).To(Equal(http.MethodGet))
				g.Expect(req.token).To(Equal("Bearer test-token"))
			}
			g.Expect(metadata.path).To(Equal("/drive/v3/files/file-1"))
			g.Expect(metadata.query.Get("fields")).To(Equal("id,name,mimeType,size,webViewLink,capabilities"))
			if tt.exportMIME == "" {
				g.Expect(content.path).To(Equal(metadata.path))
				g.Expect(content.query.Get("alt")).To(Equal("media"))
			} else {
				g.Expect(content.path).To(Equal(metadata.path + "/export"))
				g.Expect(content.query.Get("mimeType")).To(Equal(tt.exportMIME))
			}
		})
	}
}

func TestRetrieveRecordsProviderAndDownloadFailuresWithoutSavingAFile(t *testing.T) {
	const metadata = `{"id":"file-1","mimeType":"application/pdf"}`
	for _, tt := range []struct {
		name, metadata            string
		metadataCode, contentCode int
		want                      submission.MaterialStatus
	}{
		{"metadata forbidden", "", 403, 0, submission.AccessDenied},
		{"permission forbidden", `{"error":{"errors":[{"reason":"insufficientFilePermissions"}]}}`, 403, 0, submission.AccessDenied},
		{"rate limited", `{"error":{"errors":[{"reason":"userRateLimitExceeded"}]}}`, 403, 0, submission.Failed},
		{"quota exceeded", `{"error":{"details":[{"reason":"DOWNLOAD_QUOTA_EXCEEDED"}]}}`, 403, 0, submission.Failed},
		{"resource exhausted", `{"error":{"status":"RESOURCE_EXHAUSTED"}}`, 403, 0, submission.Failed},
		{"unauthorized", "", 401, 0, submission.AccessDenied},
		{"metadata missing", "", 404, 0, submission.NotFound},
		{"metadata server error", "", 500, 0, submission.Failed},
		{"malformed metadata", "not JSON", 200, 0, submission.Failed},
		{"negative size", `{"id":"file-1","mimeType":"application/pdf","size":"-1"}`, 200, 0, submission.Failed},
		{"oversized file metadata", `{"id":"file-1","mimeType":"application/pdf","size":"104857601"}`, 200, 0, submission.Failed},
		{"unexportable type", `{"id":"file-1","name":"Slides","mimeType":"application/vnd.google-apps.presentation"}`, 200, 0, submission.Pending},
		{"download disabled", `{"id":"file-1","mimeType":"application/pdf","capabilities":{"canDownload":false}}`, 200, 0, submission.AccessDenied},
		{"download forbidden", metadata, 200, 403, submission.AccessDenied},
		{"download missing", metadata, 200, 404, submission.NotFound},
		{"truncated download", metadata, 200, 200, submission.Failed},
	} {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Query().Has("fields") {
					w.WriteHeader(tt.metadataCode)
					fmt.Fprint(w, tt.metadata)
					return
				}
				if tt.contentCode == 0 {
					t.Error("unexpected content request")
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				if tt.contentCode == http.StatusOK {
					w.Header().Set("Content-Length", "100")
				}
				w.WriteHeader(tt.contentCode)
				fmt.Fprint(w, "short response")
			}))
			t.Cleanup(server.Close)
			dir := t.TempDir()
			retriever, err := drive.NewMaterialRetrieverWithTokenSource(config.DriveConfig{BaseURL: server.URL, MaterialDir: dir}, fixtureToken())
			g.Expect(err).NotTo(HaveOccurred())
			const source = "https://drive.google.com/file/d/file-1/view"
			got := retriever.Retrieve(context.Background(), submission.Material{ExternalID: "file-1", Name: "report.pdf", SourceURI: source})
			g.Expect(got.Status).To(Equal(tt.want))
			g.Expect(got.ExternalID).To(Equal("file-1"))
			g.Expect(got.SourceURI).To(Equal(source))
			g.Expect(got.LocalPath).To(BeEmpty())
			if tt.want == submission.Pending {
				g.Expect(got.Name).To(Equal("Slides"))
				g.Expect(got.MIMEType).To(Equal("application/vnd.google-apps.presentation"))
				g.Expect(got.Parsing).To(HaveValue(Equal(submission.ParseResult{Issue: submission.UnsupportedMaterial})))
			} else {
				g.Expect(got.Parsing).To(BeNil())
			}
			entries, err := os.ReadDir(dir)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(entries).To(BeEmpty())
		})
	}
}
