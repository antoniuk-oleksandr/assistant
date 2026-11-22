package application

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/alex/assistent/internal/domain/ports"
	"github.com/alex/assistent/internal/domain/submission"
	"github.com/alex/assistent/internal/domain/task"
)

type schedulerBindings struct {
	unbound        string
	activeSequence [][]task.Binding
	activeErr      error
	unbindErr      error
}

func (s *schedulerBindings) Bind(context.Context, string, string) error { return nil }
func (s *schedulerBindings) Unbind(_ context.Context, taskID string) error {
	if s.unbindErr != nil {
		return s.unbindErr
	}
	s.unbound = taskID
	return nil
}
func (s *schedulerBindings) List(context.Context, string) ([]task.Binding, error) { return nil, nil }
func (s *schedulerBindings) Active(context.Context) ([]task.Binding, error) {
	if s.activeErr != nil {
		return nil, s.activeErr
	}
	if len(s.activeSequence) > 0 {
		items := s.activeSequence[0]
		s.activeSequence = s.activeSequence[1:]
		return items, nil
	}
	return nil, nil
}

var _ ports.TaskBindingRepository = (*schedulerBindings)(nil)

type schedulerParticipants struct {
	items  []task.Participant
	taskID int64
}

func (s *schedulerParticipants) Replace(_ context.Context, taskID int64, items []task.Participant) error {
	s.taskID, s.items = taskID, items
	return nil
}
func (s *schedulerParticipants) List(context.Context, int64) ([]task.Participant, error) {
	return s.items, nil
}

type schedulerParticipantSource struct{ items []task.Participant }

func (schedulerParticipantSource) ListTasks(context.Context, string) ([]task.Task, error) {
	return nil, nil
}
func (schedulerParticipantSource) ListSubmissions(context.Context, string) ([]submission.Submission, error) {
	return nil, nil
}
func (s schedulerParticipantSource) ListParticipants(context.Context, string) ([]task.Participant, error) {
	return s.items, nil
}

type schedulerRegistrarSource struct{ registered bool }

func (s *schedulerRegistrarSource) ListTasks(context.Context, string) ([]task.Task, error) {
	return nil, nil
}
func (s *schedulerRegistrarSource) ListSubmissions(context.Context, string) ([]submission.Submission, error) {
	return nil, nil
}
func (s *schedulerRegistrarSource) ListParticipants(context.Context, string) ([]task.Participant, error) {
	return nil, nil
}
func (s *schedulerRegistrarSource) RegisterTaskCourse(string, string) { s.registered = true }

type failingParticipantSource struct{}

func (failingParticipantSource) ListTasks(context.Context, string) ([]task.Task, error) {
	return nil, nil
}
func (failingParticipantSource) ListSubmissions(context.Context, string) ([]submission.Submission, error) {
	return nil, nil
}
func (failingParticipantSource) ListParticipants(context.Context, string) ([]task.Participant, error) {
	return nil, context.Canceled
}

type failingParticipantRepository struct{}

func (failingParticipantRepository) Replace(context.Context, int64, []task.Participant) error {
	return context.Canceled
}
func (failingParticipantRepository) List(context.Context, int64) ([]task.Participant, error) {
	return nil, nil
}

type schedulerSubmissionRepository struct {
	items []submission.Submission
	err   error
}

func (s *schedulerSubmissionRepository) Upsert(_ context.Context, item submission.Submission) error {
	if s.err != nil {
		return s.err
	}
	s.items = append(s.items, item)
	return nil
}
func (s *schedulerSubmissionRepository) ListByTask(context.Context, int64) ([]submission.Submission, error) {
	return s.items, nil
}

type schedulerTaskRepository struct {
	items []task.Task
	err   error
}

func (s *schedulerTaskRepository) Register(_ context.Context, item task.Task) error {
	if s.err != nil {
		return s.err
	}
	s.items = append(s.items, item)
	return nil
}
func (s *schedulerTaskRepository) List(context.Context) ([]task.Task, error) { return s.items, nil }

type failingSubmissionSource struct{ err error }

func (failingSubmissionSource) ListTasks(context.Context, string) ([]task.Task, error) {
	return nil, nil
}
func (f failingSubmissionSource) ListSubmissions(context.Context, string) ([]submission.Submission, error) {
	return nil, f.err
}

var _ ports.ParticipantRosterRepository = (*schedulerParticipants)(nil)
var _ ports.ParticipantSource = schedulerParticipantSource{}

type schedulerSubmissionOnlySource struct{}

