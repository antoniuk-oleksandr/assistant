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

	It("loads Drive OAuth settings and defaults snapshots next to the SQLite database", func() {
		for _, key := range []string{"GOOGLE_DRIVE_BASE_URL", "GOOGLE_DRIVE_MATERIAL_DIR", "GOOGLE_DRIVE_OAUTH_CLIENT_ID", "GOOGLE_DRIVE_OAUTH_CLIENT_SECRET", "GOOGLE_DRIVE_OAUTH_REFRESH_TOKEN"} {
			GinkgoT().Setenv(key, "")
			Expect(os.Unsetenv(key)).To(Succeed())
		}
		GinkgoT().Setenv("GOOGLE_DRIVE_OAUTH_CLIENT_ID", "process-client")
		Expect(os.WriteFile(".env", []byte(`SQLITE_DATABASE_PATH=test-state/state.db
GOOGLE_DRIVE_BASE_URL=https://drive.example.invalid/v3
GOOGLE_DRIVE_OAUTH_CLIENT_ID=file-client
GOOGLE_DRIVE_OAUTH_CLIENT_SECRET=fixture-secret
GOOGLE_DRIVE_OAUTH_REFRESH_TOKEN=fixture-refresh
`), 0600)).To(Succeed())
		loaded, err := config.Load()
		Expect(err).NotTo(HaveOccurred())
		want := config.DriveConfig{BaseURL: "https://drive.example.invalid/v3", MaterialDir: filepath.Join("test-state", "materials"),
			ClientID: "process-client", ClientSecret: "fixture-secret", RefreshToken: "fixture-refresh"}
		Expect(config.Drive(loaded)).To(Equal(want))
		GinkgoT().Setenv("GOOGLE_DRIVE_MATERIAL_DIR", "custom-materials")
		loaded, err = config.Load()
		Expect(err).NotTo(HaveOccurred())
		want.MaterialDir = "custom-materials"
		Expect(config.Drive(loaded)).To(Equal(want))
	})

	It("rejects an empty configured path", func() {
		Expect(os.Setenv(config.SQLitePathEnv, "  ")).To(Succeed())
		_, err := config.Load()
		Expect(err).To(MatchError("SQLITE_DATABASE_PATH must be a non-empty file path"))
	})

	It("rejects an invalid dotenv file", func() {
		Expect(os.WriteFile(".env", []byte("INVALID=\"unterminated\n"), 0600)).To(Succeed())
		_, err := config.Load()
		Expect(err).To(MatchError("could not load .env"))
	})
})
