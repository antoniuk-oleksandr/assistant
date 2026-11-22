package application_test

import (
	"context"
	"errors"
	"time"

	"github.com/alex/assistent/internal/application"
	"github.com/alex/assistent/internal/domain/classroom"
	"github.com/alex/assistent/internal/domain/ports"
	"github.com/alex/assistent/internal/domain/scenario"
	"github.com/alex/assistent/internal/domain/submission"
	"github.com/alex/assistent/internal/domain/task"
	"github.com/alex/assistent/internal/domain/teacher"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type pollingCourseRepository struct {
	courses []classroom.Course
	err     error
}

func (r pollingCourseRepository) Register(context.Context, classroom.Course) error { return nil }
func (r pollingCourseRepository) List(context.Context) ([]classroom.Course, error) {
	return r.courses, r.err
}
func (r pollingCourseRepository) Disable(context.Context, string) error { return nil }

type pollingSource struct {
	tasks            map[string][]task.Task
	submissions      map[string][][]submission.Submission
	taskCalls        []string
	sinceCalls       []time.Time
	subCalls         []string
	taskErr          error
	subErr           error
	participants     map[string][]task.Participant
	participantsErr  error
	cancelOnTaskCall int
	cancel           context.CancelFunc
}

type nonIncrementalPollingSource struct {
	tasks       map[string][]task.Task
	submissions map[string][]submission.Submission
}

func (s *nonIncrementalPollingSource) ListTasks(_ context.Context, courseID string) ([]task.Task, error) {
	return s.tasks[courseID], nil
}

func (s *nonIncrementalPollingSource) ListSubmissions(_ context.Context, taskID string) ([]submission.Submission, error) {
	return s.submissions[taskID], nil
}

func (s *pollingSource) ListTasks(_ context.Context, courseID string) ([]task.Task, error) {
	s.taskCalls = append(s.taskCalls, courseID)
	if s.cancel != nil && len(s.taskCalls) == s.cancelOnTaskCall {
		s.cancel()
	}
	return s.tasks[courseID], s.taskErr
}

func (s *pollingSource) ListTasksSince(ctx context.Context, courseID string, checkpoint time.Time) ([]task.Task, error) {
	s.sinceCalls = append(s.sinceCalls, checkpoint)
	return s.ListTasks(ctx, courseID)
}

func (s *pollingSource) ListSubmissions(_ context.Context, taskID string) ([]submission.Submission, error) {
	s.subCalls = append(s.subCalls, taskID)
	if s.subErr != nil {
		return nil, s.subErr
	}
	batches := s.submissions[taskID]
	if len(batches) == 0 {
		return nil, nil
	}
	batch := batches[0]
	if len(batches) > 1 {
		s.submissions[taskID] = batches[1:]
	}
	return batch, nil
}

func (s *pollingSource) ListParticipants(_ context.Context, courseID string) ([]task.Participant, error) {
	if s.participantsErr != nil {
		return nil, s.participantsErr
	}
	return s.participants[courseID], nil
}

type pollingTaskRepository struct {
	registered   []task.Task
	err          error
	listErr      error
	listCalls    int
	listErrAfter int
}

func (r *pollingTaskRepository) Register(_ context.Context, item task.Task) error {
	if r.err != nil {
		return r.err
	}
	for index, existing := range r.registered {
		if existing.Provider == item.Provider && existing.CourseID == item.CourseID && existing.ExternalID == item.ExternalID {
			r.registered[index] = item
			return nil
		}
	}
	r.registered = append(r.registered, item)
	return nil
}
func (r *pollingTaskRepository) List(context.Context) ([]task.Task, error) {
	r.listCalls++
	if r.listErrAfter > 0 && r.listCalls > r.listErrAfter {
		return nil, r.listErr
	}
	if r.listErrAfter > 0 {
		return r.registered, nil
	}
	return r.registered, r.listErr
}

type pollingBindingRepository struct {
	bindings       []task.Binding
	activeBindings []task.Binding
	activeErr      error
}

func (r pollingBindingRepository) Bind(context.Context, string, string) error { return nil }
func (r pollingBindingRepository) Unbind(context.Context, string) error       { return nil }
func (r pollingBindingRepository) List(context.Context, string) ([]task.Binding, error) {
	return r.bindings, nil
}
func (r pollingBindingRepository) Active(context.Context) ([]task.Binding, error) {
	if r.activeErr != nil {
		return nil, r.activeErr
	}
	if r.activeBindings != nil {
		return r.activeBindings, nil
	}
	return r.bindings, nil
}

type pollingSubmissionRepository struct {
	items []submission.Submission
	err   error
}

type pollingParticipantRepository struct {
	items map[int64][]task.Participant
	err   error
}

type pollingMissingRepository struct {
	items                 map[int64][]ports.MissingSubmitter
	err                   error
	listErr, reconcileErr error
}

func (r *pollingMissingRepository) Reconcile(_ context.Context, taskID int64, items []ports.MissingSubmitter) error {
	if r.reconcileErr != nil {
		return r.reconcileErr
	}
	if r.err != nil {
		return r.err
	}
	r.items[taskID] = items
	return nil
}
func (r *pollingMissingRepository) List(_ context.Context, taskID int64) ([]ports.MissingSubmitter, error) {
	if r.listErr != nil {
		return nil, r.listErr
	}
	if r.err != nil {
		return nil, r.err
	}
	return r.items[taskID], nil
}

func (r *pollingParticipantRepository) Replace(_ context.Context, taskID int64, items []task.Participant) error {
	if r.err != nil {
		return r.err
	}
	r.items[taskID] = items
	return nil
}

func (r *pollingParticipantRepository) List(_ context.Context, taskID int64) ([]task.Participant, error) {
	if r.err != nil {
		return nil, r.err
	}
	return r.items[taskID], nil
}

type pollingStateRepository struct {
	checkpoint   time.Time
	lastCalls    int
	saved        []time.Time
	lastErr      error
	lastErrAfter int
	saveErr      error
}

func (r *pollingStateRepository) LastCheckpoint(context.Context) (time.Time, error) {
	r.lastCalls++
	if r.lastErrAfter > 0 && r.lastCalls > r.lastErrAfter {
		return time.Time{}, r.lastErr
	}
	if r.lastErrAfter > 0 {
		return r.checkpoint, nil
	}
	return r.checkpoint, r.lastErr
}

func (r *pollingStateRepository) SaveCheckpoint(_ context.Context, checkpoint time.Time) error {
	if r.saveErr != nil {
		return r.saveErr
	}
	r.saved = append(r.saved, checkpoint)
	return nil
}

func (r *pollingSubmissionRepository) Upsert(_ context.Context, item submission.Submission) error {
	if r.err != nil {
		return r.err
	}
	r.items = append(r.items, item)
	return nil
}
func (r *pollingSubmissionRepository) ListByTask(context.Context, int64) ([]submission.Submission, error) {
	return r.items, nil
}

var _ ports.SubmissionSource = (*pollingSource)(nil)
var _ ports.SubmissionRepository = (*pollingSubmissionRepository)(nil)

var _ = Describe("polling service", func() {
	It("rejects invalid polling intervals", func(ctx SpecContext) {
		service := application.NewPollingService(nil, nil, nil, nil, nil)
		Expect(service.Run(ctx, 0)).To(MatchError(ContainSubstring("greater than zero")))
	})

	It("propagates polling course and task repository errors", func(ctx SpecContext) {
		service := application.NewPollingService(pollingCourseRepository{err: errors.New("courses failed")}, &nonIncrementalPollingSource{}, &pollingTaskRepository{}, pollingBindingRepository{}, &pollingSubmissionRepository{})
		Expect(service.Refresh(ctx)).To(MatchError(ContainSubstring("list courses for polling")))
		service = application.NewPollingService(pollingCourseRepository{courses: []classroom.Course{{ExternalID: "course-1", Status: classroom.Active}}}, &pollingSource{taskErr: errors.New("tasks failed")}, &pollingTaskRepository{}, pollingBindingRepository{}, &pollingSubmissionRepository{})
		Expect(service.Refresh(ctx)).To(MatchError(ContainSubstring("list tasks for course")))
		service = application.NewPollingService(pollingCourseRepository{courses: []classroom.Course{{ExternalID: "course-1", Status: classroom.Active}}}, &pollingSource{}, &pollingTaskRepository{listErr: errors.New("stored tasks failed")}, pollingBindingRepository{}, &pollingSubmissionRepository{})
		Expect(service.Refresh(ctx)).To(MatchError(ContainSubstring("list persisted tasks")))
		service = application.NewPollingServiceWithScenarioCollectionAndRoster(pollingCourseRepository{courses: []classroom.Course{{ExternalID: "course-1", Status: classroom.Active}}}, &pollingSource{participantsErr: errors.New("roster failed")}, &pollingTaskRepository{}, pollingBindingRepository{}, &pollingSubmissionRepository{}, nil, nil, nil, nil, scenario.Registry{}, &pollingParticipantRepository{items: map[int64][]task.Participant{}})
		Expect(service.Refresh(ctx)).To(MatchError(ContainSubstring("list Classroom participants")))
	})

	It("supports sources without incremental polling", func(ctx SpecContext) {
		item := task.Task{ID: 31, ExternalID: "task-1", CourseID: "course-1"}
		source := &nonIncrementalPollingSource{tasks: map[string][]task.Task{"course-1": {item}}}
		tasks := &pollingTaskRepository{}
		service := application.NewPollingService(
			pollingCourseRepository{courses: []classroom.Course{{ExternalID: "course-1", Status: classroom.Active}}},
			source, tasks, pollingBindingRepository{}, &pollingSubmissionRepository{},
		)
		Expect(service.Refresh(ctx)).To(Succeed())
		Expect(tasks.registered).To(HaveLen(1))
		Expect(tasks.registered[0].ExternalID).To(Equal(item.ExternalID))
		Expect(tasks.registered[0].Phase).To(Equal(task.PhaseDiscovery))
	})

	It("skips inactive courses during refresh", func(ctx SpecContext) {
		source := &pollingSource{}
		service := application.NewPollingService(
			pollingCourseRepository{courses: []classroom.Course{{ExternalID: "inactive", Status: classroom.Disabled}}},
			source, &pollingTaskRepository{}, pollingBindingRepository{}, &pollingSubmissionRepository{},
		)
		Expect(service.Refresh(ctx)).To(Succeed())
		Expect(source.taskCalls).To(BeEmpty())
	})

	It("allows polling without a task repository", func(ctx SpecContext) {
		item := task.Task{ExternalID: "task-without-repository", CourseID: "course-1"}
		service := application.NewPollingService(
			pollingCourseRepository{courses: []classroom.Course{{ExternalID: "course-1", Status: classroom.Active}}},
			&nonIncrementalPollingSource{tasks: map[string][]task.Task{"course-1": {item}}}, nil, pollingBindingRepository{}, &pollingSubmissionRepository{},
		)
		Expect(service.Refresh(ctx)).To(Succeed())
	})

	It("preserves discovery phase and reports deadline phase persistence failures", func(ctx SpecContext) {
		future := "2026-11-30T12:00:00Z"
		item := task.Task{ID: 62, ExternalID: "discovery", CourseID: "course-1", DueAt: future}
		service := application.NewPollingServiceWithClock(
			pollingCourseRepository{courses: []classroom.Course{{ExternalID: "course-1", Status: classroom.Active}}},
			&nonIncrementalPollingSource{}, &pollingTaskRepository{}, pollingBindingRepository{bindings: []task.Binding{{Task: item}}}, &pollingSubmissionRepository{}, nil,
			func() time.Time { return time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC) },
		)
		Expect(service.Refresh(ctx)).To(Succeed())

		item.Phase = task.PhaseThreeDaysBefore
		service = application.NewPollingServiceWithClock(
			pollingCourseRepository{courses: []classroom.Course{{ExternalID: "course-1", Status: classroom.Active}}},
			&nonIncrementalPollingSource{}, &pollingTaskRepository{err: errors.New("phase persistence failed")}, pollingBindingRepository{bindings: []task.Binding{{Task: item}}}, &pollingSubmissionRepository{}, nil,
			func() time.Time { return time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC) },
		)
		Expect(service.Refresh(ctx)).To(MatchError(ContainSubstring("store deadline phase")))
	})

	It("skips inactive courses during scheduled refresh and reports roster task lookup failures", func(ctx SpecContext) {
		pollingCtx, cancel := context.WithCancel(ctx)
		defer cancel()
		source := &pollingSource{}
		service := application.NewPersistentPollingService(
			pollingCourseRepository{courses: []classroom.Course{{ExternalID: "inactive", Status: classroom.Disabled}}}, source, &pollingTaskRepository{}, pollingBindingRepository{}, &pollingSubmissionRepository{}, &pollingStateRepository{},
		)
		go func() { time.Sleep(2 * time.Millisecond); cancel() }()
		Expect(service.Run(pollingCtx, time.Millisecond)).To(Succeed())
		Expect(source.taskCalls).To(BeEmpty())

		item := task.Task{ExternalID: "roster-task", CourseID: "course-1"}
		rosterSource := &pollingSource{tasks: map[string][]task.Task{"course-1": {item}}, participants: map[string][]task.Participant{"course-1": {{ExternalID: "user-1"}}}}
		tasks := &pollingTaskRepository{listErr: errors.New("roster task lookup failed"), listErrAfter: 1}
		service = application.NewPollingServiceWithScenarioCollectionAndRoster(
			pollingCourseRepository{courses: []classroom.Course{{ExternalID: "course-1", Status: classroom.Active}}}, rosterSource, tasks, pollingBindingRepository{}, &pollingSubmissionRepository{}, nil, nil, nil, nil, scenario.Registry{}, &pollingParticipantRepository{items: map[int64][]task.Participant{}},
		)
		Expect(service.Refresh(ctx)).To(MatchError(ContainSubstring("list tasks for participant roster")))
	})

	It("persists each deadline phase and keeps repeated phases idempotent", func(ctx SpecContext) {
		item := task.Task{Provider: "google-classroom", ExternalID: "deadline-task", CourseID: "course-1", DueDate: "2026-11-10", DueTime: "12:00:00", TimeZone: "UTC"}
		source := &nonIncrementalPollingSource{tasks: map[string][]task.Task{"course-1": {item}}}
		tasks := &pollingTaskRepository{}
		current := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
		service := application.NewPollingServiceWithClock(
			pollingCourseRepository{courses: []classroom.Course{{ExternalID: "course-1", Status: classroom.Active}}},
			source, tasks, pollingBindingRepository{}, &pollingSubmissionRepository{}, nil,
			func() time.Time { return current },
		)

		refresh := func(want task.Phase) {
			Expect(service.Refresh(ctx)).To(Succeed())
			Expect(tasks.registered).To(HaveLen(1))
			Expect(tasks.registered[0].Phase).To(Equal(want))
			Expect(tasks.registered[0].PhaseAt).NotTo(BeEmpty())
		}
		refresh(task.PhaseThreeDaysBefore)
		current = time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
		refresh(task.PhaseOneDayBefore)
		current = time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
		refresh(task.PhaseDeadlineReached)
		current = current.Add(time.Minute)
		refresh(task.PhaseCompleted)
		phaseAt := tasks.registered[0].PhaseAt
		refresh(task.PhaseCompleted)
		Expect(tasks.registered[0].PhaseAt).To(Equal(phaseAt))
	})

	It("recomputes stale bound phases and never processes after the deadline", func(ctx SpecContext) {
		deadline := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
		item := task.Task{ID: 51, Provider: "google-classroom", ExternalID: "deadline-task", CourseID: "course-1", DueAt: deadline.Format(time.RFC3339), Phase: task.PhaseOneDayBefore}
		tasks := &pollingTaskRepository{registered: []task.Task{item}}
		source := &pollingSource{submissions: map[string][][]submission.Submission{"deadline-task": {nil}}}
		bindings := pollingBindingRepository{bindings: []task.Binding{{ScenarioName: "science-report", ScenarioID: 1, Task: item, Status: "active"}}}
		service := application.NewPollingServiceWithClock(
			pollingCourseRepository{courses: []classroom.Course{{ExternalID: "course-1", Status: classroom.Active}}},
			source, tasks, bindings, &pollingSubmissionRepository{}, nil,
			func() time.Time { return deadline.Add(2 * time.Minute) },
		)
		service.SetDeadlineWindows(task.DeadlineWindows{PreliminaryBefore: 2 * time.Minute, RefreshBefore: time.Minute})

		Expect(service.Refresh(ctx)).To(Succeed())
		Expect(tasks.registered[0].Phase).To(Equal(task.PhaseCompleted))
		Expect(source.subCalls).To(BeEmpty())
	})

	It("persists the course roster and matches submissions by stable user ID", func(ctx SpecContext) {
		item := task.Task{ID: 41, Provider: "google-classroom", ExternalID: "roster-task", CourseID: "course-1"}
		source := &pollingSource{
			tasks:        map[string][]task.Task{"course-1": {item}},
			participants: map[string][]task.Participant{"course-1": {{ExternalID: "user-1", Name: "Teacher One", Email: "teacher@example.test"}}},
			submissions:  map[string][][]submission.Submission{"roster-task": {{{ExternalID: "submission-1", SubmitterID: "user-1", State: submission.Submitted}}}},
		}
		tasks := &pollingTaskRepository{}
		participants := &pollingParticipantRepository{items: map[int64][]task.Participant{}}
		submissions := &pollingSubmissionRepository{}
		service := application.NewPollingServiceWithScenarioCollectionAndRoster(
			pollingCourseRepository{courses: []classroom.Course{{ExternalID: "course-1", Status: classroom.Active}}}, source, tasks,
			pollingBindingRepository{bindings: []task.Binding{{Task: item, Status: "active"}}}, submissions, nil, nil, nil, nil, scenario.Registry{}, participants,
		)
		Expect(service.Refresh(ctx)).To(Succeed())
		Expect(participants.items[41]).To(Equal([]task.Participant{{ExternalID: "user-1", Name: "Teacher One", Email: "teacher@example.test"}}))
		Expect(submissions.items).To(HaveLen(1))
		Expect(submissions.items[0].SubmitterName).To(Equal("Teacher One"))
		Expect(submissions.items[0].SubmitterEmail).To(Equal("teacher@example.test"))
	})

	It("synchronizes Classroom participants into the teacher directory", func(ctx SpecContext) {
		item := task.Task{ID: 45, Provider: "google-classroom", ExternalID: "teacher-sync-task", CourseID: "course-1"}
		source := &pollingSource{
			tasks:        map[string][]task.Task{"course-1": {item}},
			participants: map[string][]task.Participant{"course-1": {{ExternalID: "teacher-1", Name: "Teacher One", Email: "teacher@example.test"}}},
			submissions:  map[string][][]submission.Submission{"teacher-sync-task": {{{ExternalID: "submission-1", SubmitterID: "teacher-1", State: submission.Submitted}}}},
		}
		repository := &teacherRepo{}
		service := application.NewPollingServiceWithScenarioCollectionAndRoster(
			pollingCourseRepository{courses: []classroom.Course{{ExternalID: "course-1", Status: classroom.Active}}}, source, &pollingTaskRepository{},
			pollingBindingRepository{bindings: []task.Binding{{Task: item, Status: "active"}}}, &pollingSubmissionRepository{}, nil, nil, nil, nil, scenario.Registry{}, &pollingParticipantRepository{items: map[int64][]task.Participant{}},
		)
		service.SetTeacherRepository(repository)
		Expect(service.Refresh(ctx)).To(Succeed())
		Expect(repository.value.Name).To(Equal("Teacher One"))
		Expect(repository.value.Email).To(Equal("teacher@example.test"))
		Expect(repository.value.ClassroomID).To(Equal("teacher-1"))
	})

	It("updates changed Classroom teacher details", func(ctx SpecContext) {
		item := task.Task{ID: 46, Provider: "google-classroom", ExternalID: "teacher-update-task", CourseID: "course-1"}
		source := &pollingSource{
			tasks:        map[string][]task.Task{"course-1": {item}},
			participants: map[string][]task.Participant{"course-1": {{ExternalID: "teacher-1", Name: "Updated", Email: "updated@example.test"}}},
			submissions: map[string][][]submission.Submission{
				"teacher-update-task": {
					{{ExternalID: "submission-1", SubmitterID: "teacher-1", State: submission.Submitted}},
				},
			},
		}
		repository := &teacherRepo{value: teacher.Recipient{ID: 9, ClassroomID: "teacher-1", Name: "Old", Email: "old@example.test"}}
		service := application.NewPollingServiceWithScenarioCollectionAndRoster(pollingCourseRepository{courses: []classroom.Course{{ExternalID: "course-1", Status: classroom.Active}}}, source, &pollingTaskRepository{}, pollingBindingRepository{bindings: []task.Binding{{Task: item, Status: "active"}}}, &pollingSubmissionRepository{}, nil, nil, nil, nil, scenario.Registry{}, &pollingParticipantRepository{items: map[int64][]task.Participant{}})
		service.SetTeacherRepository(repository)
		Expect(service.Refresh(ctx)).To(Succeed())
		Expect(repository.value.Name).To(Equal("Updated"))
		Expect(repository.value.Email).To(Equal("updated@example.test"))
		repository.value.Name = "Old"
		repository.updateErr = errors.New("teacher update failed")
		Expect(service.Refresh(ctx)).To(MatchError(ContainSubstring("update Classroom contact")))
	})

	It("reports teacher directory synchronization failures", func(ctx SpecContext) {
		item := task.Task{ID: 47, Provider: "google-classroom", ExternalID: "teacher-error-task", CourseID: "course-1"}
		source := &pollingSource{
			tasks:        map[string][]task.Task{"course-1": {item}},
			participants: map[string][]task.Participant{"course-1": {{ExternalID: "teacher-1"}}},
			submissions: map[string][][]submission.Submission{
				"teacher-error-task": {
					{{ExternalID: "submission-1", SubmitterID: "teacher-1", State: submission.Submitted}},
				},
			},
		}
		service := application.NewPollingServiceWithScenarioCollectionAndRoster(pollingCourseRepository{courses: []classroom.Course{{ExternalID: "course-1", Status: classroom.Active}}}, source, &pollingTaskRepository{}, pollingBindingRepository{bindings: []task.Binding{{Task: item, Status: "active"}}}, &pollingSubmissionRepository{}, nil, nil, nil, nil, scenario.Registry{}, &pollingParticipantRepository{items: map[int64][]task.Participant{}})
		service.SetTeacherRepository(&teacherRepo{listErr: errors.New("teacher list failed")})
		Expect(service.Refresh(ctx)).To(MatchError(ContainSubstring("list persisted Classroom contacts")))
		service.SetTeacherRepository(&teacherRepo{createErr: errors.New("teacher create failed")})
		Expect(service.Refresh(ctx)).To(MatchError(ContainSubstring("store Classroom contact")))
	})

	It("reconciles missing submitters and removes users after they submit", func(ctx SpecContext) {
		item := task.Task{ID: 42, Provider: "google-classroom", ExternalID: "missing-task", CourseID: "course-1"}
		source := &pollingSource{
			tasks:        map[string][]task.Task{"course-1": {item}},
			participants: map[string][]task.Participant{"course-1": {{ExternalID: "user-1", Name: "First"}, {ExternalID: "user-2", Name: "Second"}}},
			submissions:  map[string][][]submission.Submission{"missing-task": {{{ExternalID: "s1", SubmitterID: "user-1", State: submission.Submitted}}, {{ExternalID: "s1", SubmitterID: "user-1", State: submission.Submitted}, {ExternalID: "s2", SubmitterID: "user-2", State: submission.Submitted}}}},
		}
		participants := &pollingParticipantRepository{items: map[int64][]task.Participant{}}
		missing := &pollingMissingRepository{items: map[int64][]ports.MissingSubmitter{}}
		service := application.NewPollingServiceWithScenarioCollectionAndRosterAndMissing(
			pollingCourseRepository{courses: []classroom.Course{{ExternalID: "course-1", Status: classroom.Active}}}, source, &pollingTaskRepository{},
			pollingBindingRepository{bindings: []task.Binding{{Task: item, Status: "active"}}}, &pollingSubmissionRepository{}, nil, nil, nil, nil, scenario.Registry{}, participants, missing,
		)
		Expect(service.Refresh(ctx)).To(Succeed())
		Expect(missing.items[42]).To(Equal([]ports.MissingSubmitter{{TaskID: 42, ExternalUserID: "user-2", Name: "Second", NotificationState: "pending"}}))
		Expect(service.Refresh(ctx)).To(Succeed())
		Expect(missing.items[42]).To(BeEmpty())
	})

	It("reports missing submitter reconciliation failures", func(ctx SpecContext) {
		item := task.Task{ID: 43, Provider: "google-classroom", ExternalID: "missing-error", CourseID: "course-1"}
		source := &pollingSource{
			tasks:        map[string][]task.Task{"course-1": {item}},
			participants: map[string][]task.Participant{"course-1": {{ExternalID: "user-1"}}},
			submissions:  map[string][][]submission.Submission{"missing-error": {{{ExternalID: "submission-1", State: submission.Submitted}}}},
		}
		service := application.NewPollingServiceWithScenarioCollectionAndRosterAndMissing(pollingCourseRepository{courses: []classroom.Course{{ExternalID: "course-1", Status: classroom.Active}}}, source, &pollingTaskRepository{}, pollingBindingRepository{bindings: []task.Binding{{Task: item}}}, &pollingSubmissionRepository{}, nil, nil, nil, nil, scenario.Registry{}, &pollingParticipantRepository{items: map[int64][]task.Participant{}}, &pollingMissingRepository{items: map[int64][]ports.MissingSubmitter{}, err: errors.New("missing store failed")})
		Expect(service.Refresh(ctx)).To(MatchError(ContainSubstring("list missing submitters: missing store failed")))
	})

	It("reports participant roster lookup failures", func(ctx SpecContext) {
		item := task.Task{ID: 44, Provider: "google-classroom", ExternalID: "participant-error", CourseID: "course-1"}
		source := &pollingSource{tasks: map[string][]task.Task{"course-1": {item}}, participants: map[string][]task.Participant{"course-1": {{ExternalID: "user-1"}}}}
		service := application.NewPollingServiceWithScenarioCollectionAndRosterAndMissing(pollingCourseRepository{courses: []classroom.Course{{ExternalID: "course-1", Status: classroom.Active}}}, source, &pollingTaskRepository{}, pollingBindingRepository{}, &pollingSubmissionRepository{}, nil, nil, nil, nil, scenario.Registry{}, &pollingParticipantRepository{items: map[int64][]task.Participant{}, err: errors.New("roster failed")}, &pollingMissingRepository{items: map[int64][]ports.MissingSubmitter{}})
		Expect(service.Refresh(ctx)).To(MatchError(ContainSubstring("store participants for task \"participant-error\": roster failed")))
	})

	It("reports scheduled reminder and missing roster persistence failures", func(ctx SpecContext) {
		item := task.Task{ID: 70, ExternalID: "reminder-error", CourseID: "course-1", DueAt: "2026-11-24T12:00:00Z", Phase: task.PhaseThreeDaysBefore}
		source := &nonIncrementalPollingSource{tasks: map[string][]task.Task{"course-1": {item}}, submissions: map[string][]submission.Submission{"reminder-error": nil}}
		missing := &pollingMissingRepository{items: map[int64][]ports.MissingSubmitter{}, listErr: errors.New("reminder missing lookup failed")}
		reminders := application.NewReminderService(&reminderRepository{}, []ports.ReminderProvider{})
		service := application.NewPollingServiceWithScenarioCollectionAndRosterAndMissingAndReminders(
			pollingCourseRepository{courses: []classroom.Course{{ExternalID: "course-1", Status: classroom.Active}}}, source, &pollingTaskRepository{}, pollingBindingRepository{bindings: []task.Binding{{Task: item}}}, &pollingSubmissionRepository{}, nil, nil, nil, nil, scenario.Registry{}, nil, missing, reminders,
		)
		service.SetClock(func() time.Time { return time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC) })
		Expect(service.Refresh(ctx)).To(MatchError(ContainSubstring("list missing submitters for reminders")))

		participants := &pollingParticipantRepository{items: map[int64][]task.Participant{}, err: errors.New("expected participant lookup failed")}
		service = application.NewPollingServiceWithScenarioCollectionAndRosterAndMissing(
			pollingCourseRepository{courses: []classroom.Course{{ExternalID: "course-1", Status: classroom.Active}}}, source, &pollingTaskRepository{}, pollingBindingRepository{bindings: []task.Binding{{Task: item}}}, &pollingSubmissionRepository{}, nil, nil, nil, nil, scenario.Registry{}, participants, &pollingMissingRepository{items: map[int64][]ports.MissingSubmitter{}},
		)
		Expect(service.Refresh(ctx)).To(MatchError(ContainSubstring("list expected participants")))

		participants = &pollingParticipantRepository{items: map[int64][]task.Participant{70: {{ExternalID: "user-1"}}}}
		service = application.NewPollingServiceWithScenarioCollectionAndRosterAndMissing(
			pollingCourseRepository{courses: []classroom.Course{{ExternalID: "course-1", Status: classroom.Active}}}, source, &pollingTaskRepository{}, pollingBindingRepository{bindings: []task.Binding{{Task: item}}}, &pollingSubmissionRepository{}, nil, nil, nil, nil, scenario.Registry{}, participants, &pollingMissingRepository{items: map[int64][]ports.MissingSubmitter{}, reconcileErr: errors.New("reconcile persistence failed")},
		)
		Expect(service.Refresh(ctx)).To(MatchError(ContainSubstring("reconcile missing submitters")))
	})

	It("excludes disabled courses and inactive task bindings from submission polling", func(ctx SpecContext) {
		activeTask := task.Task{ID: 21, ExternalID: "task-active", CourseID: "course-active"}
		inactiveTask := task.Task{ID: 22, ExternalID: "task-inactive", CourseID: "course-active"}
		source := &pollingSource{
			tasks: map[string][]task.Task{
				"course-active":   {activeTask, inactiveTask},
				"course-disabled": {{ExternalID: "task-disabled", CourseID: "course-disabled"}},
			},
			submissions: map[string][][]submission.Submission{
				"task-active":   {{{ExternalID: "submission-active", State: submission.Submitted}}},
				"task-inactive": {{{ExternalID: "submission-inactive", State: submission.Submitted}}},
			},
		}
		bindings := pollingBindingRepository{
			bindings: []task.Binding{
				{Task: activeTask, Status: "active"},
				{Task: inactiveTask, Status: "disabled"},
			},
			activeBindings: []task.Binding{{Task: activeTask, Status: "active"}},
		}
		tasks := &pollingTaskRepository{}
		submissions := &pollingSubmissionRepository{}
		service := application.NewPollingService(
			pollingCourseRepository{courses: []classroom.Course{
				{ExternalID: "course-active", Status: classroom.Active},
				{ExternalID: "course-disabled", Status: classroom.Disabled},
			}}, source, tasks, bindings, submissions,
		)

		Expect(service.Refresh(ctx)).To(Succeed())
		Expect(source.taskCalls).To(Equal([]string{"course-active"}))
		Expect(source.subCalls).To(Equal([]string{"task-active"}))
		Expect(tasks.registered).To(HaveLen(2))
		Expect(tasks.registered[0].Phase).To(Equal(task.PhaseDiscovery))
		Expect(tasks.registered[1].Phase).To(Equal(task.PhaseDiscovery))
		Expect(submissions.items).To(HaveLen(1))
		Expect(submissions.items[0].ExternalID).To(Equal("submission-active"))
	})

	It("polls only active registered courses and bound tasks, preserving state changes across repeated refreshes", func(ctx SpecContext) {
		activeTask := task.Task{ID: 11, ExternalID: "task-active", CourseID: "course-active"}
		otherCourseTask := task.Task{ID: 12, ExternalID: "task-other", CourseID: "course-other"}
		source := &pollingSource{
			tasks: map[string][]task.Task{
				"course-active":   {activeTask},
				"course-disabled": {{ExternalID: "task-disabled", CourseID: "course-disabled"}},
			},
			submissions: map[string][][]submission.Submission{
				"task-active": {
					{{ExternalID: "submission-1", State: submission.Submitted, SubmitterEmail: "first@example.com"}},
					{{ExternalID: "submission-1", State: submission.Returned, SubmitterEmail: "second@example.com"}},
				},
			},
		}
		tasks := &pollingTaskRepository{}
		submissions := &pollingSubmissionRepository{}
		bindings := pollingBindingRepository{bindings: []task.Binding{
			{Task: activeTask, Status: "active"},
			{Task: otherCourseTask, Status: "active"},
		}}
		service := application.NewPollingService(
			pollingCourseRepository{courses: []classroom.Course{
				{ExternalID: "course-active", Status: classroom.Active},
				{ExternalID: "course-disabled", Status: classroom.Disabled},
			}}, source, tasks, bindings, submissions,
		)

		Expect(service.Refresh(ctx)).To(Succeed())
		Expect(service.Refresh(ctx)).To(Succeed())
		Expect(source.taskCalls).To(Equal([]string{"course-active", "course-active"}))
		Expect(source.subCalls).To(Equal([]string{"task-active", "task-active"}))
		Expect(tasks.registered).To(HaveLen(1))
		Expect(submissions.items).To(HaveLen(2))
		Expect(submissions.items[0].State).To(Equal(submission.Submitted))
		Expect(submissions.items[1].State).To(Equal(submission.Returned))
		Expect(submissions.items[1].SubmitterEmail).To(Equal("second@example.com"))
	})

	It("defaults an empty provider state to unknown", func(ctx SpecContext) {
		taskItem := task.Task{ID: 1, ExternalID: "task-1", CourseID: "course-1"}
		source := &pollingSource{
			tasks:       map[string][]task.Task{"course-1": {taskItem}},
			submissions: map[string][][]submission.Submission{"task-1": {{{ExternalID: "submission-1"}}}},
		}
		submissions := &pollingSubmissionRepository{}
		service := application.NewPollingService(
			pollingCourseRepository{courses: []classroom.Course{{ExternalID: "course-1", Status: classroom.Active}}},
			source, &pollingTaskRepository{}, pollingBindingRepository{bindings: []task.Binding{{Task: taskItem}}}, submissions,
		)

		Expect(service.Refresh(ctx)).To(Succeed())
		Expect(submissions.items[0].State).To(Equal(submission.Unknown))
	})

	It("returns source and repository failures with operation context", func(ctx SpecContext) {
		source := &pollingSource{}
		repositoryError := errors.New("persist failed")
		submissions := &pollingSubmissionRepository{err: repositoryError}
		item := task.Task{ID: 1, ExternalID: "task-1", CourseID: "course-1"}
		service := application.NewPollingService(
			pollingCourseRepository{courses: []classroom.Course{{ExternalID: "course-1", Status: classroom.Active}}},
			source, &pollingTaskRepository{}, pollingBindingRepository{bindings: []task.Binding{{Task: item}}}, submissions,
		)
		source.submissions = map[string][][]submission.Submission{"task-1": {{{ExternalID: "submission-1"}}}}
		Expect(service.Refresh(ctx)).To(MatchError(ContainSubstring("store submission \"submission-1\": persist failed")))
	})

	It("reports participant enrichment and missing reconciliation failures", func(ctx SpecContext) {
		item := task.Task{ID: 61, ExternalID: "enrich-error", CourseID: "course-1"}
		source := &nonIncrementalPollingSource{tasks: map[string][]task.Task{"course-1": {item}}, submissions: map[string][]submission.Submission{"enrich-error": {{ExternalID: "submission-1", SubmitterID: "user-1"}}}}
		service := application.NewPollingServiceWithScenarioCollectionAndRosterAndMissing(
			pollingCourseRepository{courses: []classroom.Course{{ExternalID: "course-1", Status: classroom.Active}}}, source, &pollingTaskRepository{}, pollingBindingRepository{bindings: []task.Binding{{Task: item}}}, &pollingSubmissionRepository{}, nil, nil, nil, nil, scenario.Registry{}, &pollingParticipantRepository{items: map[int64][]task.Participant{}, err: errors.New("participant lookup failed")}, nil,
		)
		Expect(service.Refresh(ctx)).To(MatchError(ContainSubstring("list participants for submission")))

		service = application.NewPollingServiceWithScenarioCollectionAndRosterAndMissing(
			pollingCourseRepository{courses: []classroom.Course{{ExternalID: "course-1", Status: classroom.Active}}}, source, &pollingTaskRepository{}, pollingBindingRepository{bindings: []task.Binding{{Task: item}}}, &pollingSubmissionRepository{}, nil, nil, nil, nil, scenario.Registry{}, &pollingParticipantRepository{items: map[int64][]task.Participant{}}, &pollingMissingRepository{items: map[int64][]ports.MissingSubmitter{}, err: errors.New("reconcile failed")},
		)
		Expect(service.Refresh(ctx)).To(MatchError(ContainSubstring("list missing submitters")))
	})

	It("wraps failures while selecting courses, tasks, and submissions", func(ctx SpecContext) {
		item := task.Task{ID: 1, ExternalID: "task-1", CourseID: "course-1"}
		Expect(application.NewPollingService(pollingCourseRepository{err: errors.New("courses failed")}, &pollingSource{}, &pollingTaskRepository{}, pollingBindingRepository{}, &pollingSubmissionRepository{}).Refresh(ctx)).To(MatchError("list courses for polling: courses failed"))
		Expect(application.NewPollingService(pollingCourseRepository{courses: []classroom.Course{{ExternalID: "course-1", Status: classroom.Active}}}, &pollingSource{taskErr: errors.New("tasks failed")}, &pollingTaskRepository{}, pollingBindingRepository{}, &pollingSubmissionRepository{}).Refresh(ctx)).To(MatchError("list tasks for course \"course-1\": tasks failed"))
		Expect(application.NewPollingService(pollingCourseRepository{courses: []classroom.Course{{ExternalID: "course-1", Status: classroom.Active}}}, &pollingSource{tasks: map[string][]task.Task{"course-1": {item}}}, &pollingTaskRepository{err: errors.New("task store failed")}, pollingBindingRepository{}, &pollingSubmissionRepository{}).Refresh(ctx)).To(MatchError("store task \"task-1\": task store failed"))
		source := &pollingSource{submissions: map[string][][]submission.Submission{"task-1": {{{ExternalID: "submission-1"}}}}, subErr: errors.New("submissions failed")}
		Expect(application.NewPollingService(pollingCourseRepository{courses: []classroom.Course{{ExternalID: "course-1", Status: classroom.Active}}}, source, &pollingTaskRepository{}, pollingBindingRepository{bindings: []task.Binding{{Task: item}}}, &pollingSubmissionRepository{}).Refresh(ctx)).To(MatchError("list submissions for task \"task-1\": submissions failed"))
	})

	It("wraps failures while listing active task bindings", func(ctx SpecContext) {
		service := application.NewPollingService(
			pollingCourseRepository{courses: []classroom.Course{{ExternalID: "course-1", Status: classroom.Active}}},
			&pollingSource{}, &pollingTaskRepository{}, pollingBindingRepository{activeErr: errors.New("bindings failed")}, &pollingSubmissionRepository{},
		)

		Expect(service.Refresh(ctx)).To(MatchError("list active task bindings: bindings failed"))
	})

	It("stops polling promptly when its context is canceled", func(ctx SpecContext) {
		pollingCtx, cancel := context.WithCancel(ctx)
		defer cancel()
		item := task.Task{ID: 1, ExternalID: "task-1", CourseID: "course-1"}
		source := &pollingSource{
			tasks:            map[string][]task.Task{"course-1": {item}},
			cancelOnTaskCall: 2,
			cancel:           cancel,
		}
		service := application.NewPollingService(
			pollingCourseRepository{courses: []classroom.Course{{ExternalID: "course-1", Status: classroom.Active}}},
			source, &pollingTaskRepository{}, pollingBindingRepository{}, &pollingSubmissionRepository{},
		)

		started := time.Now()
		Expect(service.Run(pollingCtx, time.Millisecond)).To(Succeed())
		Expect(time.Since(started)).To(BeNumerically("<", time.Second))
		Expect(source.taskCalls).To(HaveLen(2))
	})

	It("loads the previous checkpoint and saves a new checkpoint only after a successful cycle", func(ctx SpecContext) {
		checkpoint := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
		state := &pollingStateRepository{checkpoint: checkpoint}
		item := task.Task{ID: 1, ExternalID: "task-1", CourseID: "course-1"}
		pollingCtx, cancel := context.WithCancel(ctx)
		defer cancel()
		source := &pollingSource{
			tasks:            map[string][]task.Task{"course-1": {item}},
			cancelOnTaskCall: 1,
			cancel:           cancel,
		}
		service := application.NewPersistentPollingService(
			pollingCourseRepository{courses: []classroom.Course{{ExternalID: "course-1", Status: classroom.Active}}},
			source, &pollingTaskRepository{}, pollingBindingRepository{}, &pollingSubmissionRepository{}, state,
		)

		Expect(service.Run(pollingCtx, time.Millisecond)).To(Succeed())
		Expect(state.lastCalls).To(Equal(1))
		Expect(state.saved).To(HaveLen(1))
		Expect(state.saved[0]).To(BeTemporally(">=", checkpoint))
		Expect(source.sinceCalls).To(Equal([]time.Time{checkpoint}))
	})

	It("does not advance the checkpoint when a polling cycle fails", func(ctx SpecContext) {
		state := &pollingStateRepository{checkpoint: time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)}
		service := application.NewPersistentPollingService(
			pollingCourseRepository{courses: []classroom.Course{{ExternalID: "course-1", Status: classroom.Active}}},
			&pollingSource{taskErr: errors.New("provider unavailable")}, &pollingTaskRepository{}, pollingBindingRepository{}, &pollingSubmissionRepository{}, state,
		)

		Expect(service.Run(ctx, time.Millisecond)).To(MatchError(ContainSubstring("list tasks for course \"course-1\": provider unavailable")))
		Expect(state.lastCalls).To(Equal(1))
		Expect(state.saved).To(BeEmpty())
	})

	It("rejects a zero polling interval before refreshing", func(ctx SpecContext) {
		service := application.NewPollingService(
			pollingCourseRepository{err: errors.New("refresh must not run")},
			&pollingSource{}, &pollingTaskRepository{}, pollingBindingRepository{}, &pollingSubmissionRepository{},
		)

		Expect(service.Run(ctx, 0)).To(MatchError("polling interval must be greater than zero: 0s"))
	})

	It("rejects a negative polling interval before refreshing", func(ctx SpecContext) {
		service := application.NewPollingService(
			pollingCourseRepository{err: errors.New("refresh must not run")},
			&pollingSource{}, &pollingTaskRepository{}, pollingBindingRepository{}, &pollingSubmissionRepository{},
		)

		Expect(service.Run(ctx, -time.Second)).To(MatchError("polling interval must be greater than zero: -1s"))
	})

	It("propagates checkpoint and save errors", func(ctx SpecContext) {
		state := &pollingStateRepository{lastErr: errors.New("checkpoint failed")}
		service := application.NewPersistentPollingService(
			pollingCourseRepository{}, &pollingSource{}, &pollingTaskRepository{}, pollingBindingRepository{}, &pollingSubmissionRepository{}, state,
		)
		Expect(service.Run(ctx, time.Millisecond)).To(MatchError("checkpoint failed"))

		state = &pollingStateRepository{saveErr: errors.New("save failed")}
		service = application.NewPersistentPollingService(
			pollingCourseRepository{}, &pollingSource{}, &pollingTaskRepository{}, pollingBindingRepository{}, &pollingSubmissionRepository{}, state,
		)
		Expect(service.Run(ctx, time.Millisecond)).To(MatchError("save failed"))
	})

	It("returns a checkpoint error from a later polling tick", func(ctx SpecContext) {
		state := &pollingStateRepository{lastErr: errors.New("second checkpoint failed"), lastErrAfter: 1}
		pollingCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		service := application.NewPersistentPollingService(
			pollingCourseRepository{}, &pollingSource{}, &pollingTaskRepository{}, pollingBindingRepository{}, &pollingSubmissionRepository{}, state,
		)
		Expect(service.Run(pollingCtx, time.Millisecond)).To(MatchError("second checkpoint failed"))
		Expect(state.lastCalls).To(Equal(2))
	})
})
