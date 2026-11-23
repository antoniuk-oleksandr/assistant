package application_test

import (
	"context"
	"errors"
	"time"

	"github.com/alex/assistent/internal/application"
	"github.com/alex/assistent/internal/domain/ports"
	"github.com/alex/assistent/internal/domain/report"
	"github.com/alex/assistent/internal/domain/scenario"
	"github.com/alex/assistent/internal/domain/submission"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
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

var _ = Describe("scenario collection service", func() {
	It("processes and persists the lifecycle", func() {
		runs := &collectionRunRepository{}
		processor := &collectionProcessor{result: scenario.ProcessResult{Outcome: scenario.Completed, ResultPath: "result.json"}}
		service := application.NewScenarioCollectionService(collectionSubmissionRepository{items: []submission.Submission{{ExternalID: "submission-1"}}}, runs)

		run, err := service.Process(context.Background(), 3, 7, processor)
		Expect(err).NotTo(HaveOccurred())
		Expect(run.Status).To(Equal(report.Completed))
		Expect(run.ResultPath).To(Equal("result.json"))
		Expect(runs.updates).To(HaveLen(3))
		Expect(processor.input.ScenarioID).To(Equal(int64(3)))
		Expect(processor.input.TaskID).To(Equal(int64(7)))
		Expect(processor.input.Items).To(HaveLen(1))
	})

	It("persists failures", func() {
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
			By(tc.name)
			service := application.NewScenarioCollectionService(tc.submission, tc.run)
			_, err := service.Process(context.Background(), 3, 7, tc.processor)
			Expect(err).To(MatchError(ContainSubstring(tc.want)))
			if tc.run.createErr == nil && tc.run.updateErr == nil {
				Expect(tc.run.updates).NotTo(BeEmpty())
			}
		}
	})

	It("reports terminal update failures", func() {
		runs := &collectionRunRepository{updateErr: errors.New("terminal update failed"), failAt: 3}
		processor := &collectionProcessor{result: scenario.ProcessResult{Outcome: scenario.Completed}}
		service := application.NewScenarioCollectionService(collectionSubmissionRepository{}, runs)
		_, err := service.Process(context.Background(), 1, 2, processor)
		Expect(err).To(MatchError(ContainSubstring("persist terminal report run: terminal update failed")))
	})

	It("does not reprocess an existing terminal run", func() {
		existing := report.NewRun(3, 7, time.Now())
		Expect(existing.Transition(report.Collecting, time.Now())).To(Succeed())
		Expect(existing.Transition(report.Processing, time.Now())).To(Succeed())
		Expect(existing.ApplyResult(scenario.ProcessResult{Outcome: scenario.Completed, ResultPath: "already-generated.json"}, time.Now())).To(Succeed())
		runs := &collectionRunRepository{existing: &existing}
		processor := &collectionProcessor{result: scenario.ProcessResult{Outcome: scenario.Completed}}
		service := application.NewScenarioCollectionService(collectionSubmissionRepository{}, runs)
		got, err := service.Process(context.Background(), 3, 7, processor)
		Expect(err).NotTo(HaveOccurred())
		Expect(got).To(Equal(existing))
		Expect(runs.updates).To(BeEmpty())
		Expect(processor.input.Items).To(BeNil())
	})

	It("reports existing run lookup failures", func() {
		runs := &collectionRunRepository{getErr: errors.New("lookup failed")}
		service := application.NewScenarioCollectionService(collectionSubmissionRepository{}, runs)
		_, err := service.Process(context.Background(), 3, 7, &collectionProcessor{})
		Expect(err).To(MatchError(ContainSubstring("find report run: lookup failed")))
	})

	It("reports forced existing run lookup failures", func() {
		runs := &collectionRunRepository{getErr: errors.New("forced lookup failed")}
		service := application.NewScenarioCollectionService(collectionSubmissionRepository{}, runs)
		_, err := service.ProcessForTaskUpdate(context.Background(), 3, 7, "science-report", "course-1", &collectionProcessor{})
		Expect(err).To(MatchError(ContainSubstring("find report run: forced lookup failed")))
	})

	It("reports course data failures", func() {
		service := application.NewScenarioCollectionServiceWithRendererResultAndCourseData(
			collectionSubmissionRepository{}, &collectionRunRepository{}, nil, nil, nil, collectionCourseDataRepository{err: errors.New("course data failed")},
		)
		processor := &collectionProcessor{result: scenario.ProcessResult{Outcome: scenario.Completed, ReportData: &scenario.ReportData{}}}
		_, err := service.ProcessForTask(context.Background(), 3, 7, "science-report", "course-1", processor)
		Expect(err).To(MatchError(ContainSubstring("load course report data: course data failed")))
	})

	It("handles processing update failures and invalid outcomes", func() {
		processingFailure := &collectionRunRepository{updateErr: errors.New("processing update failed"), failAt: 2}
		service := application.NewScenarioCollectionService(collectionSubmissionRepository{}, processingFailure)
		_, err := service.Process(context.Background(), 1, 2, &collectionProcessor{result: scenario.ProcessResult{Outcome: scenario.Completed}})
		Expect(err).To(MatchError(ContainSubstring("persist processing report run: processing update failed")))

		runs := &collectionRunRepository{}
		service = application.NewScenarioCollectionService(collectionSubmissionRepository{}, runs)
		_, err = service.Process(context.Background(), 1, 2, &collectionProcessor{result: scenario.ProcessResult{Outcome: "invalid"}})
		Expect(err).To(MatchError(ContainSubstring("InvalidReportOutcome")))
	})
})

func contains(value, fragment string) bool {
	for i := 0; i+len(fragment) <= len(value); i++ {
		if value[i:i+len(fragment)] == fragment {
			return true
		}
	}
	return false
}
