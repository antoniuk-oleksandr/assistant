package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alex/assistent/internal/application"
	"github.com/alex/assistent/internal/domain/ports"
	"github.com/alex/assistent/internal/domain/report"
	"github.com/alex/assistent/internal/domain/scenario"
	"github.com/alex/assistent/internal/domain/submission"
)

type collectionSubmissionRepository struct {
	items []submission.Submission
	err   error
}

func (r collectionSubmissionRepository) Upsert(context.Context, submission.Submission) error {
	return nil
}

func (r collectionSubmissionRepository) ListByTask(context.Context, int64) ([]submission.Submission, error) {
	return r.items, r.err
}

type collectionRunRepository struct {
	run       report.Run
	existing  *report.Run
	getErr    error
	createErr error
	updateErr error
	updates   []report.Run
	updateAt  int
	failAt    int
}

func (r *collectionRunRepository) Create(_ context.Context, run *report.Run) error {
	if r.createErr != nil {
		return r.createErr
	}
	run.ID = 42
	r.run = *run
	return nil
}

func (r *collectionRunRepository) Update(_ context.Context, run *report.Run) error {
	r.updateAt++
	if r.updateErr != nil && (r.failAt == 0 || r.updateAt == r.failAt) {
		return r.updateErr
	}
	r.updates = append(r.updates, *run)
	r.run = *run
	return nil
}

func (r *collectionRunRepository) Get(context.Context, int64) (report.Run, error) { return r.run, nil }
func (r *collectionRunRepository) GetByScenarioTask(context.Context, int64, int64) (report.Run, error) {
	if r.getErr != nil {
		return report.Run{}, r.getErr
	}
	if r.existing == nil {
		return report.Run{}, ports.ErrReportRunNotFound
	}
	return *r.existing, nil
}

type collectionProcessor struct {
	result scenario.ProcessResult
	err    error
	input  scenario.Collection
}

type collectionCourseDataRepository struct{ err error }

func (r collectionCourseDataRepository) Set(context.Context, string, string, map[string]string) error {
	return r.err
}
func (r collectionCourseDataRepository) List(context.Context, string, string) ([]scenario.CourseDataField, error) {
	if r.err != nil {
		return nil, r.err
	}
	return []scenario.CourseDataField{{Name: "department_name", Value: "Science"}}, nil
}

func (p *collectionProcessor) Process(input scenario.Collection) (scenario.ProcessResult, error) {
	p.input = input
	return p.result, p.err
}

func TestScenarioCollectionServiceProcessesAndPersistsLifecycle(t *testing.T) {
	runs := &collectionRunRepository{}
	processor := &collectionProcessor{result: scenario.ProcessResult{Outcome: scenario.Completed, ResultPath: "result.json"}}
	service := application.NewScenarioCollectionService(collectionSubmissionRepository{items: []submission.Submission{{ExternalID: "submission-1"}}}, runs)

	run, err := service.Process(context.Background(), 3, 7, processor)
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}
	if run.Status != report.Completed || run.ResultPath != "result.json" || len(runs.updates) != 3 {
		t.Fatalf("run = %#v, updates = %#v", run, runs.updates)
	}
	if processor.input.ScenarioID != 3 || processor.input.TaskID != 7 || len(processor.input.Items) != 1 {
		t.Fatalf("processor input = %#v", processor.input)
	}
}

func TestScenarioCollectionServicePersistsFailures(t *testing.T) {
	tests := []struct {
		name       string
		processor  scenario.CollectionProcessor
		submission collectionSubmissionRepository
		run        *collectionRunRepository
		want       string
	}{
		{"nil processor", nil, collectionSubmissionRepository{}, &collectionRunRepository{}, "nil collection processor"},
		{"nil processor persistence error", nil, collectionSubmissionRepository{}, &collectionRunRepository{updateErr: errors.New("failed update")}, "nil collection processor; persist failed report run: failed update"},
		{"submission error", &collectionProcessor{}, collectionSubmissionRepository{err: errors.New("read failed")}, &collectionRunRepository{}, "collect submissions for task 7: read failed"},
		{"processor error", &collectionProcessor{err: errors.New("rule failed")}, collectionSubmissionRepository{}, &collectionRunRepository{}, "process scenario collection: rule failed"},
		{"create error", &collectionProcessor{}, collectionSubmissionRepository{}, &collectionRunRepository{createErr: errors.New("insert failed")}, "persist report run: insert failed"},
		{"collecting update error", &collectionProcessor{}, collectionSubmissionRepository{}, &collectionRunRepository{updateErr: errors.New("update failed")}, "persist collecting report run: update failed"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			service := application.NewScenarioCollectionService(tc.submission, tc.run)
			_, err := service.Process(context.Background(), 3, 7, tc.processor)
			if err == nil || err.Error() != tc.want && !contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
			if tc.run.createErr == nil && tc.run.updateErr == nil && len(tc.run.updates) == 0 {
				t.Fatalf("failure was not persisted: %#v", tc.run)
			}
		})
	}
}

