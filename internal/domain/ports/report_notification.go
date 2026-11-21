package ports

import (
	"context"

	"github.com/alex/assistent/internal/domain/report"
)

// ReportReadyNotifier informs the configured account owner that a report was
// generated. The output link is passed separately because it is produced by
// the renderer after the structured result is persisted.
type ReportReadyNotifier interface {
	Enabled() bool
	Notify(context.Context, report.Result, string) error
}
