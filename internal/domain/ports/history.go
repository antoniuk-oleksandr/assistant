package ports

import (
	"context"
	"github.com/alex/assistent/internal/domain/history"
)

type HistoryRepository interface {
	ListHistory(context.Context) ([]history.Entry, error)
}
