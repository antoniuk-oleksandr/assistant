package drive

import (
	"errors"
	"io"
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type failingErrorReader struct{}

func (failingErrorReader) Read([]byte) (int, error) { return 0, errors.New("read failed") }

func TestProviderErrors(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Drive provider errors")
}

var _ = Describe("Drive quota error detection", func() {
	It("rejects unreadable and oversized bodies and accepts resource exhaustion", func() {
		Expect(isQuotaError(failingErrorReader{})).To(BeFalse())
		Expect(isQuotaError(strings.NewReader(strings.Repeat("x", maxErrorBody+1)))).To(BeFalse())
		Expect(isQuotaError(io.LimitReader(strings.NewReader(`{"error":{"status":"RESOURCE_EXHAUSTED"}}`), maxErrorBody+1))).To(BeTrue())
	})
})
