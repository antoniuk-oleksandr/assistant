package ports

import (
	"context"
	"errors"

	"github.com/alex/assistent/internal/domain/report"
)

var ErrReportResultNotFound = errors.New("report result not found")

type ReportResultRepository interface {
	Create(context.Context, *report.Result) error
	Update(context.Context, *report.Result) error
	GetLatestByRun(context.Context, int64) (report.Result, error)
}
