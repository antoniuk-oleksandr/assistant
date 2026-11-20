package application

import (
	"context"
	"fmt"
	"time"

	"github.com/alex/assistent/internal/domain/classroom"
	"github.com/alex/assistent/internal/domain/ports"
	"github.com/alex/assistent/internal/domain/submission"
	"github.com/alex/assistent/internal/domain/task"
)

const DefaultPollInterval = 5 * time.Minute

// PollingService refreshes registered courses and persists provider state. The
// provider is deliberately a port so Classroom HTTP details stay outside the
// application layer.
type PollingService struct {
	courses     ports.ClassroomCourseRepository
	source      ports.SubmissionSource
	tasks       ports.TaskRepository
	bindings    ports.TaskBindingRepository
	submissions ports.SubmissionRepository
}

func NewPollingService(courses ports.ClassroomCourseRepository, source ports.SubmissionSource, tasks ports.TaskRepository, bindings ports.TaskBindingRepository, submissions ports.SubmissionRepository) *PollingService {
	return &PollingService{courses: courses, source: source, tasks: tasks, bindings: bindings, submissions: submissions}
}

func (s *PollingService) Refresh(ctx context.Context) error {
	courses, err := s.courses.List(ctx)
	if err != nil {
		return fmt.Errorf("list courses for polling: %w", err)
	}
	for _, course := range courses {
		if course.Status != classroom.Active {
			continue
		}
		if err := s.refreshCourse(ctx, course.ExternalID); err != nil {
			return err
		}
	}
	return nil
}

// Run refreshes immediately and then continues polling until ctx is canceled.
// Checkpointing, restart recovery, and single-instance coordination are kept
// out of this lifecycle loop for the later recovery work.
func (s *PollingService) Run(ctx context.Context, interval time.Duration) error {
	if interval <= 0 {
		return fmt.Errorf("polling interval must be greater than zero: %s", interval)
	}
	if err := s.Refresh(ctx); err != nil {
		return err
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := s.Refresh(ctx); err != nil {
				return err
			}
		}
	}
}

func (s *PollingService) refreshCourse(ctx context.Context, courseID string) error {
	items, err := s.source.ListTasks(ctx, courseID)
	if err != nil {
		return fmt.Errorf("list tasks for course %q: %w", courseID, err)
	}
	for _, item := range items {
		if err := s.tasks.Register(ctx, item); err != nil {
			return fmt.Errorf("store task %q: %w", item.ExternalID, err)
		}
	}
	bindings, err := s.bindings.Active(ctx)
	if err != nil {
		return fmt.Errorf("list active task bindings: %w", err)
	}
	for _, binding := range bindings {
		if binding.Task.CourseID != courseID {
			continue
		}
		if err := s.refreshTask(ctx, binding.Task); err != nil {
			return err
		}
	}
	return nil
}

func (s *PollingService) refreshTask(ctx context.Context, item task.Task) error {
	items, err := s.source.ListSubmissions(ctx, item.ExternalID)
	if err != nil {
		return fmt.Errorf("list submissions for task %q: %w", item.ExternalID, err)
	}
	for _, current := range items {
		current.TaskID = item.ID
		current.TaskExternalID = item.ExternalID
		if current.State == "" {
			current.State = submission.Unknown
		}
		if err := s.submissions.Upsert(ctx, current); err != nil {
			return fmt.Errorf("store submission %q: %w", current.ExternalID, err)
		}
	}
	return nil
}
