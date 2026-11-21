package application_test

import (
	"context"
	"errors"
	"time"

	"github.com/alex/assistent/internal/application"
	"github.com/alex/assistent/internal/domain/classroom"
	"github.com/alex/assistent/internal/domain/submission"
	"github.com/alex/assistent/internal/domain/task"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type classroomRepositoryStub struct {
	courses []classroom.Course
	err     error
	calls   []string
}

type taskRepositoryStub struct {
	tasks       []task.Task
	err         error
	registerErr error
}

func (r *taskRepositoryStub) Register(context.Context, task.Task) error { return r.registerErr }
func (r *taskRepositoryStub) List(context.Context) ([]task.Task, error) { return r.tasks, r.err }

type taskSourceStub struct {
	tasks []task.Task
	err   error
}

func (s *taskSourceStub) ListTasks(context.Context, string) ([]task.Task, error) {
	return s.tasks, s.err
}
func (s *taskSourceStub) ListSubmissions(context.Context, string) ([]submission.Submission, error) {
	return nil, nil
}

func (r *classroomRepositoryStub) Register(_ context.Context, course classroom.Course) error {
	r.calls = append(r.calls, "register:"+course.ExternalID)
	return r.err
}
func (r *classroomRepositoryStub) List(context.Context) ([]classroom.Course, error) {
	r.calls = append(r.calls, "list")
	return r.courses, r.err
}
func (r *classroomRepositoryStub) Disable(_ context.Context, externalID string) error {
	r.calls = append(r.calls, "disable:"+externalID)
	return r.err
}

var _ = Describe("Classroom service", func() {
	It("normalizes references before registering them", func(ctx SpecContext) {
		repository := &classroomRepositoryStub{}
		service := application.NewClassroomService(repository)

		Expect(service.Add(ctx, "https://classroom.google.com/c/OTAwNDIzOTQ1Mjk3")).To(Succeed())
		Expect(repository.calls).To(Equal([]string{"register:900423945297"}))
	})

	It("returns validation errors without calling the repository", func(ctx SpecContext) {
		repository := &classroomRepositoryStub{}
		service := application.NewClassroomService(repository)

		Expect(service.Add(ctx, "not a course")).To(MatchError(ContainSubstring("InvalidClassroomCourseReference")))
		Expect(service.Remove(ctx, "not a course")).To(MatchError(ContainSubstring("InvalidClassroomCourseReference")))
		Expect(repository.calls).To(BeEmpty())
	})

	It("maps listed domain courses into application results", func(ctx SpecContext) {
		created := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
		updated := created.Add(time.Hour)
		repository := &classroomRepositoryStub{courses: []classroom.Course{{
			ExternalID: "123", SourceReference: "123", Status: classroom.Disabled,
			CreatedAt: created, UpdatedAt: updated,
		}}}
		service := application.NewClassroomService(repository)

		result, err := service.List(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal([]application.ClassroomCourseResult{{ExternalID: "123", SourceReference: "123", Status: "disabled", CreatedAt: created, UpdatedAt: updated}}))
		Expect(repository.calls).To(Equal([]string{"list"}))
	})

	It("propagates repository errors for each operation", func(ctx SpecContext) {
		repository := &classroomRepositoryStub{err: errors.New("repository failed")}
		service := application.NewClassroomService(repository)

		Expect(service.Add(ctx, "123")).To(MatchError("repository failed"))
		Expect(service.List(ctx)).Error().To(MatchError("repository failed"))
		Expect(service.Remove(ctx, "123")).To(MatchError("repository failed"))
	})

	It("lists only tasks belonging to the requested course", func(ctx SpecContext) {
		repository := &taskRepositoryStub{tasks: []task.Task{
			{ExternalID: "task-1", CourseID: "123", Title: "Report", Status: task.Discovered},
			{ExternalID: "task-2", CourseID: "456", Title: "Other", Status: task.Bound},
		}}
		source := &taskSourceStub{tasks: []task.Task{repository.tasks[0]}}
		service := application.NewCourseTaskService(repository, source)

		result, err := service.List(ctx, "123")
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal([]application.CourseTaskResult{{ExternalID: "task-1", CourseID: "123", Title: "Report", Status: "discovered"}}))
		Expect(service.List(ctx, "invalid")).Error().To(MatchError(ContainSubstring("InvalidClassroomCourseReference")))

		repository.registerErr = errors.New("task repository failed")
		source.err = nil
		Expect(service.List(ctx, "123")).Error().To(MatchError("task repository failed"))
		source.err = errors.New("Classroom API failed")
		repository.registerErr = nil
		Expect(service.List(ctx, "123")).Error().To(MatchError("Classroom API failed"))
	})
})
