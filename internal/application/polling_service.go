package application

import (
	"context"
	"fmt"
	"time"

	"github.com/alex/assistent/internal/domain/classroom"
	"github.com/alex/assistent/internal/domain/ports"
	"github.com/alex/assistent/internal/domain/scenario"
	"github.com/alex/assistent/internal/domain/submission"
	"github.com/alex/assistent/internal/domain/task"
)

const DefaultPollInterval = 5 * time.Minute

// PollingService refreshes registered courses and persists provider state. The
// provider is deliberately a port so Classroom HTTP details stay outside the
// application layer.
type PollingService struct {
	courses      ports.ClassroomCourseRepository
	source       ports.SubmissionSource
	tasks        ports.TaskRepository
	bindings     ports.TaskBindingRepository
	submissions  ports.SubmissionRepository
	state        ports.PollingStateRepository
	materials    ports.MaterialRetriever
	parsing      *MaterialParsing
	collection   *ScenarioCollectionService
	registry     scenario.Registry
	participants ports.ParticipantRosterRepository
	clock        func() time.Time
}

func NewPollingService(courses ports.ClassroomCourseRepository, source ports.SubmissionSource, tasks ports.TaskRepository, bindings ports.TaskBindingRepository, submissions ports.SubmissionRepository) *PollingService {
	return NewPersistentPollingService(courses, source, tasks, bindings, submissions, noopPollingState{})
}

func NewPersistentPollingService(courses ports.ClassroomCourseRepository, source ports.SubmissionSource, tasks ports.TaskRepository, bindings ports.TaskBindingRepository, submissions ports.SubmissionRepository, state ports.PollingStateRepository) *PollingService {
	return NewPollingServiceWithMaterials(courses, source, tasks, bindings, submissions, state, nil)
}

func NewPollingServiceWithMaterials(courses ports.ClassroomCourseRepository, source ports.SubmissionSource, tasks ports.TaskRepository, bindings ports.TaskBindingRepository, submissions ports.SubmissionRepository, state ports.PollingStateRepository, materials ports.MaterialRetriever) *PollingService {
	return NewPollingServiceWithParsing(courses, source, tasks, bindings, submissions, state, materials, nil)
}

func NewPollingServiceWithParsing(courses ports.ClassroomCourseRepository, source ports.SubmissionSource, tasks ports.TaskRepository, bindings ports.TaskBindingRepository, submissions ports.SubmissionRepository, state ports.PollingStateRepository, materials ports.MaterialRetriever, parsing *MaterialParsing) *PollingService {
	return NewPollingServiceWithScenarioCollection(courses, source, tasks, bindings, submissions, state, materials, parsing, nil, scenario.Registry{})
}

func NewPollingServiceWithScenarioCollection(courses ports.ClassroomCourseRepository, source ports.SubmissionSource, tasks ports.TaskRepository, bindings ports.TaskBindingRepository, submissions ports.SubmissionRepository, state ports.PollingStateRepository, materials ports.MaterialRetriever, parsing *MaterialParsing, collection *ScenarioCollectionService, registry scenario.Registry) *PollingService {
	return newPollingServiceWithScenarioCollection(courses, source, tasks, bindings, submissions, state, materials, parsing, collection, registry, nil)
}

func NewPollingServiceWithScenarioCollectionAndRoster(courses ports.ClassroomCourseRepository, source ports.SubmissionSource, tasks ports.TaskRepository, bindings ports.TaskBindingRepository, submissions ports.SubmissionRepository, state ports.PollingStateRepository, materials ports.MaterialRetriever, parsing *MaterialParsing, collection *ScenarioCollectionService, registry scenario.Registry, participants ports.ParticipantRosterRepository) *PollingService {
	return newPollingServiceWithScenarioCollection(courses, source, tasks, bindings, submissions, state, materials, parsing, collection, registry, participants)
}

func newPollingServiceWithScenarioCollection(courses ports.ClassroomCourseRepository, source ports.SubmissionSource, tasks ports.TaskRepository, bindings ports.TaskBindingRepository, submissions ports.SubmissionRepository, state ports.PollingStateRepository, materials ports.MaterialRetriever, parsing *MaterialParsing, collection *ScenarioCollectionService, registry scenario.Registry, participants ports.ParticipantRosterRepository) *PollingService {
	return &PollingService{courses: courses, source: source, tasks: tasks, bindings: bindings, submissions: submissions, state: state, materials: materials, parsing: parsing, collection: collection, registry: registry, participants: participants, clock: func() time.Time { return time.Now().UTC() }}
}

func NewPollingServiceWithClock(courses ports.ClassroomCourseRepository, source ports.SubmissionSource, tasks ports.TaskRepository, bindings ports.TaskBindingRepository, submissions ports.SubmissionRepository, state ports.PollingStateRepository, clock func() time.Time) *PollingService {
	service := NewPersistentPollingService(courses, source, tasks, bindings, submissions, state)
	if clock != nil {
		service.clock = clock
	}
	return service
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
	checkpoint, err := s.state.LastCheckpoint(ctx)
	if err != nil {
		return err
	}
	if err := s.refreshAndCheckpoint(ctx, checkpoint); err != nil {
		return err
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			checkpoint, err = s.state.LastCheckpoint(ctx)
			if err != nil {
				return err
			}
			if err := s.refreshAndCheckpoint(ctx, checkpoint); err != nil {
				return err
			}
		}
	}
}

