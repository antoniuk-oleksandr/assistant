package ports

import (
	"context"

	"github.com/alex/assistent/internal/domain/report"
	"github.com/alex/assistent/internal/domain/task"
)

// ReportReadyNotifier informs active configured notification channels that a
// report was generated. The output link is passed separately because it is
// produced by the renderer after the structured result is persisted.
type ReportReadyNotifier interface {
	Enabled() bool
	Notify(context.Context, report.Result, string) error
}

type ReportReadyTelegramSender interface {
	SendReportReady(context.Context, string, string) error
}

type TelegramUpdate struct {
	ID       int64
	ChatID   string
	ChatType string
	Text     string
}

type TelegramUpdateSource interface {
	GetUpdates(context.Context, int64, int) ([]TelegramUpdate, error)
}

type TelegramPairingMessenger interface {
	SendMessage(context.Context, string, string) error
}

type PreliminaryReportNotifier interface {
	Notify(context.Context, task.Task, []MissingSubmitter) error
}