func TestScenarioCollectionServiceReportsTerminalUpdateFailure(t *testing.T) {
	runs := &collectionRunRepository{updateErr: errors.New("terminal update failed"), failAt: 3}
	processor := &collectionProcessor{result: scenario.ProcessResult{Outcome: scenario.Completed}}
	service := application.NewScenarioCollectionService(collectionSubmissionRepository{}, runs)
	_, err := service.Process(context.Background(), 1, 2, processor)
	if err == nil || !contains(err.Error(), "persist terminal report run: terminal update failed") {
		t.Fatalf("error = %v", err)
	}
}

func TestScenarioCollectionServiceDoesNotReprocessExistingTerminalRun(t *testing.T) {
	existing := report.NewRun(3, 7, time.Now())
	if err := existing.Transition(report.Collecting, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := existing.Transition(report.Processing, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := existing.ApplyResult(scenario.ProcessResult{Outcome: scenario.Completed, ResultPath: "already-generated.json"}, time.Now()); err != nil {
		t.Fatal(err)
	}
	runs := &collectionRunRepository{existing: &existing}
	processor := &collectionProcessor{result: scenario.ProcessResult{Outcome: scenario.Completed}}
	service := application.NewScenarioCollectionService(collectionSubmissionRepository{}, runs)
	got, err := service.Process(context.Background(), 3, 7, processor)
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}
	if got != existing || len(runs.updates) != 0 || processor.input.Items != nil {
		t.Fatalf("existing run was reprocessed: got=%#v updates=%#v input=%#v", got, runs.updates, processor.input)
	}
}

func TestScenarioCollectionServiceReportsExistingRunLookupFailure(t *testing.T) {
	runs := &collectionRunRepository{getErr: errors.New("lookup failed")}
	service := application.NewScenarioCollectionService(collectionSubmissionRepository{}, runs)
	_, err := service.Process(context.Background(), 3, 7, &collectionProcessor{})
	if err == nil || !contains(err.Error(), "find report run: lookup failed") {
		t.Fatalf("Process() error = %v", err)
	}
}

func TestScenarioCollectionServiceReportsForcedExistingRunLookupFailure(t *testing.T) {
	runs := &collectionRunRepository{getErr: errors.New("forced lookup failed")}
	service := application.NewScenarioCollectionService(collectionSubmissionRepository{}, runs)
	_, err := service.ProcessForTaskUpdate(context.Background(), 3, 7, "science-report", "course-1", &collectionProcessor{})
	if err == nil || !contains(err.Error(), "find report run: forced lookup failed") {
		t.Fatalf("forced Process() error = %v", err)
	}
}

func TestScenarioCollectionServiceReportsCourseDataFailure(t *testing.T) {
	service := application.NewScenarioCollectionServiceWithRendererResultAndCourseData(
		collectionSubmissionRepository{}, &collectionRunRepository{}, nil, nil, nil, collectionCourseDataRepository{err: errors.New("course data failed")},
	)
	processor := &collectionProcessor{result: scenario.ProcessResult{Outcome: scenario.Completed, ReportData: &scenario.ReportData{}}}
	_, err := service.ProcessForTask(context.Background(), 3, 7, "science-report", "course-1", processor)
	if err == nil || !contains(err.Error(), "load course report data: course data failed") {
		t.Fatalf("course data error = %v", err)
	}
}

func TestScenarioCollectionServiceHandlesProcessingUpdateAndInvalidOutcome(t *testing.T) {
	processingFailure := &collectionRunRepository{updateErr: errors.New("processing update failed"), failAt: 2}
	service := application.NewScenarioCollectionService(collectionSubmissionRepository{}, processingFailure)
	_, err := service.Process(context.Background(), 1, 2, &collectionProcessor{result: scenario.ProcessResult{Outcome: scenario.Completed}})
	if err == nil || !contains(err.Error(), "persist processing report run: processing update failed") {
		t.Fatalf("processing update error = %v", err)
	}

	runs := &collectionRunRepository{}
	service = application.NewScenarioCollectionService(collectionSubmissionRepository{}, runs)
	_, err = service.Process(context.Background(), 1, 2, &collectionProcessor{result: scenario.ProcessResult{Outcome: "invalid"}})
	if err == nil || !contains(err.Error(), "InvalidReportOutcome") {
		t.Fatalf("invalid outcome error = %v", err)
	}
}

func contains(value, fragment string) bool {
	for i := 0; i+len(fragment) <= len(value); i++ {
		if value[i:i+len(fragment)] == fragment {
			return true
		}
	}
	return false
}