func (s *PollingService) refreshAndCheckpoint(ctx context.Context, checkpoint time.Time) error {
	if err := s.refreshSince(ctx, checkpoint); err != nil {
		return err
	}
	return s.state.SaveCheckpoint(ctx, time.Now().UTC())
}

func (s *PollingService) refreshSince(ctx context.Context, checkpoint time.Time) error {
	courses, err := s.courses.List(ctx)
	if err != nil {
		return fmt.Errorf("list courses for polling: %w", err)
	}
	for _, course := range courses {
		if course.Status != classroom.Active {
			continue
		}
		if err := s.refreshCourseSince(ctx, course.ExternalID, checkpoint); err != nil {
			return err
		}
	}
	return nil
}

type noopPollingState struct{}

func (noopPollingState) LastCheckpoint(context.Context) (time.Time, error) { return time.Time{}, nil }
func (noopPollingState) SaveCheckpoint(context.Context, time.Time) error   { return nil }

func (s *PollingService) refreshCourse(ctx context.Context, courseID string) error {
	return s.refreshCourseSince(ctx, courseID, time.Time{})
}

func (s *PollingService) refreshCourseSince(ctx context.Context, courseID string, checkpoint time.Time) error {
	var items []task.Task
	var err error
	if incremental, ok := s.source.(ports.IncrementalSubmissionSource); ok {
		items, err = incremental.ListTasksSince(ctx, courseID, checkpoint)
	} else {
		items, err = s.source.ListTasks(ctx, courseID)
	}
	if err != nil {
		return fmt.Errorf("list tasks for course %q: %w", courseID, err)
	}
	var existing []task.Task
	if s.tasks != nil {
		existing, err = s.tasks.List(ctx)
		if err != nil {
			return fmt.Errorf("list persisted tasks: %w", err)
		}
	}
	now := time.Now().UTC()
	if s.clock != nil {
		now = s.clock()
	}
	for _, item := range items {
		if s.tasks == nil {
			continue
		}
		previous := findTask(existing, item.Provider, item.CourseID, item.ExternalID)
		item.Phase = task.DeterminePhase(item, now)
		if previous.Phase == item.Phase {
			item.PhaseAt = previous.PhaseAt
		} else {
			item.PhaseAt = now.UTC().Format(time.RFC3339Nano)
		}
		if err := s.tasks.Register(ctx, item); err != nil {
			return fmt.Errorf("store task %q: %w", item.ExternalID, err)
		}
	}
	if s.participants != nil {
		participantSource, ok := s.source.(ports.ParticipantSource)
		if ok {
			persisted, listErr := s.tasks.List(ctx)
			if listErr != nil {
				return fmt.Errorf("list tasks for participant roster: %w", listErr)
			}
			for _, item := range items {
				stored := findTask(persisted, item.Provider, item.CourseID, item.ExternalID)
				roster, listErr := participantSource.ListParticipants(ctx, item.CourseID)
				if listErr != nil {
					return fmt.Errorf("list participants for task %q: %w", item.ExternalID, listErr)
				}
				if err := s.participants.Replace(ctx, stored.ID, roster); err != nil {
					return fmt.Errorf("store participants for task %q: %w", item.ExternalID, err)
				}
			}
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
		if err := s.refreshTask(ctx, binding); err != nil {
			return err
		}
	}
	return nil
}

func findTask(items []task.Task, provider, courseID, externalID string) task.Task {
	for _, item := range items {
		if item.Provider == provider && item.CourseID == courseID && item.ExternalID == externalID {
			return item
		}
	}
	return task.Task{}
}

func (s *PollingService) refreshTask(ctx context.Context, binding task.Binding) error {
	item := binding.Task
	items, err := s.source.ListSubmissions(ctx, item.ExternalID)
	if err != nil {
		return fmt.Errorf("list submissions for task %q: %w", item.ExternalID, err)
	}
	for _, current := range items {
		current.TaskID = item.ID
		current.TaskExternalID = item.ExternalID
		if err := s.enrichParticipant(ctx, &current); err != nil {
			return err
		}
		if current.State == "" {
			current.State = submission.Unknown
		}
		if s.materials != nil && current.State == submission.Submitted {
			current.Materials, err = s.processMaterials(ctx, binding, current)
			if err != nil {
				return err
			}
		}
		if err := s.submissions.Upsert(ctx, current); err != nil {
			return fmt.Errorf("store submission %q: %w", current.ExternalID, err)
		}
	}
	if s.collection != nil {
		processor, err := s.registry.ResolveProcessor(binding.ScenarioName)
		if err != nil {
			return fmt.Errorf("resolve collection processor for scenario %q: %w", binding.ScenarioName, err)
		}
		if _, err := s.collection.ProcessForTask(ctx, binding.ScenarioID, item.ID, binding.ScenarioName, item.CourseID, processor); err != nil {
			return fmt.Errorf("process collection for task %q: %w", item.ExternalID, err)
		}
	}
	return nil
}

func (s *PollingService) enrichParticipant(ctx context.Context, current *submission.Submission) error {
	if s.participants == nil || current.TaskID == 0 || current.SubmitterID == "" {
		return nil
	}
	roster, err := s.participants.List(ctx, current.TaskID)
	if err != nil {
		return fmt.Errorf("list participants for submission %q: %w", current.ExternalID, err)
	}
	for _, participant := range roster {
		if participant.ExternalID == current.SubmitterID {
			if current.SubmitterName == "" {
				current.SubmitterName = participant.Name
			}
			if current.SubmitterEmail == "" {
				current.SubmitterEmail = participant.Email
			}
			break
		}
	}
	return nil
}
