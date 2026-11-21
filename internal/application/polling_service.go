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

const DefaultPollInterval = 24 * time.Hour

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
	missing      ports.MissingSubmitterRepository
	reminders    *ReminderService
	head         *HeadNotificationService
	deadlines    task.DeadlineWindows
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
	return newPollingServiceWithScenarioCollection(courses, source, tasks, bindings, submissions, state, materials, parsing, collection, registry, nil, nil, nil, nil)
}

func NewPollingServiceWithScenarioCollectionAndRoster(courses ports.ClassroomCourseRepository, source ports.SubmissionSource, tasks ports.TaskRepository, bindings ports.TaskBindingRepository, submissions ports.SubmissionRepository, state ports.PollingStateRepository, materials ports.MaterialRetriever, parsing *MaterialParsing, collection *ScenarioCollectionService, registry scenario.Registry, participants ports.ParticipantRosterRepository) *PollingService {
	return newPollingServiceWithScenarioCollection(courses, source, tasks, bindings, submissions, state, materials, parsing, collection, registry, participants, nil, nil, nil)
}

func NewPollingServiceWithScenarioCollectionAndRosterAndMissing(courses ports.ClassroomCourseRepository, source ports.SubmissionSource, tasks ports.TaskRepository, bindings ports.TaskBindingRepository, submissions ports.SubmissionRepository, state ports.PollingStateRepository, materials ports.MaterialRetriever, parsing *MaterialParsing, collection *ScenarioCollectionService, registry scenario.Registry, participants ports.ParticipantRosterRepository, missing ports.MissingSubmitterRepository) *PollingService {
	return newPollingServiceWithScenarioCollection(courses, source, tasks, bindings, submissions, state, materials, parsing, collection, registry, participants, missing, nil, nil)
}

func NewPollingServiceWithScenarioCollectionAndRosterAndMissingAndReminders(courses ports.ClassroomCourseRepository, source ports.SubmissionSource, tasks ports.TaskRepository, bindings ports.TaskBindingRepository, submissions ports.SubmissionRepository, state ports.PollingStateRepository, materials ports.MaterialRetriever, parsing *MaterialParsing, collection *ScenarioCollectionService, registry scenario.Registry, participants ports.ParticipantRosterRepository, missing ports.MissingSubmitterRepository, reminders *ReminderService, head *HeadNotificationService) *PollingService {
	return newPollingServiceWithScenarioCollection(courses, source, tasks, bindings, submissions, state, materials, parsing, collection, registry, participants, missing, reminders, head)
}

func newPollingServiceWithScenarioCollection(courses ports.ClassroomCourseRepository, source ports.SubmissionSource, tasks ports.TaskRepository, bindings ports.TaskBindingRepository, submissions ports.SubmissionRepository, state ports.PollingStateRepository, materials ports.MaterialRetriever, parsing *MaterialParsing, collection *ScenarioCollectionService, registry scenario.Registry, participants ports.ParticipantRosterRepository, missing ports.MissingSubmitterRepository, reminders *ReminderService, head *HeadNotificationService) *PollingService {
	return &PollingService{courses: courses, source: source, tasks: tasks, bindings: bindings, submissions: submissions, state: state, materials: materials, parsing: parsing, collection: collection, registry: registry, participants: participants, missing: missing, reminders: reminders, head: head, deadlines: task.DefaultDeadlineWindows, clock: func() time.Time { return time.Now().UTC() }}
}

func (s *PollingService) SetDeadlineWindows(deadlines task.DeadlineWindows) {
	if deadlines.PreliminaryBefore <= 0 {
		deadlines.PreliminaryBefore = task.DefaultDeadlineWindows.PreliminaryBefore
	}
	if deadlines.RefreshBefore <= 0 || deadlines.RefreshBefore > deadlines.PreliminaryBefore {
		deadlines.RefreshBefore = task.DefaultDeadlineWindows.RefreshBefore
	}
	s.deadlines = deadlines
}

