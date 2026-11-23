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

type reportReadyNotifierSpy struct {
	calls int
	err   error
}

func (n *reportReadyNotifierSpy) Enabled() bool { return true }
func (n *reportReadyNotifierSpy) Notify(context.Context, report.Result, string) error {
	n.calls++
	return n.err
}

var _ = Describe("scenario collection results", func() {
	It("persists partial results", func() {
		runs := &collectionRunRepository{}
		results := &resultRepository{}
		service := application.NewScenarioCollectionServiceWithRendererAndResult(collectionSubmissionRepository{}, runs, resultRenderer{}, resultOutputRepository{}, results)
		processor := &collectionProcessor{result: scenario.ProcessResult{Outcome: scenario.Partial, ReportData: &scenario.ReportData{ScenarioID: 3, TaskID: 7}}}
		got, err := service.Process(context.Background(), 3, 7, processor)
		Expect(err).NotTo(HaveOccurred())
		Expect(got.Status).To(Equal(report.Completed))
		Expect(results.value.Status).To(Equal(report.ResultPartial))
		Expect(results.value.Partial).To(BeTrue())
	})

	It("processes an incremental update as a new run", func() {
		runs := &collectionRunRepository{}
		service := application.NewScenarioCollectionService(collectionSubmissionRepository{items: []submission.Submission{}}, runs)
		processor := &collectionProcessor{result: scenario.ProcessResult{Outcome: scenario.Completed}}
		first, err := service.ProcessForTask(context.Background(), 3, 7, "science-report", "course-1", processor)
		Expect(err).NotTo(HaveOccurred())
		second, err := service.ProcessForTaskUpdate(context.Background(), 3, 7, "science-report", "course-1", processor)
		Expect(err).NotTo(HaveOccurred())
		Expect(second.ID).To(Equal(first.ID))
		Expect(second.Status).To(Equal(report.Completed))
		Expect(runs.updates).NotTo(BeEmpty())
	})

	It("skips unchanged incremental updates", func() {
		runs := &collectionRunRepository{}
		service := application.NewScenarioCollectionService(collectionSubmissionRepository{items: []submission.Submission{}}, runs)
		processor := &collectionProcessor{result: scenario.ProcessResult{Outcome: scenario.Completed}}
		first, err := service.ProcessForTask(context.Background(), 3, 7, "science-report", "course-1", processor)
		Expect(err).NotTo(HaveOccurred())
		runs.existing = &first
		second, err := service.ProcessForTaskUpdate(context.Background(), 3, 7, "science-report", "course-1", processor)
		Expect(err).NotTo(HaveOccurred())
		Expect(second.ID).To(Equal(first.ID))
	})

	It("processes changed incremental updates", func() {
		runs := &collectionRunRepository{existing: &report.Run{ID: 9, InputFingerprint: "old", Status: report.Completed}}
		service := application.NewScenarioCollectionService(collectionSubmissionRepository{items: []submission.Submission{{ExternalID: "new-submission"}}}, runs)
		processor := &collectionProcessor{result: scenario.ProcessResult{Outcome: scenario.Completed}}
		got, err := service.ProcessForTaskUpdate(context.Background(), 3, 7, "science-report", "course-1", processor)
		Expect(err).NotTo(HaveOccurred())
		Expect(got.ID).To(Equal(int64(42)))
	})

	It("reports incremental submission lookup failures", func() {
		service := application.NewScenarioCollectionService(collectionSubmissionRepository{err: errors.New("submission lookup failed")}, &collectionRunRepository{})
		_, err := service.ProcessForTaskUpdate(context.Background(), 3, 7, "science-report", "course-1", &collectionProcessor{result: scenario.ProcessResult{Outcome: scenario.Completed}})
		Expect(err).To(MatchError(ContainSubstring("collect submissions for task 7")))
	})

	It("persists failed results and reports persistence errors", func() {
		runs := &collectionRunRepository{}
		results := &resultRepository{}
		service := application.NewScenarioCollectionServiceWithRendererAndResult(collectionSubmissionRepository{}, runs, resultRenderer{}, resultOutputRepository{}, results)
		processor := &collectionProcessor{result: scenario.ProcessResult{Outcome: scenario.Failed, ReportData: &scenario.ReportData{}}}
		got, err := service.Process(context.Background(), 3, 7, processor)
		Expect(err).NotTo(HaveOccurred())
		Expect(got.Status).To(Equal(report.Failed))
		Expect(results.value.Status).To(Equal(report.ResultFailed))
		failing := &resultRepository{createErr: errors.New("result insert failed")}
		service = application.NewScenarioCollectionServiceWithRendererAndResult(collectionSubmissionRepository{}, &collectionRunRepository{}, resultRenderer{}, resultOutputRepository{}, failing)
		_, err = service.Process(context.Background(), 3, 7, &collectionProcessor{result: scenario.ProcessResult{Outcome: scenario.Completed, ReportData: &scenario.ReportData{}}})
		Expect(err).To(MatchError(ContainSubstring("persist processing result")))
	})

	It("does not notify for failed results", func() {
		notifier := &reportReadyNotifierSpy{}
		service := application.NewScenarioCollectionServiceWithReportNotifier(
			collectionSubmissionRepository{},
			&collectionRunRepository{},
			resultRenderer{},
			resultOutputRepository{},
			nil,
			nil,
			notifier,
		)

		got, err := service.Process(context.Background(), 3, 7, &collectionProcessor{
			result: scenario.ProcessResult{Outcome: scenario.Failed, ReportData: &scenario.ReportData{}},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(got.Status).To(Equal(report.Failed))
		Expect(notifier.calls).To(BeZero())
	})

	It("logs report-ready notification failures", func() {
		notifier := &reportReadyNotifierSpy{err: errors.New("notification failed")}
		service := application.NewScenarioCollectionServiceWithReportNotifier(
			collectionSubmissionRepository{}, &collectionRunRepository{}, resultRenderer{}, resultOutputRepository{}, nil, nil, notifier,
		)
		_, err := service.Process(context.Background(), 3, 7, &collectionProcessor{
			result: scenario.ProcessResult{Outcome: scenario.Completed, ReportData: &scenario.ReportData{}},
		})
		Expect(err).NotTo(HaveOccurred())
	})

	It("reports renderer and output errors", func() {
		processor := &collectionProcessor{result: scenario.ProcessResult{Outcome: scenario.Completed, ReportData: &scenario.ReportData{}}}
		service := application.NewScenarioCollectionServiceWithRendererAndResult(collectionSubmissionRepository{}, &collectionRunRepository{}, resultRenderer{err: errors.New("render failed")}, resultOutputRepository{}, &resultRepository{})
		_, err := service.Process(context.Background(), 3, 7, processor)
		Expect(err).To(MatchError(ContainSubstring("render report package")))
		service = application.NewScenarioCollectionServiceWithRendererAndResult(collectionSubmissionRepository{}, &collectionRunRepository{}, resultRenderer{}, resultOutputRepository{err: errors.New("output failed")}, &resultRepository{})
		_, err = service.Process(context.Background(), 3, 7, processor)
		Expect(err).To(MatchError(ContainSubstring("persist report output")))
	})
})

var _ ports.ReportResultRepository = (*resultRepository)(nil)
var _ ports.ReportRenderer = resultRenderer{}
var _ ports.ReportOutputRepository = resultOutputRepository{}
var _ ports.SubmissionRepository = collectionSubmissionRepository{}
