package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

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

type Config struct {
	Database DatabaseConfig
	Drive    DriveConfig
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
	materialDir := get("GOOGLE_DRIVE_MATERIAL_DIR")
	if materialDir == "" {
		materialDir = filepath.Join(filepath.Dir(path), "materials")
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
	}, nil
}

func Database(cfg Config) DatabaseConfig {
	return cfg.Database
}

func Drive(cfg Config) DriveConfig { return cfg.Drive }
