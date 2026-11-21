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

type resultRepository struct {
	value     report.Result
	createErr error
	updateErr error
}

func (r *resultRepository) Create(_ context.Context, value *report.Result) error {
	if r.createErr != nil {
		return r.createErr
	}
	value.ID = 9
	r.value = *value
	return nil
}
func (r *resultRepository) Update(_ context.Context, value *report.Result) error {
	if r.updateErr != nil {
		return r.updateErr
	}
	r.value = *value
	return nil
}
func (r *resultRepository) GetLatestByRun(context.Context, int64) (report.Result, error) {
	return r.value, nil
}

type resultRenderer struct{ err error }

func (r resultRenderer) RenderAndSave(context.Context, scenario.ReportData) (report.Output, error) {
	if r.err != nil {
		return report.Output{}, r.err
	}
	return report.Output{Format: "test", Path: "result.json", Version: 1, CreatedAt: time.Now().UTC()}, nil
}

type resultOutputRepository struct{ err error }

func (r resultOutputRepository) Create(context.Context, ports.ReportOutput) error { return r.err }

func TestScenarioCollectionServicePersistsPartialResult(t *testing.T) {
	runs := &collectionRunRepository{}
	results := &resultRepository{}
	service := application.NewScenarioCollectionServiceWithRendererAndResult(collectionSubmissionRepository{}, runs, resultRenderer{}, resultOutputRepository{}, results)
	processor := &collectionProcessor{result: scenario.ProcessResult{Outcome: scenario.Partial, ReportData: &scenario.ReportData{ScenarioID: 3, TaskID: 7}}}
	got, err := service.Process(context.Background(), 3, 7, processor)
	if err != nil || got.Status != report.Completed || results.value.Status != report.ResultPartial || !results.value.Partial {
		t.Fatalf("run=%#v result=%#v err=%v", got, results.value, err)
	}
}

func TestScenarioCollectionServiceProcessesAnIncrementalUpdateAsANewRun(t *testing.T) {
	runs := &collectionRunRepository{}
	service := application.NewScenarioCollectionService(collectionSubmissionRepository{items: []submission.Submission{}}, runs)
	processor := &collectionProcessor{result: scenario.ProcessResult{Outcome: scenario.Completed}}
	first, err := service.ProcessForTask(context.Background(), 3, 7, "science-report", "course-1", processor)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.ProcessForTaskUpdate(context.Background(), 3, 7, "science-report", "course-1", processor)
	if err != nil || second.ID != first.ID || second.Status != report.Completed {
		t.Fatalf("first=%#v second=%#v err=%v", first, second, err)
	}
	if len(runs.updates) == 0 {
		t.Fatal("incremental run was not persisted")
	}
}

func TestScenarioCollectionServiceSkipsUnchangedIncrementalUpdate(t *testing.T) {
	runs := &collectionRunRepository{}
	service := application.NewScenarioCollectionService(collectionSubmissionRepository{items: []submission.Submission{}}, runs)
	processor := &collectionProcessor{result: scenario.ProcessResult{Outcome: scenario.Completed}}
	first, err := service.ProcessForTask(context.Background(), 3, 7, "science-report", "course-1", processor)
	if err != nil {
		t.Fatal(err)
	}
	runs.existing = &first
	second, err := service.ProcessForTaskUpdate(context.Background(), 3, 7, "science-report", "course-1", processor)
	if err != nil || second.ID != first.ID {
		t.Fatalf("first=%#v second=%#v err=%v", first, second, err)
	}
}

func TestScenarioCollectionServiceProcessesChangedIncrementalUpdate(t *testing.T) {
	runs := &collectionRunRepository{existing: &report.Run{ID: 9, InputFingerprint: "old", Status: report.Completed}}
	service := application.NewScenarioCollectionService(collectionSubmissionRepository{items: []submission.Submission{{ExternalID: "new-submission"}}}, runs)
	processor := &collectionProcessor{result: scenario.ProcessResult{Outcome: scenario.Completed}}
	got, err := service.ProcessForTaskUpdate(context.Background(), 3, 7, "science-report", "course-1", processor)
	if err != nil || got.ID != 42 {
		t.Fatalf("run=%#v err=%v", got, err)
	}
}

func TestScenarioCollectionServiceReportsIncrementalSubmissionLookupFailure(t *testing.T) {
	service := application.NewScenarioCollectionService(collectionSubmissionRepository{err: errors.New("submission lookup failed")}, &collectionRunRepository{})
	_, err := service.ProcessForTaskUpdate(context.Background(), 3, 7, "science-report", "course-1", &collectionProcessor{result: scenario.ProcessResult{Outcome: scenario.Completed}})
	if err == nil || !contains(err.Error(), "collect submissions for task 7") {
		t.Fatalf("error = %v", err)
	}
}

func TestScenarioCollectionServicePersistsFailedResultAndReportsPersistenceError(t *testing.T) {
	runs := &collectionRunRepository{}
	results := &resultRepository{}
	service := application.NewScenarioCollectionServiceWithRendererAndResult(collectionSubmissionRepository{}, runs, resultRenderer{}, resultOutputRepository{}, results)
	processor := &collectionProcessor{result: scenario.ProcessResult{Outcome: scenario.Failed, ReportData: &scenario.ReportData{}}}
	got, err := service.Process(context.Background(), 3, 7, processor)
	if err != nil || got.Status != report.Failed || results.value.Status != report.ResultFailed {
		t.Fatalf("run=%#v result=%#v err=%v", got, results.value, err)
	}
	failing := &resultRepository{createErr: errors.New("result insert failed")}
	service = application.NewScenarioCollectionServiceWithRendererAndResult(collectionSubmissionRepository{}, &collectionRunRepository{}, resultRenderer{}, resultOutputRepository{}, failing)
	_, err = service.Process(context.Background(), 3, 7, &collectionProcessor{result: scenario.ProcessResult{Outcome: scenario.Completed, ReportData: &scenario.ReportData{}}})
	if err == nil || !contains(err.Error(), "persist processing result") {
		t.Fatalf("error=%v", err)
	}
}

func TestScenarioCollectionServiceReportsRendererAndOutputErrors(t *testing.T) {
	processor := &collectionProcessor{result: scenario.ProcessResult{Outcome: scenario.Completed, ReportData: &scenario.ReportData{}}}
	service := application.NewScenarioCollectionServiceWithRendererAndResult(collectionSubmissionRepository{}, &collectionRunRepository{}, resultRenderer{err: errors.New("render failed")}, resultOutputRepository{}, &resultRepository{})
	_, err := service.Process(context.Background(), 3, 7, processor)
	if err == nil || !contains(err.Error(), "render report package") {
		t.Fatalf("error=%v", err)
	}
	service = application.NewScenarioCollectionServiceWithRendererAndResult(collectionSubmissionRepository{}, &collectionRunRepository{}, resultRenderer{}, resultOutputRepository{err: errors.New("output failed")}, &resultRepository{})
	_, err = service.Process(context.Background(), 3, 7, processor)
	if err == nil || !contains(err.Error(), "persist report output") {
		t.Fatalf("error=%v", err)
	}
}

var _ ports.ReportResultRepository = (*resultRepository)(nil)
var _ ports.ReportRenderer = resultRenderer{}
var _ ports.ReportOutputRepository = resultOutputRepository{}
var _ ports.SubmissionRepository = collectionSubmissionRepository{}
