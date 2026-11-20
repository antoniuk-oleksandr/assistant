package ports

import (
	"context"
	"errors"
	"time"

	"github.com/alex/assistent/internal/domain/report"
	"github.com/alex/assistent/internal/domain/scenario"
)

var ErrReportRunNotFound = errors.New("report run not found")

type ReportRunRepository interface {
	Create(context.Context, *report.Run) error
	Update(context.Context, *report.Run) error
	Get(context.Context, int64) (report.Run, error)
}

// ScenarioTaskReportRunRepository is the optional lookup capability used to
// make repeated polling idempotent.
type ScenarioTaskReportRunRepository interface {
	ReportRunRepository
	GetByScenarioTask(context.Context, int64, int64) (report.Run, error)
}

type ReportOutput struct {
	ReportRunID int64
	Format      string
	Path        string
	Version     int
	CreatedAt   time.Time
}

type ReportOutputRepository interface {
	Create(context.Context, ReportOutput) error
}

type ReportRenderer interface {
	RenderAndSave(context.Context, scenario.ReportData) (report.Output, error)
}
