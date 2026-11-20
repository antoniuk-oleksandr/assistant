package application

import (
	"context"
	"errors"
	"time"

	"github.com/alex/assistent/internal/domain/ports"
	"github.com/alex/assistent/internal/domain/scenario"
	"github.com/alex/assistent/internal/domain/task"
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
	bindings   ports.TaskBindingRepository
	courses    ports.ScenarioCourseBindingRepository
}

func NewScenarioService(repository ports.ScenarioRepository, registry scenario.Registry) *ScenarioService {
	return &ScenarioService{repository: repository, registry: registry}
}

func NewScenarioServiceWithBindings(repository ports.ScenarioRepository, registry scenario.Registry, bindings ports.TaskBindingRepository) *ScenarioService {
	return &ScenarioService{repository: repository, registry: registry, bindings: bindings}
}

func NewScenarioServiceWithCourseBindings(repository ports.ScenarioRepository, registry scenario.Registry, bindings ports.TaskBindingRepository, courses ports.ScenarioCourseBindingRepository) *ScenarioService {
	return &ScenarioService{repository: repository, registry: registry, bindings: bindings, courses: courses}
}

type TaskBindingResult struct {
	ScenarioName string
	ScenarioType string
	TaskID       string
	CourseID     string
	Title        string
	Status       string
}

type CourseBindingResult struct {
	ScenarioName string
	ScenarioType string
	CourseID     string
	Status       string
}

func (s *ScenarioService) BindCourse(ctx context.Context, scenarioName, courseID string) error {
	if s.courses == nil {
		return errors.New("scenario course bindings are unavailable")
	}
	return s.courses.Bind(ctx, scenarioName, courseID)
}

func (s *ScenarioService) UnbindCourse(ctx context.Context, scenarioName, courseID string) error {
	if s.courses == nil {
		return errors.New("scenario course bindings are unavailable")
	}
	return s.courses.Unbind(ctx, scenarioName, courseID)
}

func (s *ScenarioService) ListCourseBindings(ctx context.Context, scenarioName string) ([]CourseBindingResult, error) {
	if s.courses == nil {
		return nil, errors.New("scenario course bindings are unavailable")
	}
	items, err := s.courses.List(ctx, scenarioName)
	if err != nil {
		return nil, err
	}
	result := make([]CourseBindingResult, 0, len(items))
	for _, item := range items {
		result = append(result, CourseBindingResult{ScenarioName: item.ScenarioName, ScenarioType: item.ScenarioType, CourseID: item.CourseID, Status: item.Status})
	}
	return result, nil
}

func (s *ScenarioService) BindTask(ctx context.Context, scenarioName, taskID string) error {
	if s.bindings == nil {
		return errors.New("task bindings are unavailable")
	}
	return s.bindings.Bind(ctx, scenarioName, taskID)
}

func (s *ScenarioService) UnbindTask(ctx context.Context, taskID string) error {
	if s.bindings == nil {
		return errors.New("task bindings are unavailable")
	}
	return s.bindings.Unbind(ctx, taskID)
}

func (s *ScenarioService) ListTaskBindings(ctx context.Context, scenarioName string) ([]TaskBindingResult, error) {
	if s.bindings == nil {
		return nil, errors.New("task bindings are unavailable")
	}
	items, err := s.bindings.List(ctx, scenarioName)
	if err != nil {
		return nil, err
	}
	result := make([]TaskBindingResult, 0, len(items))
	for _, item := range items {
		result = append(result, TaskBindingResult{ScenarioName: item.ScenarioName, ScenarioType: item.ScenarioType, TaskID: item.Task.ExternalID, CourseID: item.Task.CourseID, Title: item.Task.Title, Status: item.Status})
	}
	return result, nil
}

func (s *ScenarioService) ActiveTaskBindings(ctx context.Context) ([]task.Binding, error) {
	if s.bindings == nil {
		return nil, errors.New("task bindings are unavailable")
	}
	return s.bindings.Active(ctx)
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
