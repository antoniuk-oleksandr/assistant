package application_test

import (
	"context"
	"errors"
	"time"

	"github.com/alex/assistent/internal/application"
	"github.com/alex/assistent/internal/domain/classroom"
	"github.com/alex/assistent/internal/domain/ports"
	"github.com/alex/assistent/internal/domain/submission"
	"github.com/alex/assistent/internal/domain/task"
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

type pollingTaskRepository struct {
	registered []task.Task
	err        error
}

func (r *pollingTaskRepository) Register(_ context.Context, item task.Task) error {
	if r.err != nil {
		return r.err
	}
	r.registered = append(r.registered, item)
	return nil
}
func (r *pollingTaskRepository) List(context.Context) ([]task.Task, error) { return r.registered, nil }

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
	It("supports sources without incremental polling", func(ctx SpecContext) {
		item := task.Task{ID: 31, ExternalID: "task-1", CourseID: "course-1"}
		source := &nonIncrementalPollingSource{tasks: map[string][]task.Task{"course-1": {item}}}
		tasks := &pollingTaskRepository{}
		service := application.NewPollingService(
			pollingCourseRepository{courses: []classroom.Course{{ExternalID: "course-1", Status: classroom.Active}}},
			source, tasks, pollingBindingRepository{}, &pollingSubmissionRepository{},
		)
		Expect(service.Refresh(ctx)).To(Succeed())
		Expect(tasks.registered).To(ConsistOf(item))
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
		Expect(tasks.registered).To(ConsistOf(activeTask, inactiveTask))
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
		Expect(tasks.registered).To(HaveLen(2))
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

		Expect(service.Run(pollingCtx, time.Hour)).To(Succeed())
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

		Expect(service.Run(ctx, time.Hour)).To(MatchError(ContainSubstring("list tasks for course \"course-1\": provider unavailable")))
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
		Expect(service.Run(ctx, time.Hour)).To(MatchError("checkpoint failed"))

		state = &pollingStateRepository{saveErr: errors.New("save failed")}
		service = application.NewPersistentPollingService(
			pollingCourseRepository{}, &pollingSource{}, &pollingTaskRepository{}, pollingBindingRepository{}, &pollingSubmissionRepository{}, state,
		)
		Expect(service.Run(ctx, time.Hour)).To(MatchError("save failed"))
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
