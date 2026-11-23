package application_test

import (
	"context"
	"errors"
	"time"

	"github.com/alex/assistent/internal/application"
	"github.com/alex/assistent/internal/domain/classroom"
	"github.com/alex/assistent/internal/domain/report"
	"github.com/alex/assistent/internal/domain/scenario"
	"github.com/alex/assistent/internal/domain/submission"
	"github.com/alex/assistent/internal/domain/task"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type pollingCollectionRunRepository struct {
	run report.Run
}

func (r *pollingCollectionRunRepository) Create(_ context.Context, run *report.Run) error {
	run.ID = 1
	r.run = *run
	return nil
}
func (r *pollingCollectionRunRepository) Update(_ context.Context, run *report.Run) error {
	r.run = *run
	return nil
}
func (r *pollingCollectionRunRepository) Get(context.Context, int64) (report.Run, error) {
	return r.run, nil
}

type pollingCollectionProcessor struct{}

func (pollingCollectionProcessor) Process(collection scenario.Collection) (scenario.ProcessResult, error) {
	if len(collection.Items) != 1 || collection.Items[0].State != submission.Submitted {
		return scenario.ProcessResult{}, errors.New("unexpected collection")
	}
	return scenario.ProcessResult{Outcome: scenario.Completed}, nil
}

var _ = Describe("polling scenario collection", func() {
	It("processes a bound scenario collection", func() {
		item := task.Task{ID: 8, ExternalID: "task-8", CourseID: "course-1"}
		source := &pollingSource{
			tasks:       map[string][]task.Task{"course-1": {item}},
			submissions: map[string][][]submission.Submission{"task-8": {{{ExternalID: "submission-8", State: submission.Submitted}}}},
		}
		submissions := &pollingSubmissionRepository{}
		runs := &pollingCollectionRunRepository{}
		collection := application.NewScenarioCollectionService(submissions, runs)
		registry := scenario.NewRegistryWithProcessors(
			[]scenario.Implementation{{Name: "science-report"}},
			map[string]scenario.CollectionProcessor{"science-report": pollingCollectionProcessor{}},
		)
		service := application.NewPollingServiceWithScenarioCollection(
			pollingCourseRepository{courses: []classroom.Course{{ExternalID: "course-1", Status: classroom.Active}}},
			source, &pollingTaskRepository{}, pollingBindingRepository{bindings: []task.Binding{{ScenarioName: "science-report", ScenarioID: 3, Task: item}}},
			submissions, &pollingStateRepository{}, nil, nil, collection, registry,
		)
		Expect(service.Refresh(context.Background())).NotTo(HaveOccurred())
		Expect(runs.run.Status).To(Equal(report.Completed))
	})

	It("waits for the three-day preliminary phase", func() {
		deadline := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
		item := task.Task{ID: 9, ExternalID: "deadline-task", CourseID: "course-1", DueAt: deadline.Format(time.RFC3339), Phase: task.PhaseDiscovery}
		source := &pollingSource{
			tasks:       map[string][]task.Task{"course-1": {item}},
			submissions: map[string][][]submission.Submission{"deadline-task": {{{ExternalID: "submission-1", State: submission.Submitted}}}},
		}
		submissions := &pollingSubmissionRepository{}
		runs := &pollingCollectionRunRepository{}
		collection := application.NewScenarioCollectionService(submissions, runs)
		registry := scenario.NewRegistryWithProcessors([]scenario.Implementation{{Name: "science-report"}}, map[string]scenario.CollectionProcessor{"science-report": pollingCollectionProcessor{}})
		bindings := &pollingBindingRepository{bindings: []task.Binding{{ScenarioName: "science-report", ScenarioID: 3, Task: item}}}
		service := application.NewPollingServiceWithScenarioCollection(pollingCourseRepository{courses: []classroom.Course{{ExternalID: "course-1", Status: classroom.Active}}}, source, &pollingTaskRepository{}, bindings, submissions, &pollingStateRepository{}, nil, nil, collection, registry)
		current := deadline.Add(-72*time.Hour - time.Minute)
		service.SetClock(func() time.Time { return current })
		Expect(service.Refresh(context.Background())).NotTo(HaveOccurred())
		Expect(runs.run.ID).To(BeZero())
		current = deadline.Add(-72 * time.Hour)
		item.Phase = task.PhaseThreeDaysBefore
		source.tasks["course-1"] = []task.Task{item}
		bindings.bindings[0].Task = item
		Expect(service.Refresh(context.Background())).NotTo(HaveOccurred())
		Expect(runs.run.Status).To(Equal(report.Completed))
	})

	It("reports a missing scenario processor", func() {
		item := task.Task{ID: 8, ExternalID: "task-8", CourseID: "course-1"}
		service := application.NewPollingServiceWithScenarioCollection(
			pollingCourseRepository{courses: []classroom.Course{{ExternalID: "course-1", Status: classroom.Active}}},
			&pollingSource{tasks: map[string][]task.Task{"course-1": {item}}}, &pollingTaskRepository{},
			pollingBindingRepository{bindings: []task.Binding{{ScenarioName: "missing", Task: item}}}, &pollingSubmissionRepository{},
			&pollingStateRepository{}, nil, nil, application.NewScenarioCollectionService(&pollingSubmissionRepository{}, &pollingCollectionRunRepository{}), scenario.NewRegistry(),
		)
		Expect(service.Refresh(context.Background())).To(MatchError(ContainSubstring("resolve collection processor")))
	})

	It("reports course lookup failures", func() {
		service := application.NewPersistentPollingService(
			pollingCourseRepository{err: errors.New("course lookup failed")}, &pollingSource{}, &pollingTaskRepository{}, pollingBindingRepository{}, &pollingSubmissionRepository{}, &pollingStateRepository{},
		)
		Expect(service.Run(context.Background(), time.Millisecond)).To(MatchError(ContainSubstring("list courses for polling: course lookup failed")))
	})
})
