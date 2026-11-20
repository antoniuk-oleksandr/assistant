package renderer

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/alex/assistent/internal/domain/report"
	"github.com/alex/assistent/internal/domain/scenario"
)

type LocalReportRenderer struct {
	directory string
}

func NewLocalReportRenderer() *LocalReportRenderer {
	return &LocalReportRenderer{directory: os.TempDir()}
}

func (r *LocalReportRenderer) RenderAndSave(_ context.Context, data scenario.ReportData) (report.Output, error) {
	content, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return report.Output{}, fmt.Errorf("marshal report package: %w", err)
	}
	if err := os.MkdirAll(r.directory, 0700); err != nil {
		return report.Output{}, fmt.Errorf("create report output directory: %w", err)
	}
	file, err := os.CreateTemp(r.directory, "science-report-*.json")
	if err != nil {
		return report.Output{}, fmt.Errorf("create report package: %w", err)
	}
	path := file.Name()
	if _, err := file.Write(content); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return report.Output{}, fmt.Errorf("write report package: %w", err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return report.Output{}, fmt.Errorf("close report package: %w", err)
	}
	return report.Output{Format: "local-package", Path: filepath.Clean(path), Version: 1, CreatedAt: time.Now().UTC()}, nil
}
