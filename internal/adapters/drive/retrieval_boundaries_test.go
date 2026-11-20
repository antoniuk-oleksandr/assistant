package drive_test

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/alex/assistent/internal/adapters/drive"
	"github.com/alex/assistent/internal/domain/submission"
	"github.com/alex/assistent/internal/platform/config"
	. "github.com/onsi/gomega"
	"golang.org/x/oauth2"
)

func TestDriveEndpointValidation(t *testing.T) {
	for _, raw := range []string{"", "https://fixture.invalid/drive/v3", "http://[::1]:8080", "http://localhost:8080"} {
		_, err := drive.NewMaterialRetrieverWithTokenSource(config.DriveConfig{BaseURL: raw}, fixtureToken())
		NewWithT(t).Expect(err).NotTo(HaveOccurred(), raw)
	}
	for _, raw := range []string{"http://fixture.invalid/drive/v3", "https://fixture@localhost", "https://localhost/?x=y", "https://localhost/#fragment", "https://%zz"} {
		_, err := drive.NewMaterialRetrieverWithTokenSource(config.DriveConfig{BaseURL: raw}, fixtureToken())
		NewWithT(t).Expect(err).To(MatchError(ContainSubstring("invalid GOOGLE_DRIVE_BASE_URL")), raw)
	}
}

func TestDriveDoesNotFollowProviderRedirect(t *testing.T) {
	g := NewWithT(t)
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Drive redirect target was contacted")
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer target.Close()
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer provider.Close()
	retriever, err := drive.NewMaterialRetrieverWithTokenSource(config.DriveConfig{BaseURL: provider.URL, MaterialDir: t.TempDir()}, fixtureToken())
	g.Expect(err).NotTo(HaveOccurred())
	got := retriever.Retrieve(context.Background(), submission.Material{ExternalID: "file-1", SourceURI: "https://drive.google.com/open?id=file-1&resourcekey=test-key"})
	g.Expect(got.Status).To(Equal(submission.Failed))
	g.Expect(got.LocalPath).To(BeEmpty())
}

func TestDriveDoesNotRequestContentWithInvalidReferenceContextOrToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("unexpected provider request")
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	for _, tc := range []struct {
		name, id string
		ctx      context.Context
		tokens   oauth2.TokenSource
	}{
		{"invalid file ID", "file/one", context.Background(), fixtureToken()},
		{"canceled context", "file-1", canceled, fixtureToken()},
		{"empty token", "file-1", context.Background(), oauth2.StaticTokenSource(&oauth2.Token{})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			retriever, err := drive.NewMaterialRetrieverWithTokenSource(config.DriveConfig{BaseURL: server.URL, MaterialDir: t.TempDir()}, tc.tokens)
			g.Expect(err).NotTo(HaveOccurred())
			got := retriever.Retrieve(tc.ctx, submission.Material{ExternalID: tc.id})
			g.Expect(got.Status).To(Equal(submission.Failed))
			g.Expect(got.LocalPath).To(BeEmpty())
		})
	}
}

func TestDriveDoesNotPublishOversizedOrUnwritableSnapshots(t *testing.T) {
	const body = "fixture payload"
	for _, tc := range []struct{ name, filename, extension string }{
		{"unsafe provider extension falls back to binary", "fixture.longextension", ".bin"},
		{"unwritable directory component", "fixture.TXT", ".txt"},
		{"conflicting digest path is not overwritten", "fixture.TXT", ".txt"},
		{"oversized content length", "fixture.TXT", ".txt"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Query().Has("fields") {
					fmt.Fprintf(w, `{"id":"file-1","name":%q,"mimeType":"application/octet-stream"}`, tc.filename)
					return
				}
				if tc.name == "oversized content length" {
					w.Header().Set("Content-Length", "104857601")
					w.WriteHeader(http.StatusOK)
					return
				}
				fmt.Fprint(w, body)
			}))
			defer server.Close()
			dir := t.TempDir()
			digest := fmt.Sprintf("%x%s", sha256.Sum256([]byte(body)), tc.extension)
			if tc.name == "conflicting digest path is not overwritten" {
				g.Expect(os.WriteFile(filepath.Join(dir, digest), []byte("old"), 0600)).To(Succeed())
			}
			if tc.name == "unwritable directory component" {
				blocker := filepath.Join(dir, "not-a-directory")
				g.Expect(os.WriteFile(blocker, []byte("old"), 0600)).To(Succeed())
				dir = filepath.Join(blocker, "snapshots")
			}
			retriever, err := drive.NewMaterialRetrieverWithTokenSource(config.DriveConfig{BaseURL: server.URL, MaterialDir: dir}, fixtureToken())
			g.Expect(err).NotTo(HaveOccurred())
			got := retriever.Retrieve(context.Background(), submission.Material{ExternalID: "file-1"})
			if tc.extension == ".bin" {
				g.Expect(got.Status).To(Equal(submission.Pending))
				g.Expect(got.LocalPath).To(Equal(filepath.Join(dir, digest)))
				return
			}
			g.Expect(got.Status).To(Equal(submission.Failed))
			g.Expect(got.LocalPath).To(BeEmpty())
			switch tc.name {
			case "unwritable directory component":
				contents, err := os.ReadFile(filepath.Dir(dir))
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(string(contents)).To(Equal("old"))
			case "conflicting digest path is not overwritten":
				entries, err := os.ReadDir(dir)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(entries).To(HaveLen(1))
				contents, err := os.ReadFile(filepath.Join(dir, digest))
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(string(contents)).To(Equal("old"))
			default:
				entries, err := os.ReadDir(dir)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(entries).To(BeEmpty())
			}
		})
	}
}
