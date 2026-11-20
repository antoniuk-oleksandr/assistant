package mappers

import (
	"time"

	"github.com/alex/assistent/internal/application"
	"github.com/alex/assistent/internal/presentation/dto"
)

func ScenarioToDTO(item application.ScenarioResult) dto.Scenario {
	lastRun := "-"
	if item.LastRunAt != nil {
		lastRun = item.LastRunAt.UTC().Format(time.RFC3339Nano)
	}
	return dto.Scenario{
		Name:               item.Name,
		ImplementationType: item.ImplementationType,
		Status:             item.Status,
		CreatedAt:          item.CreatedAt.UTC().Format(time.RFC3339Nano),
		UpdatedAt:          item.UpdatedAt.UTC().Format(time.RFC3339Nano),
		LastRunAt:          lastRun,
	}
}
