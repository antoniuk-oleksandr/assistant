package application

import (
	"context"
	"time"

	"github.com/alex/assistent/internal/domain/ports"
	"github.com/alex/assistent/internal/domain/scenario"
)

// ScenarioResult is the administrative view of a registered scenario.
type ScenarioResult struct {
	Name               string
	ImplementationType string
	Status             string
	CreatedAt          time.Time
	UpdatedAt          time.Time
	LastRunAt          *time.Time
}

type ScenarioService struct {
	repository ports.ScenarioRepository
	registry   scenario.Registry
}

func NewScenarioService(repository ports.ScenarioRepository, registry scenario.Registry) *ScenarioService {
	return &ScenarioService{repository: repository, registry: registry}
}

func (s *ScenarioService) Add(ctx context.Context, name string) error {
	implementation, err := s.registry.Resolve(name)
	if err != nil {
		return err
	}
	return s.repository.Create(ctx, implementation)
}

func (s *ScenarioService) List(ctx context.Context) ([]ScenarioResult, error) {
	registered, err := s.repository.List(ctx)
	if err != nil {
		return nil, err
	}
	results := make([]ScenarioResult, 0, len(registered))
	for _, item := range registered {
		results = append(results, ScenarioResult{
			Name:               item.Implementation.Name,
			ImplementationType: item.Implementation.Type,
			Status:             string(item.Status),
			CreatedAt:          item.CreatedAt,
			UpdatedAt:          item.UpdatedAt,
			LastRunAt:          item.LastRunAt,
		})
	}
	return results, nil
}

func (s *ScenarioService) Enable(ctx context.Context, name string) error {
	if _, err := s.registry.Resolve(name); err != nil {
		return err
	}
	return s.repository.SetStatus(ctx, name, scenario.Active)
}

func (s *ScenarioService) Disable(ctx context.Context, name string) error {
	if _, err := s.registry.Resolve(name); err != nil {
		return err
	}
	return s.repository.SetStatus(ctx, name, scenario.Disabled)
}

func (s *ScenarioService) Remove(ctx context.Context, name string) error {
	if _, err := s.registry.Resolve(name); err != nil {
		return err
	}
	return s.repository.Remove(ctx, name)
}
