package config

import (
	"errors"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

const SQLitePathEnv = "SQLITE_DATABASE_PATH"
const DefaultSQLitePath = "data/report-system.db"

type DatabaseConfig struct {
	SQLitePath string
}

type Config struct {
	Database DatabaseConfig
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
	return Config{Database: DatabaseConfig{SQLitePath: path}}, nil
}

func Database(cfg Config) DatabaseConfig {
	return cfg.Database
}
