package drive

import (
	"context"
	"fmt"
	. "github.com/onsi/ginkgo/v2"
	"net/http"
	"net/http/httptest"
	"sync/atomic"

	"github.com/alex/assistent/internal/domain/submission"
	"github.com/alex/assistent/internal/platform/config"
	. "github.com/onsi/gomega"
)

type oauthGinkgoTest struct{}

func (*oauthGinkgoTest) TempDir() string          { return GinkgoT().TempDir() }
func (*oauthGinkgoTest) Setenv(key, value string) { GinkgoT().Setenv(key, value) }
func (*oauthGinkgoTest) Cleanup(fn func())        { DeferCleanup(fn) }
func (*oauthGinkgoTest) Helper()                  {}
func (*oauthGinkgoTest) Fatal(args ...any)        { Expect(false).To(BeTrue(), fmt.Sprint(args...)) }
func (*oauthGinkgoTest) Fatalf(format string, args ...any) {
	Expect(false).To(BeTrue(), fmt.Sprintf(format, args...))
}
func (*oauthGinkgoTest) Errorf(format string, args ...any) {
	Expect(false).To(BeTrue(), fmt.Sprintf(format, args...))
}
func (*oauthGinkgoTest) Error(args ...any) { Expect(false).To(BeTrue(), fmt.Sprint(args...)) }
func (*oauthGinkgoTest) Run(name string, fn func(*oauthGinkgoTest)) bool {
	By(name)
	fn(&oauthGinkgoTest{})
	return true
}

func testDriveRefreshesExpiredAccessTokenAndUsesRotatedRefreshToken(t *oauthGinkgoTest) {
	g := NewWithT(t)
	var exchanges atomic.Int32
	var apiCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			if err := r.ParseForm(); err != nil {
				t.Errorf("parse token request: %v", err)
			}
			if r.Method != http.MethodPost || r.PostForm.Get("grant_type") != "refresh_token" ||
				r.PostForm.Get("client_id") != "fixture-client" || r.PostForm.Get("client_secret") != "fixture-secret" {
				t.Error("unexpected OAuth refresh request")
			}
			w.Header().Set("Content-Type", "application/json")
			switch exchanges.Add(1) {
			case 1:
				if r.PostForm.Get("refresh_token") != "initial-refresh" {
					t.Error("initial refresh token not sent")
				}
				// Force expiration without sleeping. The next token request must refresh.
				fmt.Fprintf(w, `{"access_token":"expired-access","token_type":"Bearer","expires_in":-1,"refresh_token":"rotated-refresh","scope":%q}`, readonlyScope)
			case 2:
				if r.PostForm.Get("refresh_token") != "rotated-refresh" {
					t.Error("rotated refresh token not sent")
				}
				fmt.Fprintf(w, `{"access_token":"fresh-access","token_type":"Bearer","expires_in":3600,"scope":%q}`, readonlyScope)
			default:
				t.Error("valid access token was not reused")
				w.WriteHeader(http.StatusBadRequest)
			}
		case "/files/file-1":
			apiCalls.Add(1)
			if r.Header.Get("Authorization") != "Bearer fresh-access" {
				t.Error("Drive request did not use refreshed access token")
			}
			if r.URL.Query().Has("fields") {
				fmt.Fprint(w, `{"id":"file-1","mimeType":"application/pdf"}`)
			} else {
				fmt.Fprint(w, "fixture PDF")
			}
		default:
			t.Errorf("unexpected request: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	g.Expect(readonlyScope).To(Equal("https://www.googleapis.com/auth/drive.readonly"))
	cfg := config.DriveConfig{BaseURL: server.URL, MaterialDir: t.TempDir(), ClientID: "fixture-client", ClientSecret: "fixture-secret", RefreshToken: "initial-refresh"}
	tokens, err := newTokenSource(cfg, server.URL+"/token", server.Client())
	g.Expect(err).NotTo(HaveOccurred())
	first, err := tokens.Token()
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(first.AccessToken).To(Equal("expired-access"))
	g.Expect(first.Valid()).To(BeFalse())
	retriever, err := NewMaterialRetrieverWithTokenSource(cfg, tokens)
	g.Expect(err).NotTo(HaveOccurred())
	got := retriever.Retrieve(context.Background(), submission.Material{ExternalID: "file-1"})
	g.Expect(got.Status).To(Equal(submission.Pending))
	g.Expect(got.LocalPath).To(BeAnExistingFile())
	g.Expect(exchanges.Load()).To(Equal(int32(2)))
	g.Expect(apiCalls.Load()).To(Equal(int32(2)))
}

func testDriveWithoutCredentialsDoesNotContactProvider(t *oauthGinkgoTest) {
	g := NewWithT(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Drive was contacted without credentials")
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	cfg := config.DriveConfig{BaseURL: server.URL, MaterialDir: t.TempDir()}
	retriever, err := NewMaterialRetriever(cfg)
	g.Expect(err).NotTo(HaveOccurred())
	got := retriever.Retrieve(context.Background(), submission.Material{ExternalID: "file-1"})
	g.Expect(got.Status).To(Equal(submission.AccessDenied))
	g.Expect(got.LocalPath).To(BeEmpty())
	cfg.ClientID = "fixture-client"
	_, err = NewMaterialRetriever(cfg)
	g.Expect(err).To(MatchError(ContainSubstring("Drive OAuth requires")))
}

var _ = Describe("oauth legacy tests", func() {
	It("runs the converted standard-library cases", func() {
		t := &oauthGinkgoTest{}
		testDriveRefreshesExpiredAccessTokenAndUsesRotatedRefreshToken(t)
		testDriveWithoutCredentialsDoesNotContactProvider(t)
	})
})