func (schedulerSubmissionOnlySource) ListTasks(context.Context, string) ([]task.Task, error) {
	return nil, nil
}
func (schedulerSubmissionOnlySource) ListSubmissions(context.Context, string) ([]submission.Submission, error) {
	return nil, nil
}

func TestNextEventForTaskUsesExactDeadlineOffsets(t *testing.T) {
	deadline := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	windows := task.DeadlineWindows{PreliminaryBefore: 2 * time.Minute, RefreshBefore: time.Minute}
	tests := []struct {
		name    string
		current task.Phase
		want    task.Phase
		at      time.Time
	}{
		{"preliminary", task.PhaseDiscovery, task.PhaseThreeDaysBefore, deadline.Add(-2 * time.Minute)},
		{"refresh", task.PhaseThreeDaysBefore, task.PhaseOneDayBefore, deadline.Add(-time.Minute)},
		{"final", task.PhaseOneDayBefore, task.PhaseDeadlineReached, deadline},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			phase, at, ok := nextEventForTask(test.current, deadline, windows)
			if !ok || phase != test.want || !at.Equal(test.at) {
				t.Fatalf("nextEventForTask() = (%q, %s, %t), want (%q, %s, true)", phase, at, ok, test.want, test.at)
			}
		})
	}
}

func TestNextEventForTaskDoesNotScheduleTerminalPhases(t *testing.T) {
	deadline := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	for _, phase := range []task.Phase{task.PhaseDeadlineReached, task.PhaseCompleted} {
		if _, _, ok := nextEventForTask(phase, deadline, task.DefaultDeadlineWindows); ok {
			t.Fatalf("terminal phase %q unexpectedly scheduled", phase)
		}
	}
}

func TestNextEventForTaskUsesDefaultsForInvalidWindows(t *testing.T) {
	deadline := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	phase, at, ok := nextEventForTask(task.PhaseDiscovery, deadline, task.DeadlineWindows{})
	if !ok || phase != task.PhaseThreeDaysBefore || !at.Equal(deadline.Add(-task.DefaultDeadlineWindows.PreliminaryBefore)) {
		t.Fatalf("invalid windows produced (%q, %s, %t)", phase, at, ok)
	}
	phase, at, ok = nextEventForTask(task.PhaseThreeDaysBefore, deadline, task.DeadlineWindows{PreliminaryBefore: time.Minute, RefreshBefore: 2 * time.Minute})
	if !ok || phase != task.PhaseOneDayBefore || !at.Equal(deadline.Add(-task.DefaultDeadlineWindows.RefreshBefore)) {
		t.Fatalf("invalid refresh window produced (%q, %s, %t)", phase, at, ok)
	}
}

func TestNextDeadlineEventSkipsExpiredAndChoosesEarliest(t *testing.T) {
	now := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	bindings := []task.Binding{
		{Task: task.Task{ExternalID: "expired", DueAt: now.Add(-time.Hour).Format(time.RFC3339)}},
		{Task: task.Task{ExternalID: "later", DueAt: now.Add(2 * time.Hour).Format(time.RFC3339)}},
		{Task: task.Task{ExternalID: "earlier", DueAt: now.Add(time.Hour).Format(time.RFC3339)}},
		{Task: task.Task{ExternalID: "unknown"}},
	}
	event, ok := nextDeadlineEvent(bindings, now, task.DeadlineWindows{PreliminaryBefore: 30 * time.Minute, RefreshBefore: 10 * time.Minute})
	if !ok || event.binding.Task.ExternalID != "earlier" || !event.at.Equal(now.Add(30*time.Minute)) {
		t.Fatalf("event = %#v, found=%t", event, ok)
	}
}

func TestNextDeadlineEventSkipsTerminalPhases(t *testing.T) {
	now := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	if _, ok := nextDeadlineEvent([]task.Binding{{Task: task.Task{DueAt: now.Add(time.Hour).Format(time.RFC3339), Phase: task.PhaseCompleted}}}, now, task.DefaultDeadlineWindows); ok {
		t.Fatal("terminal phase unexpectedly scheduled")
	}
}

