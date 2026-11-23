package locking_test

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/alex/assistent/internal/platform/config"
	"github.com/alex/assistent/internal/platform/locking"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Process lock", func() {
	It("rejects duplicates and allows acquire after release", func() {
		cfg := config.DatabaseConfig{SQLitePath: filepath.Join(GinkgoT().TempDir(), "state.db")}
		first := locking.NewProcessLock(cfg)
		second := locking.NewProcessLock(cfg)

		Expect(first.Acquire()).To(Succeed())
		Expect(errors.Is(second.Acquire(), locking.ErrApplicationAlreadyRunning)).To(BeTrue())
		Expect(first.Release()).To(Succeed())
		Expect(second.Acquire()).To(Succeed())
		Expect(second.Release()).To(Succeed())
	})

	It("treats release before acquire as a no-op", func() {
		lock := locking.NewProcessLock(config.DatabaseConfig{SQLitePath: filepath.Join(GinkgoT().TempDir(), "state.db")})
		Expect(lock.Release()).To(Succeed())
	})

	It("reports lock directory errors", func() {
		parent := filepath.Join(GinkgoT().TempDir(), "not-a-directory")
		Expect(os.WriteFile(parent, []byte("file"), 0600)).To(Succeed())
		lock := locking.NewProcessLock(config.DatabaseConfig{SQLitePath: filepath.Join(parent, "state.db")})
		Expect(lock.Acquire()).To(HaveOccurred())
	})

	It("reports lock file errors", func() {
		path := filepath.Join(GinkgoT().TempDir(), "state.db")
		Expect(os.Mkdir(path+".lock", 0700)).To(Succeed())
		lock := locking.NewProcessLock(config.DatabaseConfig{SQLitePath: path})
		Expect(lock.Acquire()).To(HaveOccurred())
	})
})
