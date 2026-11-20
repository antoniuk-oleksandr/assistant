package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/alex/assistent/internal/platform/config"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestConfig(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Configuration")
}

var _ = Describe("configuration loading", func() {
	BeforeEach(func() {
		Expect(os.Unsetenv(config.SQLitePathEnv)).To(Succeed())
		Expect(os.Chdir(GinkgoT().TempDir())).To(Succeed())
	})

	It("uses the default database path", func() {
		loaded, err := config.Load()
		Expect(err).NotTo(HaveOccurred())
		Expect(loaded.Database.SQLitePath).To(Equal(config.DefaultSQLitePath))
		Expect(config.Database(loaded)).To(Equal(loaded.Database))
	})

	It("loads a path from .env and lets the process environment override it", func() {
		envPath := filepath.Join(GinkgoT().TempDir(), "from-env.db")
		Expect(os.WriteFile(".env", []byte(config.SQLitePathEnv+"=from-file.db\n"), 0600)).To(Succeed())
		loaded, err := config.Load()
		Expect(err).NotTo(HaveOccurred())
		Expect(loaded.Database.SQLitePath).To(Equal("from-file.db"))

		Expect(os.Setenv(config.SQLitePathEnv, envPath)).To(Succeed())
		loaded, err = config.Load()
		Expect(err).NotTo(HaveOccurred())
		Expect(loaded.Database.SQLitePath).To(Equal(envPath))
	})

	It("rejects an empty configured path", func() {
		Expect(os.Setenv(config.SQLitePathEnv, "  ")).To(Succeed())
		_, err := config.Load()
		Expect(err).To(MatchError("SQLITE_DATABASE_PATH must be a non-empty file path"))
	})
})