func TestDeadlineEventDisablesTaskWithoutFetching(t *testing.T) {
	bindings := &schedulerBindings{}
	service := &PollingService{bindings: bindings}
	event := deadlineEvent{binding: task.Binding{Task: task.Task{ExternalID: "task-7"}}, phase: task.PhaseDeadlineReached}
	if err := service.executeDeadlineEvent(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	if bindings.unbound != "task-7" {
		t.Fatalf("unbound task = %q", bindings.unbound)
	}
}

func TestSchedulerLoadsParticipantRosterBeforeFetchingSubmissions(t *testing.T) {
	participants := &schedulerParticipants{}
	service := &PollingService{
		participants: participants,
		source:       schedulerParticipantSource{items: []task.Participant{{ExternalID: "teacher-1", Email: "teacher@example.com"}}},
	}
	item := task.Task{ID: 17, ExternalID: "task-11", CourseID: "course-1"}
	if err := service.refreshParticipantRoster(context.Background(), item); err != nil {
		t.Fatal(err)
	}
	if participants.taskID != item.ID || len(participants.items) != 1 || participants.items[0].ExternalID != "teacher-1" {
		t.Fatalf("participants = %#v taskID=%d", participants.items, participants.taskID)
	}
}

func TestSchedulerReportsParticipantRefreshFailureAndRegistersCourse(t *testing.T) {
	source := &schedulerRegistrarSource{}
	service := &PollingService{source: source, participants: failingParticipantRepository{}, bindings: &schedulerBindings{}, tasks: &schedulerTaskRepository{}, submissions: &schedulerSubmissionRepository{}}
	event := deadlineEvent{binding: task.Binding{Task: task.Task{ID: 1, ExternalID: "task", CourseID: "course"}}, phase: task.PhaseThreeDaysBefore}
	if err := service.executeDeadlineEvent(context.Background(), event); err == nil || !strings.Contains(err.Error(), "persist participants for task") {
		t.Fatalf("executeDeadlineEvent() error = %v", err)
	}
	if !source.registered {
		t.Fatal("task course was not registered")
	}
}

func TestSchedulerUsesClockAndReturnsWhenContextIsCanceled(t *testing.T) {
	service := &PollingService{bindings: &schedulerBindings{}, clock: nil}
	if got := service.currentTime(); got.IsZero() {
		t.Fatal("currentTime returned zero time")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := service.RunScheduled(ctx); err != nil {
		t.Fatalf("RunScheduled() error = %v", err)
	}
	if err := waitForScheduler(ctx, time.Hour); err != context.Canceled {
		t.Fatalf("waitForScheduler() error = %v, want context canceled", err)
	}
	if err := waitForScheduler(context.Background(), time.Millisecond); err != nil {
		t.Fatalf("waitForScheduler() timer error = %v", err)
	}
}

func TestSchedulerReturnsActiveBindingErrors(t *testing.T) {
	service := &PollingService{bindings: &schedulerBindings{activeErr: context.Canceled}}
	if err := service.RunScheduled(context.Background()); err == nil {
		t.Fatal("RunScheduled() ignored active binding error")
	}
}

func TestSchedulerReturnsExpiredBindingErrors(t *testing.T) {
	service := &PollingService{bindings: &schedulerBindings{
		activeSequence: [][]task.Binding{{{Task: task.Task{ExternalID: "expired", DueAt: "2026-11-21T11:00:00Z"}}}},
		unbindErr:      errors.New("unbind failed"),
	}, clock: func() time.Time { return time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC) }}
	if err := service.RunScheduled(context.Background()); err == nil || !strings.Contains(err.Error(), "disable expired report task") {
		t.Fatalf("RunScheduled() error = %v", err)
	}
}

func TestSchedulerLogsEventFailureAndStopsWhenCanceled(t *testing.T) {
	now := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	bindings := &schedulerBindings{activeSequence: [][]task.Binding{{{Task: task.Task{ExternalID: "failed-event", CourseID: "course", DueAt: now.Format(time.RFC3339), Phase: task.PhaseDiscovery}}}, {}}}
	service := &PollingService{bindings: bindings, source: failingSubmissionSource{err: errors.New("submission provider failed")}, tasks: &schedulerTaskRepository{}, submissions: &schedulerSubmissionRepository{}, clock: func() time.Time { return now }}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()
	if err := service.RunScheduled(ctx); err != nil {
		t.Fatalf("RunScheduled() error = %v", err)
	}
}

func TestSchedulerReturnsImmediatelyForCanceledEventContext(t *testing.T) {
	now := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	service := &PollingService{bindings: &schedulerBindings{activeSequence: [][]task.Binding{{{Task: task.Task{ExternalID: "canceled-event", CourseID: "course", DueAt: now.Format(time.RFC3339), Phase: task.PhaseDiscovery}}}}}, source: failingSubmissionSource{err: errors.New("submission provider failed")}, tasks: &schedulerTaskRepository{}, submissions: &schedulerSubmissionRepository{}, clock: func() time.Time { return now }}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := service.RunScheduled(ctx); err != nil {
		t.Fatalf("RunScheduled() error = %v", err)
	}
}

func TestSchedulerHandlesDeadlineReachedAndRegistrationFailures(t *testing.T) {
	bindings := &schedulerBindings{}
	service := &PollingService{bindings: bindings}
	if err := service.executeDeadlineEvent(context.Background(), deadlineEvent{binding: task.Binding{Task: task.Task{ExternalID: "deadline"}}, phase: task.PhaseDeadlineReached}); err != nil {
		t.Fatal(err)
	}
	if bindings.unbound != "deadline" {
		t.Fatalf("unbound task = %q", bindings.unbound)
	}

	registering := &schedulerTaskRepository{err: context.Canceled}
	service = &PollingService{
		bindings:     &schedulerBindings{},
		source:       schedulerParticipantSource{},
		participants: &schedulerParticipants{},
		tasks:        registering,
		clock:        func() time.Time { return time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC) },
	}
	event := deadlineEvent{binding: task.Binding{Task: task.Task{ExternalID: "task", CourseID: "course"}}, phase: task.PhaseThreeDaysBefore}
	if err := service.executeDeadlineEvent(context.Background(), event); err == nil {
		t.Fatal("executeDeadlineEvent() ignored task registration failure")
	}
}

