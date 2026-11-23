package application

import (
	"context"
	"errors"

	"github.com/alex/assistent/internal/domain/ports"
	"github.com/alex/assistent/internal/domain/scenario"
)

const ScientificReportName = "science-report"

// ReportService is the only public workflow service. The fixed scientific
// report implementation uses the existing lifecycle repositories internally,
// but callers never select or manage a scenario.
type ReportService struct {
	repository ports.ScenarioRepository
	registry   scenario.Registry
	bindings   ports.TaskBindingRepository
	courses    ports.ScenarioCourseBindingRepository
	data       ports.ScenarioCourseDataRepository
}

func NewReportService(repository ports.ScenarioRepository, registry scenario.Registry, bindings ports.TaskBindingRepository, courses ports.ScenarioCourseBindingRepository, data ports.ScenarioCourseDataRepository) *ReportService {
	return &ReportService{repository: repository, registry: registry, bindings: bindings, courses: courses, data: data}
}

func (s *ReportService) Configure(ctx context.Context, courseID string) error {
	if err := s.Add(ctx); err != nil {
		return err
	}
	if err := s.Enable(ctx); err != nil {
		return err
	}
	return s.BindCourse(ctx, courseID)
}

func (s *ReportService) Add(ctx context.Context) error {
	implementation, err := s.registry.Resolve(ScientificReportName)
	if err != nil {
		return err
	}
	return s.repository.Create(ctx, implementation)
}

func (s *ReportService) Enable(ctx context.Context) error {
	if _, err := s.registry.Resolve(ScientificReportName); err != nil {
		return err
	}
	return s.repository.SetStatus(ctx, ScientificReportName, scenario.Active)
}

func (s *ReportService) BindCourse(ctx context.Context, courseID string) error {
	if s.courses == nil {
		return errors.New("report course bindings are unavailable")
	}
	return s.courses.Bind(ctx, ScientificReportName, courseID)
}

func (s *ReportService) BindTask(ctx context.Context, taskID string) error {
	if s.bindings == nil {
		return errors.New("report task bindings are unavailable")
	}
	return s.bindings.Bind(ctx, ScientificReportName, taskID)
}

func (s *ReportService) UnbindTask(ctx context.Context, taskID string) error {
	if s.bindings == nil {
		return errors.New("report task bindings are unavailable")
	}
	return s.bindings.Unbind(ctx, taskID)
}

func (s *ReportService) ListTasks(ctx context.Context) ([]TaskBindingResult, error) {
	if s.bindings == nil {
		return nil, errors.New("report task bindings are unavailable")
	}
	items, err := s.bindings.List(ctx, ScientificReportName)
	if err != nil {
		return nil, err
	}
	result := make([]TaskBindingResult, 0, len(items))
	for _, item := range items {
		result = append(result, TaskBindingResult{TaskID: item.Task.ExternalID, CourseID: item.Task.CourseID, Title: item.Task.Title, Status: item.Status})
	}
	return result, nil
}

type TaskBindingResult struct {
	TaskID   string
	CourseID string
	Title    string
	Status   string
}

type CourseDataResult struct {
	Name  string
	Value string
}

func (s *ReportService) SetData(ctx context.Context, courseID string, fields map[string]string) error {
	if s.data == nil {
		return errors.New("report course data is unavailable")
	}
	return s.data.Set(ctx, ScientificReportName, courseID, fields)
}

func (s *ReportService) ListData(ctx context.Context, courseID string) ([]CourseDataResult, error) {
	if s.data == nil {
		return nil, errors.New("report course data is unavailable")
	}
	items, err := s.data.List(ctx, ScientificReportName, courseID)
	if err != nil {
		return nil, err
	}
	result := make([]CourseDataResult, 0, len(items))
	for _, item := range items {
		result = append(result, CourseDataResult{Name: item.Name, Value: item.Value})
	}
	return result, nil
}
