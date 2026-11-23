package drive

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/alex/assistent/internal/domain/submission"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"golang.org/x/oauth2"
)

type failingRoundTripper struct{}

func (failingRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("transport failed")
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("read failed") }

var _ = Describe("Drive content helpers", func() {
	It("selects exports and safe extensions", func() {
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
			Expect(path).To(Equal(item.path))
			Expect(ext).To(Equal(item.ext))
		}
		path, query, ext := contentRequest(submission.Material{MIMEType: googleMIME + "form"})
		Expect(path).To(BeEmpty())
		Expect(query).To(BeNil())
		Expect(ext).To(BeEmpty())
	})

	It("publishes digest files and rejects invalid inputs", func() {
		dir := GinkgoT().TempDir()
		path, err := saveFile(context.Background(), dir, ".txt", strings.NewReader("content"))
		Expect(err).NotTo(HaveOccurred())
		Expect(filepath.Dir(path)).To(Equal(dir))
		contents, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(contents)).To(Equal("content"))
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err = saveFile(ctx, dir, ".txt", bytes.NewReader([]byte("content")))
		Expect(err).To(HaveOccurred())
		_, err = saveFile(context.Background(), dir, ".txt", io.MultiReader(strings.NewReader(strings.Repeat("x", maxFileBytes)), strings.NewReader("x")))
		Expect(err).To(HaveOccurred())
		_, err = saveFile(context.Background(), dir, ".txt", failingReader{})
		Expect(err).To(HaveOccurred())
		digest := sha256.Sum256([]byte("directory"))
		Expect(os.Mkdir(filepath.Join(dir, fmt.Sprintf("%x.txt", digest)), 0700)).To(Succeed())
		_, err = saveFile(context.Background(), dir, ".txt", strings.NewReader("directory"))
		Expect(err).To(HaveOccurred())
	})

	It("reports request and transport failures", func() {
		retriever := &materialRetriever{baseURL: "://bad", client: http.DefaultClient, tokens: oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "token"})}
		_, status := retriever.get(context.Background(), "file", nil, "")
		Expect(status).To(Equal(submission.Failed))
		retriever.baseURL = "http://example.test"
		retriever.client = &http.Client{Transport: failingRoundTripper{}}
		_, status = retriever.get(context.Background(), "file", nil, "")
		Expect(status).To(Equal(submission.Failed))
	})
})