func TestRefreshParticipantRosterAllowsMissingOptionalDependencies(t *testing.T) {
	service := &PollingService{}
	if err := service.refreshParticipantRoster(context.Background(), task.Task{ExternalID: "task"}); err != nil {
		t.Fatal(err)
	}
	service = &PollingService{participants: &schedulerParticipants{}, source: schedulerSubmissionOnlySource{}}
	if err := service.refreshParticipantRoster(context.Background(), task.Task{ExternalID: "task"}); err == nil {
		t.Fatal("missing participant source was ignored")
	}
}

func TestSchedulerDisablesExpiredBindingsAndContinues(t *testing.T) {
	bindings := &schedulerBindings{activeSequence: [][]task.Binding{
		{{Task: task.Task{ExternalID: "expired", DueAt: "2026-11-21T11:00:00Z"}}},
		{},
	}}
	service := &PollingService{bindings: bindings, clock: func() time.Time {
		return time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := service.RunScheduled(ctx); err != nil {
		t.Fatalf("RunScheduled() error = %v", err)
	}
	if bindings.unbound != "expired" {
		t.Fatalf("unbound task = %q", bindings.unbound)
	}
}

func TestSchedulerWaitsForFutureEventUntilCanceled(t *testing.T) {
	now := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	bindings := &schedulerBindings{activeSequence: [][]task.Binding{{
		{Task: task.Task{ExternalID: "future", DueAt: now.Add(time.Hour).Format(time.RFC3339)}},
	}}}
	service := &PollingService{bindings: bindings, deadlines: task.DeadlineWindows{PreliminaryBefore: time.Minute, RefreshBefore: 30 * time.Second}, clock: func() time.Time { return now }}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := service.RunScheduled(ctx); err != nil {
		t.Fatalf("RunScheduled() error = %v", err)
	}
}

func TestSchedulerProcessesDueDeadlineEvent(t *testing.T) {
	now := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	bindings := &schedulerBindings{activeSequence: [][]task.Binding{{{Task: task.Task{ExternalID: "due", DueAt: now.Format(time.RFC3339), Phase: task.PhaseOneDayBefore}}}, {}}}
	service := &PollingService{bindings: bindings, clock: func() time.Time { return now }}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := service.RunScheduled(ctx); err != nil {
		t.Fatal(err)
	}
	if bindings.unbound != "due" {
		t.Fatalf("due task was not unbound: %q", bindings.unbound)
	}
}

func TestSchedulerPropagatesUnbindAndParticipantFailures(t *testing.T) {
	service := &PollingService{bindings: &schedulerBindings{unbindErr: context.Canceled}}
	event := deadlineEvent{binding: task.Binding{Task: task.Task{ExternalID: "task"}}, phase: task.PhaseDeadlineReached}
	if err := service.executeDeadlineEvent(context.Background(), event); err == nil {
		t.Fatal("deadline unbind failure was ignored")
	}
	service = &PollingService{source: failingParticipantSource{}, participants: &schedulerParticipants{}}
	if err := service.refreshParticipantRoster(context.Background(), task.Task{ExternalID: "task"}); err == nil {
		t.Fatal("participant source failure was ignored")
	}
	service = &PollingService{source: schedulerParticipantSource{}, participants: failingParticipantRepository{}}
	if err := service.refreshParticipantRoster(context.Background(), task.Task{ExternalID: "task"}); err == nil {
		t.Fatal("participant repository failure was ignored")
	}
}

func TestSchedulerReportsExpiredUnbindAndScheduledProcessingErrors(t *testing.T) {
	now := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	service := &PollingService{bindings: &schedulerBindings{unbindErr: errors.New("unbind failed")}}
	if err := service.disableExpiredBindings(context.Background(), []task.Binding{{Task: task.Task{ExternalID: "expired", DueAt: now.Add(-time.Hour).Format(time.RFC3339)}}}, now); err == nil {
		t.Fatal("expired unbind error was ignored")
	}
	service = &PollingService{bindings: &schedulerBindings{}, source: failingSubmissionSource{err: errors.New("submission failed")}, clock: func() time.Time { return now }}
	event := deadlineEvent{binding: task.Binding{Task: task.Task{ExternalID: "task", CourseID: "course"}}, phase: task.PhaseThreeDaysBefore}
	if err := service.executeDeadlineEvent(context.Background(), event); err == nil {
		t.Fatal("scheduled processing error was ignored")
	}
}

func TestSchedulerRefreshTaskPersistsSubmissions(t *testing.T) {
	repository := &schedulerSubmissionRepository{}
	service := &PollingService{
		source:      schedulerParticipantSource{},
		submissions: repository,
	}
	service.source = submissionSourceWithItems{}
	binding := task.Binding{Task: task.Task{ID: 7, ExternalID: "task", Phase: task.PhaseOneDayBefore}}
	if err := service.refreshTask(context.Background(), binding); err != nil {
		t.Fatal(err)
	}
	if len(repository.items) != 1 || repository.items[0].TaskID != 7 || repository.items[0].TaskExternalID != "task" {
		t.Fatalf("persisted submissions = %#v", repository.items)
	}
	service.source = failingSubmissionSource{err: context.Canceled}
	if err := service.refreshTask(context.Background(), binding); err == nil {
		t.Fatal("submission source failure was ignored")
	}
	repository.err = context.Canceled
	service.source = submissionSourceWithItems{}
	if err := service.refreshTask(context.Background(), binding); err == nil {
		t.Fatal("submission repository failure was ignored")
	}
}

func TestSchedulerExecutesReportPhaseAndPersistsIt(t *testing.T) {
	repository := &schedulerSubmissionRepository{}
	tasks := &schedulerTaskRepository{}
	service := &PollingService{
		source:      submissionSourceWithItems{},
		submissions: repository,
		tasks:       tasks,
		clock: func() time.Time {
			return time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
		},
	}
	event := deadlineEvent{binding: task.Binding{Task: task.Task{ID: 9, ExternalID: "task", CourseID: "course"}}, phase: task.PhaseThreeDaysBefore}
	if err := service.executeDeadlineEvent(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	if len(tasks.items) != 1 || tasks.items[0].Phase != task.PhaseThreeDaysBefore {
		t.Fatalf("persisted tasks = %#v", tasks.items)
	}
	tasks.err = context.Canceled
	if err := service.executeDeadlineEvent(context.Background(), event); err == nil {
		t.Fatal("task persistence failure was ignored")
	}
	service.source = failingSubmissionSource{err: context.Canceled}
	tasks.err = nil
	if err := service.executeDeadlineEvent(context.Background(), event); err == nil {
		t.Fatal("scheduled refresh failure was ignored")
	}
}

type submissionSourceWithItems struct{}

func (submissionSourceWithItems) ListTasks(context.Context, string) ([]task.Task, error) {
	return nil, nil
}
func (submissionSourceWithItems) ListSubmissions(context.Context, string) ([]submission.Submission, error) {
	return []submission.Submission{{ExternalID: "submission", State: submission.Submitted}}, nil
}

func TestSchedulerReportsBindingAndParticipantErrors(t *testing.T) {
	service := &PollingService{bindings: &schedulerBindings{activeErr: context.Canceled}}
	if err := service.disableExpiredBindings(context.Background(), nil, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := service.RunScheduled(context.Background()); err == nil {
		t.Fatal("RunScheduled() succeeded with binding error")
	}
	service = &PollingService{source: schedulerSubmissionOnlySource{}, participants: &schedulerParticipants{}}
	if err := service.refreshParticipantRoster(context.Background(), task.Task{ExternalID: "task"}); err == nil {
		t.Fatal("refreshParticipantRoster() succeeded without participant source")
	}
	service = &PollingService{}
	if err := service.refreshParticipantRoster(context.Background(), task.Task{ExternalID: "task"}); err != nil {
		t.Fatalf("nil participant source should be ignored: %v", err)
	}
}
