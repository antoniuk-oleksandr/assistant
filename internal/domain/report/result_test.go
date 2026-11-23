package report_test

import (
	"testing"
	"time"

	"github.com/alex/assistent/internal/domain/report"
	"github.com/alex/assistent/internal/domain/scenario"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestResultLifecycle(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Report result lifecycle")
}

var _ = Describe("Report result lifecycle", func() {
	It("creates and completes a partial result", func() {
		now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		result := report.NewResult(4, 5, 6, scenario.ReportData{ScenarioID: 5, TaskID: 6}, now)
		Expect(result.Version).To(Equal(1))
		Expect(result.Status).To(Equal(report.ResultProcessing))
		Expect(result.CreatedAt).To(Equal(now))
		result.Complete(report.ResultPartial, now.Add(time.Minute), "one invalid material")
		Expect(result.Status).To(Equal(report.ResultPartial))
		Expect(result.Partial).To(BeTrue())
		Expect(result.ErrorMessage).NotTo(BeEmpty())
		Expect(result.UpdatedAt).To(BeTemporally(">", result.CreatedAt))
	})
})