// SetClock supplies a deterministic clock for deadline and polling tests.
func (s *PollingService) SetClock(clock func() time.Time) {
	if clock != nil {
		s.clock = clock
	}
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

// Run waits for the configured interval before each refresh and continues
// polling until ctx is canceled. This prevents application startup from
// triggering report generation outside the scheduled polling cadence.
func (s *PollingService) Run(ctx context.Context, interval time.Duration) error {
	if interval <= 0 {
		return fmt.Errorf("polling interval must be greater than zero: %s", interval)
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			checkpoint, err := s.state.LastCheckpoint(ctx)
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
		item.Phase = task.DeterminePhaseWithWindows(item, now, s.deadlines)
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
	for index := range bindings {
		if bindings[index].Task.CourseID != courseID {
			continue
		}
		current := bindings[index].Task
		if current.DueDate != "" || current.DueAt != "" {
			phase := task.DeterminePhaseWithWindows(current, now, s.deadlines)
			if current.Phase == "" && phase == task.PhaseDiscovery {
				bindings[index].Task = current
				continue
			}
			if current.Phase != phase {
				current.Phase = phase
				current.PhaseAt = now.UTC().Format(time.RFC3339Nano)
				if err := s.tasks.Register(ctx, current); err != nil {
					return fmt.Errorf("store deadline phase for task %q: %w", current.ExternalID, err)
				}
			}
		}
		bindings[index].Task = current
	}
	for _, binding := range bindings {
		if binding.Task.CourseID != courseID {
			continue
		}
		if !shouldProcessPreliminaryReport(binding.Task) {
			continue
		}
		if err := s.refreshTask(ctx, binding); err != nil {
			return err
		}
	}
	return nil
}

func shouldProcessPreliminaryReport(item task.Task) bool {
	if item.DueDate == "" && item.DueAt == "" {
		return true
	}
	return item.Phase == task.PhaseThreeDaysBefore || item.Phase == task.PhaseOneDayBefore
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
	if err := s.reconcileMissing(ctx, item.ID, items); err != nil {
		return err
	}
	if s.reminders != nil && item.Phase == task.PhaseThreeDaysBefore {
		if err := s.scheduleMissingReminders(ctx, item); err != nil {
			return err
		}
	}
	if s.collection != nil {
		processor, err := s.registry.ResolveProcessor(binding.ScenarioName)
		if err != nil {
			return fmt.Errorf("resolve collection processor for scenario %q: %w", binding.ScenarioName, err)
		}
		var processErr error
		if item.Phase == task.PhaseOneDayBefore {
			_, processErr = s.collection.ProcessForTaskUpdate(ctx, binding.ScenarioID, item.ID, binding.ScenarioName, item.CourseID, processor)
		} else {
			_, processErr = s.collection.ProcessForTask(ctx, binding.ScenarioID, item.ID, binding.ScenarioName, item.CourseID, processor)
		}
		if processErr != nil {
			return fmt.Errorf("process collection for task %q: %w", item.ExternalID, processErr)
		}
	}
	return nil
}

func (s *PollingService) scheduleMissingReminders(ctx context.Context, item task.Task) error {
	if s.missing == nil || item.DueAt == "" {
		return nil
	}
	missing, err := s.missing.List(ctx, item.ID)
	if err != nil {
		return fmt.Errorf("list missing submitters for reminders: %w", err)
	}
	targets := make([]ports.ReminderTarget, 0, len(missing))
	for _, person := range missing {
		if person.Email == "" {
			continue
		}
		targets = append(targets, ports.ReminderTarget{
			PersonID: person.ExternalUserID, Channel: "email", Destination: person.Email,
			TaskTitle: item.Title, ClassroomLink: fmt.Sprintf("https://classroom.google.com/c/%s/a/%s/details", item.CourseID, item.ExternalID),
		})
	}
	if _, err := s.reminders.Schedule(ctx, item, nil, targets, s.deadlines.PreliminaryBefore); err != nil {
		return fmt.Errorf("schedule missing submission reminders: %w", err)
	}
	if s.head != nil {
		missing, err := s.missing.List(ctx, item.ID)
		if err != nil {
			return fmt.Errorf("list missing submitters for head notification: %w", err)
		}
		if err := s.head.ScheduleWithLead(ctx, item, missing, s.deadlines.PreliminaryBefore); err != nil {
			return err
		}
	}
	return nil
}

func (s *PollingService) reconcileMissing(ctx context.Context, taskID int64, submissions []submission.Submission) error {
	if s.participants == nil || s.missing == nil || taskID == 0 {
		return nil
	}
	roster, err := s.participants.List(ctx, taskID)
	if err != nil {
		return fmt.Errorf("list expected participants: %w", err)
	}
	previous, err := s.missing.List(ctx, taskID)
	if err != nil {
		return fmt.Errorf("list missing submitters: %w", err)
	}
	states := make(map[string]string, len(previous))
	for _, item := range previous {
		states[item.ExternalUserID] = item.NotificationState
	}
	submitted := make(map[string]bool, len(submissions))
	for _, item := range submissions {
		if item.State == submission.Submitted {
			submitted[item.SubmitterID] = true
		}
	}
	missing := make([]ports.MissingSubmitter, 0, len(roster))
	for _, participant := range roster {
		if submitted[participant.ExternalID] {
			continue
		}
		state := states[participant.ExternalID]
		if state == "" {
			state = "pending"
		}
		missing = append(missing, ports.MissingSubmitter{TaskID: taskID, ExternalUserID: participant.ExternalID, Name: participant.Name, Email: participant.Email, NotificationState: state})
	}
	if err := s.missing.Reconcile(ctx, taskID, missing); err != nil {
		return fmt.Errorf("reconcile missing submitters: %w", err)
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
