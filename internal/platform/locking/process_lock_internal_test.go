package locking

import (
	"path/filepath"
	"testing"

	"github.com/alex/assistent/internal/platform/config"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestProcessLockInternal(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Process lock internal behavior")
}

var _ = Describe("Process lock", func() {
	It("reports release failure for a closed lock file", func() {
		lock := NewProcessLock(config.DatabaseConfig{SQLitePath: filepath.Join(GinkgoT().TempDir(), "state.db")})
		Expect(lock.Acquire()).To(Succeed())
		Expect(lock.file.Close()).To(Succeed())
		Expect(lock.Release()).To(HaveOccurred())
	})
})
