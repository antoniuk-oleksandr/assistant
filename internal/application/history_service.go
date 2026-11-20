package application

import (
	"context"
	"github.com/alex/assistent/internal/domain/history"
	"github.com/alex/assistent/internal/domain/ports"
)

type HistoryService struct{ repository ports.HistoryRepository }

func NewHistoryService(repository ports.HistoryRepository) *HistoryService {
	return &HistoryService{repository: repository}
}
func (s *HistoryService) List(ctx context.Context) ([]history.Entry, error) {
	return s.repository.ListHistory(ctx)
}
