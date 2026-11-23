package parser

import (
	"context"
	"os"
	"path/filepath"

	"github.com/alex/assistent/internal/domain/submission"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("local text signals", func() {
	It("reports cancellation after reading", func() {
		path := filepath.Join(GinkgoT().TempDir(), "text.txt")
		Expect(os.WriteFile(path, []byte("publication evidence"), 0o600)).To(Succeed())
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, _, err := localTextSignals(ctx, submission.Material{Name: "text.txt", MIMEType: "text/plain", LocalPath: path})
		Expect(err).To(MatchError(context.Canceled))
	})
	It("reports unexpected file errors", func() {
		_, _, err := localTextSignals(context.Background(), submission.Material{Name: "text.txt", MIMEType: "text/plain", LocalPath: "bad\x00path"})
		Expect(err).To(HaveOccurred())
		_, _, err = localTextSignals(context.Background(), submission.Material{Name: "text.txt", MIMEType: "text/plain", LocalPath: GinkgoT().TempDir()})
		Expect(err).To(HaveOccurred())
	})
})
