package logging

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/alex/assistent/internal/platform/config"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// New creates the application logger and rotates its output by calendar day.
// The logger is lifecycle-managed so buffered entries are flushed on shutdown.
func New(lifecycle fx.Lifecycle, cfg config.LoggingConfig) (*zap.Logger, error) {
	if strings.TrimSpace(cfg.Directory) == "" {
		return nil, fmt.Errorf("log directory must not be empty")
	}
	if err := os.MkdirAll(cfg.Directory, 0o700); err != nil {
		return nil, fmt.Errorf("create log directory: %w", err)
	}
	writer := &dailyWriter{directory: cfg.Directory}
	if err := writer.rotate(time.Now().UTC()); err != nil {
		return nil, err
	}
	level := zapcore.InfoLevel
	if err := level.UnmarshalText([]byte(strings.ToLower(cfg.Level))); err != nil {
		return nil, fmt.Errorf("parse log level: %w", err)
	}
	encoder := zap.NewProductionEncoderConfig()
	encoder.TimeKey = "timestamp"
	core := zapcore.NewCore(zapcore.NewJSONEncoder(encoder), zapcore.AddSync(writer), level)
	logger := zap.New(core)
	lifecycle.Append(fx.Hook{OnStop: func(_ context.Context) error {
		// Fx may emit its final lifecycle event after application OnStop hooks
		// have completed. Keep the file descriptor open until process exit so
		// that the Fx logger cannot write to a closed sink during shutdown.
		return logger.Sync()
	}})
	return logger, nil
}

type dailyWriter struct {
	mu        sync.Mutex
	directory string
	date      string
	file      *os.File
}

func (w *dailyWriter) rotate(now time.Time) error {
	date := now.UTC().Format("2006-01-02")
	if w.file != nil && w.date == date {
		return nil
	}
	path := filepath.Join(w.directory, date+".log")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open log file: %w", err)
	}
	if w.file != nil {
		_ = w.file.Close()
	}
	w.file, w.date = file, date
	return nil
}

func (w *dailyWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.rotate(time.Now().UTC()); err != nil {
		return 0, err
	}
	return w.file.Write(p)
}

func (w *dailyWriter) Sync() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return nil
	}
	return w.file.Sync()
}
