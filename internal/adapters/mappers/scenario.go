package mappers

import (
	"fmt"
	"time"

	"github.com/alex/assistent/internal/adapters/models"
	"github.com/alex/assistent/internal/domain/scenario"
)

func ToScenario(row models.ScenarioRow) (scenario.Scenario, error) {
	created, err := time.Parse(time.RFC3339Nano, row.CreatedAt)
	if err != nil {
		return scenario.Scenario{}, fmt.Errorf("parse scenario created_at: %w", err)
	}
	updated, err := time.Parse(time.RFC3339Nano, row.UpdatedAt)
	if err != nil {
		return scenario.Scenario{}, fmt.Errorf("parse scenario updated_at: %w", err)
	}
	result := scenario.Scenario{
		ID:             row.ID,
		Implementation: scenario.Implementation{Name: row.Name, Type: row.ImplementationType},
		Status:         scenario.Status(row.Status),
		CreatedAt:      created,
		UpdatedAt:      updated,
	}
	if row.LastRunAt.Valid {
		lastRun, err := time.Parse(time.RFC3339Nano, row.LastRunAt.String)
		if err != nil {
			return scenario.Scenario{}, fmt.Errorf("parse scenario last_run_at: %w", err)
		}
		result.LastRunAt = &lastRun
	}
	return result, nil
}
