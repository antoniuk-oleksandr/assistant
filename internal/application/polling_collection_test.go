package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alex/assistent/internal/application"
	"github.com/alex/assistent/internal/domain/classroom"
	"github.com/alex/assistent/internal/domain/report"
	"github.com/alex/assistent/internal/domain/scenario"
	"github.com/alex/assistent/internal/domain/submission"
	"github.com/alex/assistent/internal/domain/task"
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

func TestPollingProcessesBoundScenarioCollection(t *testing.T) {
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
	if err := service.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}
	if runs.run.Status != report.Completed {
		t.Fatalf("report run = %#v", runs.run)
	}
}

func TestPollingWaitsForTheThreeDayPreliminaryPhase(t *testing.T) {
	item := task.Task{ID: 9, ExternalID: "deadline-task", CourseID: "course-1", DueAt: "2026-11-10T12:00:00Z", Phase: task.PhaseDiscovery}
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
	if err := service.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if runs.run.ID != 0 {
		t.Fatalf("preliminary report created during discovery: %#v", runs.run)
	}
	item.Phase = task.PhaseThreeDaysBefore
	source.tasks["course-1"] = []task.Task{item}
	bindings.bindings[0].Task = item
	if err := service.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if runs.run.Status != report.Completed {
		t.Fatalf("preliminary report = %#v", runs.run)
	}
}

func TestPollingReportsMissingScenarioProcessor(t *testing.T) {
	item := task.Task{ID: 8, ExternalID: "task-8", CourseID: "course-1"}
	service := application.NewPollingServiceWithScenarioCollection(
		pollingCourseRepository{courses: []classroom.Course{{ExternalID: "course-1", Status: classroom.Active}}},
		&pollingSource{tasks: map[string][]task.Task{"course-1": {item}}}, &pollingTaskRepository{},
		pollingBindingRepository{bindings: []task.Binding{{ScenarioName: "missing", Task: item}}}, &pollingSubmissionRepository{},
		&pollingStateRepository{}, nil, nil, application.NewScenarioCollectionService(&pollingSubmissionRepository{}, &pollingCollectionRunRepository{}), scenario.NewRegistry(),
	)
	if err := service.Refresh(context.Background()); err == nil || !contains(err.Error(), "resolve collection processor") {
		t.Fatalf("Refresh() error = %v", err)
	}
}

func TestPollingRunReportsCourseLookupFailure(t *testing.T) {
	service := application.NewPersistentPollingService(
		pollingCourseRepository{err: errors.New("course lookup failed")}, &pollingSource{}, &pollingTaskRepository{}, pollingBindingRepository{}, &pollingSubmissionRepository{}, &pollingStateRepository{},
	)
	if err := service.Run(context.Background(), time.Hour); err == nil || !contains(err.Error(), "list courses for polling: course lookup failed") {
		t.Fatalf("Run() error = %v", err)
	}
}
