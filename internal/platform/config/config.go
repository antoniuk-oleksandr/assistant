package config

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

const SQLitePathEnv = "SQLITE_DATABASE_PATH"
const DefaultSQLitePath = "data/report-system.db"

type DatabaseConfig struct {
	SQLitePath string
}

type DriveConfig struct {
	BaseURL      string
	MaterialDir  string
	ClientID     string
	ClientSecret string
	RefreshToken string
}

type SheetsConfig struct {
	BaseURL      string
	ClientID     string
	ClientSecret string
	RefreshToken string
	Title        string
}

type ReportConfig struct {
	Renderer string
}

type EmailConfig struct {
	SMTPHost     string
	SMTPPort     int
	SMTPUsername string
	SMTPPassword string
	From         string
	TemplatePath string
}

type TelegramConfig struct {
	Token   string
	BaseURL string
}

type DeadlineConfig struct {
	PreliminaryBefore time.Duration
	RefreshBefore     time.Duration
}

type Config struct {
	Database DatabaseConfig
	Drive    DriveConfig
	Sheets   SheetsConfig
	Report   ReportConfig
	Email    EmailConfig
	Telegram TelegramConfig
	Deadline DeadlineConfig
}

func Load() (Config, error) {
	values, err := godotenv.Read()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		// A parse error may contain values from .env.
		return Config{}, errors.New("could not load .env")
	}
	path, present := os.LookupEnv(SQLitePathEnv)
	if !present {
		path, present = values[SQLitePathEnv]
	}
	if !present {
		path = DefaultSQLitePath
	}
	if strings.TrimSpace(path) == "" || strings.ContainsRune(path, 0) {
		return Config{}, errors.New("SQLITE_DATABASE_PATH must be a non-empty file path")
	}
	get := func(key string) string {
		if value, ok := os.LookupEnv(key); ok {
			return value
		}
		return values[key]
	}
	preliminary, err := parseFlexibleDuration(get("REPORT_PRELIMINARY_BEFORE"), 72*time.Hour)
	if err != nil {
		return Config{}, errors.New("REPORT_PRELIMINARY_BEFORE must be a duration such as 3d or 72h")
	}
	refresh, err := parseFlexibleDuration(get("REPORT_REFRESH_BEFORE"), 24*time.Hour)
	if err != nil || refresh > preliminary {
		return Config{}, errors.New("REPORT_REFRESH_BEFORE must be a duration no greater than REPORT_PRELIMINARY_BEFORE")
	}
	materialDir := get("GOOGLE_DRIVE_MATERIAL_DIR")
	if materialDir == "" {
		materialDir = filepath.Join(filepath.Dir(path), "materials")
	}
	sheetTitle := get("GOOGLE_SHEETS_REPORT_TITLE")
	if sheetTitle == "" {
		sheetTitle = "Scientific activity report"
	}
	reportRenderer := get("REPORT_RENDERER")
	if reportRenderer == "" {
		reportRenderer = "google-sheets"
	}
	templatePath := get("REPORT_READY_EMAIL_TEMPLATE")
	if templatePath == "" {
		templatePath = "templates/report-ready-email.txt"
	}
	smtpPort := 587
	if configuredPort := strings.TrimSpace(get("SMTP_PORT")); configuredPort != "" {
		parsedPort, parseErr := strconv.Atoi(configuredPort)
		if parseErr != nil || parsedPort < 1 || parsedPort > 65535 {
			return Config{}, errors.New("SMTP_PORT must be a valid TCP port")
		}
		smtpPort = parsedPort
	}
	return Config{
		Database: DatabaseConfig{SQLitePath: path},
		Drive: DriveConfig{
			BaseURL:      get("GOOGLE_DRIVE_BASE_URL"),
			MaterialDir:  materialDir,
			ClientID:     get("GOOGLE_DRIVE_OAUTH_CLIENT_ID"),
			ClientSecret: get("GOOGLE_DRIVE_OAUTH_CLIENT_SECRET"),
			RefreshToken: get("GOOGLE_DRIVE_OAUTH_REFRESH_TOKEN"),
		},
		Sheets: SheetsConfig{
			BaseURL: get("GOOGLE_SHEETS_BASE_URL"), ClientID: get("GOOGLE_SHEETS_OAUTH_CLIENT_ID"),
			ClientSecret: get("GOOGLE_SHEETS_OAUTH_CLIENT_SECRET"), RefreshToken: get("GOOGLE_SHEETS_OAUTH_REFRESH_TOKEN"), Title: sheetTitle,
		},
		Report:   ReportConfig{Renderer: reportRenderer},
		Email:    EmailConfig{SMTPHost: get("SMTP_HOST"), SMTPPort: smtpPort, SMTPUsername: get("SMTP_USERNAME"), SMTPPassword: get("SMTP_PASSWORD"), From: get("SMTP_FROM"), TemplatePath: templatePath},
		Telegram: TelegramConfig{Token: get("TELEGRAM_BOT_TOKEN"), BaseURL: get("TELEGRAM_BASE_URL")},
		Deadline: DeadlineConfig{PreliminaryBefore: preliminary, RefreshBefore: refresh},
	}, nil
}

func parseFlexibleDuration(value string, fallback time.Duration) (time.Duration, error) {
	if strings.TrimSpace(value) == "" {
		return fallback, nil
	}
	value = strings.TrimSpace(value)
	if strings.HasSuffix(value, "d") {
		days, err := strconv.ParseFloat(strings.TrimSuffix(value, "d"), 64)
		if err != nil || days <= 0 {
			return 0, errors.New("invalid days")
		}
		return time.Duration(days * float64(24*time.Hour)), nil
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		return 0, errors.New("invalid duration")
	}
	return parsed, nil
}

func Database(cfg Config) DatabaseConfig {
	return cfg.Database
}

func Drive(cfg Config) DriveConfig { return cfg.Drive }

func Sheets(cfg Config) SheetsConfig { return cfg.Sheets }

func Report(cfg Config) ReportConfig     { return cfg.Report }
func Email(cfg Config) EmailConfig       { return cfg.Email }
func Telegram(cfg Config) TelegramConfig { return cfg.Telegram }
func Deadline(cfg Config) DeadlineConfig { return cfg.Deadline }
