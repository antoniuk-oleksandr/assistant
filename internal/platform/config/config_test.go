package config_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

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
		Expect(config.Report(loaded)).To(Equal(config.ReportConfig{Renderer: "google-sheets", TemplatePath: "templates/scientific-report/output_example.xlsx", ReminderPath: "templates/missing-submission-reminder.txt"}))
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
		for _, key := range []string{"GOOGLE_DRIVE_BASE_URL", "GOOGLE_DRIVE_MATERIAL_DIR", "GOOGLE_CLIENT_ID", "GOOGLE_CLIENT_SECRET", "GOOGLE_REFRESH_TOKEN"} {
			GinkgoT().Setenv(key, "")
			Expect(os.Unsetenv(key)).To(Succeed())
		}
		GinkgoT().Setenv("GOOGLE_CLIENT_ID", "process-client")
		Expect(os.WriteFile(".env", []byte(`SQLITE_DATABASE_PATH=test-state/state.db
GOOGLE_DRIVE_BASE_URL=https://drive.example.invalid/v3
GOOGLE_CLIENT_ID=file-client
GOOGLE_CLIENT_SECRET=fixture-secret
GOOGLE_REFRESH_TOKEN=fixture-refresh
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

	It("rejects a path containing a NUL byte", func() {
		Expect(os.WriteFile(".env", []byte(config.SQLitePathEnv+"=state\x00.db\n"), 0600)).To(Succeed())
		_, err := config.Load()
		Expect(err).To(MatchError("SQLITE_DATABASE_PATH must be a non-empty file path"))
	})

	It("loads Google Sheets report settings", func() {
		GinkgoT().Setenv("GOOGLE_SHEETS_BASE_URL", "https://sheets.example.invalid/v4")
		GinkgoT().Setenv("GOOGLE_CLIENT_ID", "client")
		GinkgoT().Setenv("GOOGLE_CLIENT_SECRET", "secret")
		GinkgoT().Setenv("GOOGLE_REFRESH_TOKEN", "refresh")
		GinkgoT().Setenv("GOOGLE_SHEETS_REPORT_TITLE", "Department report")
		GinkgoT().Setenv("REPORT_RENDERER", "google-sheets")
		loaded, err := config.Load()
		Expect(err).NotTo(HaveOccurred())
		Expect(config.Sheets(loaded)).To(Equal(config.SheetsConfig{BaseURL: "https://sheets.example.invalid/v4", ClientID: "client", ClientSecret: "secret", RefreshToken: "refresh", Title: "Department report"}))
		Expect(config.Report(loaded)).To(Equal(config.ReportConfig{Renderer: "google-sheets", TemplatePath: "templates/scientific-report/output_example.xlsx", ReminderPath: "templates/missing-submission-reminder.txt"}))
	})

	It("loads Telegram notification settings", func() {
		GinkgoT().Setenv("TELEGRAM_BOT_TOKEN", "bot-token")
		GinkgoT().Setenv("TELEGRAM_BOT_USERNAME", "Assistant4747Bot")
		GinkgoT().Setenv("TELEGRAM_BASE_URL", "https://telegram.example.invalid")
		GinkgoT().Setenv("TELEGRAM_CHAT_ID", "-1001234567890")
		loaded, err := config.Load()
		Expect(err).NotTo(HaveOccurred())
		Expect(config.Telegram(loaded)).To(Equal(config.TelegramConfig{Token: "bot-token", Username: "Assistant4747Bot", BaseURL: "https://telegram.example.invalid", ChatID: "-1001234567890"}))
	})

	It("loads Google SMTP notification settings", func() {
		GinkgoT().Setenv("SMTP_HOST", "smtp.gmail.com")
		GinkgoT().Setenv("SMTP_PORT", "587")
		GinkgoT().Setenv("SMTP_USERNAME", "sender@gmail.com")
		GinkgoT().Setenv("SMTP_PASSWORD", "app-password")
		GinkgoT().Setenv("SMTP_FROM", "reports@gmail.com")
		loaded, err := config.Load()
		Expect(err).NotTo(HaveOccurred())
		Expect(config.Email(loaded)).To(Equal(config.EmailConfig{SMTPHost: "smtp.gmail.com", SMTPPort: 587, SMTPUsername: "sender@gmail.com", SMTPPassword: "app-password", From: "reports@gmail.com", TemplatePath: "templates/report-ready-email.txt"}))
	})

	It("rejects invalid SMTP port values", func() {
		for _, value := range []string{"not-a-port", "0", "65536"} {
			GinkgoT().Setenv("SMTP_PORT", value)
			_, err := config.Load()
			Expect(err).To(MatchError("SMTP_PORT must be a valid TCP port"))
			GinkgoT().Setenv("SMTP_PORT", "")
		}
	})

	It("loads minute-scale deadline windows", func() {
		GinkgoT().Setenv("REPORT_PRELIMINARY_BEFORE", "5m")
		GinkgoT().Setenv("REPORT_REFRESH_BEFORE", "2m")
		loaded, err := config.Load()
		Expect(err).NotTo(HaveOccurred())
		Expect(config.Deadline(loaded)).To(Equal(config.DeadlineConfig{PreliminaryBefore: 5 * time.Minute, RefreshBefore: 2 * time.Minute}))
	})

	It("accepts fractional day windows", func() {
		GinkgoT().Setenv("REPORT_PRELIMINARY_BEFORE", "0.5d")
		GinkgoT().Setenv("REPORT_REFRESH_BEFORE", "2h")
		loaded, err := config.Load()
		Expect(err).NotTo(HaveOccurred())
		Expect(config.Deadline(loaded).PreliminaryBefore).To(Equal(12 * time.Hour))
	})

	It("rejects invalid or inverted deadline windows", func() {
		GinkgoT().Setenv("REPORT_PRELIMINARY_BEFORE", "nope")
		_, err := config.Load()
		Expect(err).To(MatchError("REPORT_PRELIMINARY_BEFORE must be a duration such as 3d or 72h"))
		GinkgoT().Setenv("REPORT_PRELIMINARY_BEFORE", "5m")
		GinkgoT().Setenv("REPORT_REFRESH_BEFORE", "6m")
		_, err = config.Load()
		Expect(err).To(MatchError("REPORT_REFRESH_BEFORE must be a duration no greater than REPORT_PRELIMINARY_BEFORE"))
		GinkgoT().Setenv("REPORT_PRELIMINARY_BEFORE", "-1d")
		GinkgoT().Setenv("REPORT_REFRESH_BEFORE", "1m")
		_, err = config.Load()
		Expect(err).To(MatchError("REPORT_PRELIMINARY_BEFORE must be a duration such as 3d or 72h"))
		GinkgoT().Setenv("REPORT_PRELIMINARY_BEFORE", "5m")
		GinkgoT().Setenv("REPORT_REFRESH_BEFORE", "-1s")
		_, err = config.Load()
		Expect(err).To(MatchError("REPORT_REFRESH_BEFORE must be a duration no greater than REPORT_PRELIMINARY_BEFORE"))
		GinkgoT().Setenv("REPORT_PRELIMINARY_BEFORE", "0d")
		GinkgoT().Setenv("REPORT_REFRESH_BEFORE", "1m")
		_, err = config.Load()
		Expect(err).To(MatchError("REPORT_PRELIMINARY_BEFORE must be a duration such as 3d or 72h"))
		GinkgoT().Setenv("REPORT_PRELIMINARY_BEFORE", "5m")
		GinkgoT().Setenv("REPORT_REFRESH_BEFORE", "0s")
		_, err = config.Load()
		Expect(err).To(MatchError("REPORT_REFRESH_BEFORE must be a duration no greater than REPORT_PRELIMINARY_BEFORE"))
	})

	It("rejects an invalid dotenv file", func() {
		Expect(os.WriteFile(".env", []byte("INVALID=\"unterminated\n"), 0600)).To(Succeed())
		_, err := config.Load()
		Expect(err).To(MatchError("could not load .env"))
	})
})
