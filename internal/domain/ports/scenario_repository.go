package ports

import (
	"context"

	"github.com/alex/assistent/internal/domain/scenario"
)

type ScenarioRepository interface {
	// Create registers an implementation as active. Repeating it leaves an existing registration unchanged.
	Create(ctx context.Context, implementation scenario.Implementation) error
	List(ctx context.Context) ([]scenario.Scenario, error)
	ListActive(ctx context.Context) ([]scenario.Scenario, error)
	// SetStatus and Remove return scenario.ErrNotFound for an unregistered name.
	SetStatus(ctx context.Context, name string, status scenario.Status) error
	Remove(ctx context.Context, name string) error
}
