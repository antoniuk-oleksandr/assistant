package application

import (
	"context"
	"github.com/alex/assistent/internal/domain/classroom"
	"github.com/alex/assistent/internal/domain/ports"
	"time"
)

type ClassroomCourseResult struct {
	ExternalID      string
	SourceReference string
	Status          string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}
type ClassroomService struct {
	repository ports.ClassroomCourseRepository
}

type CourseTaskResult struct {
	ExternalID string
	CourseID   string
	Title      string
	Status     string
}

type CourseTaskService struct {
	tasks  ports.TaskRepository
	source ports.SubmissionSource
}

func NewCourseTaskService(tasks ports.TaskRepository, source ports.SubmissionSource) *CourseTaskService {
	return &CourseTaskService{tasks: tasks, source: source}
}

func (s *CourseTaskService) List(ctx context.Context, reference string) ([]CourseTaskResult, error) {
	courseID, err := classroom.NormalizeReference(reference)
	if err != nil {
		return nil, err
	}
	items, err := s.source.ListTasks(ctx, courseID)
	if err != nil {
		return nil, err
	}
	result := make([]CourseTaskResult, 0)
	for _, item := range items {
		if item.CourseID == "" {
			item.CourseID = courseID
		}
		if err := s.tasks.Register(ctx, item); err != nil {
			return nil, err
		}
		result = append(result, CourseTaskResult{ExternalID: item.ExternalID, CourseID: item.CourseID, Title: item.Title, Status: string(item.Status)})
	}
	return result, nil
}

func NewClassroomService(repository ports.ClassroomCourseRepository) *ClassroomService {
	return &ClassroomService{repository: repository}
}
func (s *ClassroomService) Add(ctx context.Context, reference string) error {
	id, err := classroom.NormalizeReference(reference)
	if err != nil {
		return err
	}
	return s.repository.Register(ctx, classroom.Course{ExternalID: id, SourceReference: reference})
}
func (s *ClassroomService) List(ctx context.Context) ([]ClassroomCourseResult, error) {
	courses, err := s.repository.List(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]ClassroomCourseResult, 0, len(courses))
	for _, course := range courses {
		result = append(result, ClassroomCourseResult{ExternalID: course.ExternalID, SourceReference: course.SourceReference, Status: string(course.Status), CreatedAt: course.CreatedAt, UpdatedAt: course.UpdatedAt})
	}
	return result, nil
}
func (s *ClassroomService) Remove(ctx context.Context, reference string) error {
	id, err := classroom.NormalizeReference(reference)
	if err != nil {
		return err
	}
	return s.repository.Disable(ctx, id)
}
