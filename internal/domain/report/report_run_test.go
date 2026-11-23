package report_test

import (
	"errors"
	"time"

	"github.com/alex/assistent/internal/domain/report"
	"github.com/alex/assistent/internal/domain/scenario"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Report run lifecycle", func() {
	It("transitions and applies a partial result", func() {
		now := time.Date(2026, time.November, 20, 12, 0, 0, 0, time.FixedZone("local", 2*60*60))
		run := report.NewRun(3, 7, now)
		Expect(run.Status).To(Equal(report.Scheduled))
		Expect(run.CreatedAt).To(Equal(now.UTC()))
		Expect(run.Transition(report.Collecting, now)).To(Succeed())
		Expect(run.StartedAt).NotTo(BeNil())
		Expect(*run.StartedAt).To(Equal(now.UTC()))
		Expect(run.Transition(report.Processing, now)).To(Succeed())
		Expect(run.ApplyResult(scenario.ProcessResult{Outcome: scenario.Partial, ResultPath: "/tmp/result", ErrorMessage: "one failed"}, now.Add(time.Minute))).To(Succeed())
		Expect(run.Status).To(Equal(report.Completed))
		Expect(run.Partial).To(BeTrue())
		Expect(run.ResultPath).To(Equal("/tmp/result"))
		Expect(run.ErrorMessage).To(Equal("one failed"))
		Expect(run.FinishedAt).NotTo(BeNil())
	})

	It("rejects invalid transitions and outcomes", func() {
		now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		cases := []struct {
			name string
			from report.Status
			to   report.Status
		}{
			{"scheduled to processing", report.Scheduled, report.Processing},
			{"collecting to completed", report.Collecting, report.Completed},
			{"processing to scheduled", report.Processing, report.Scheduled},
			{"completed to failed", report.Completed, report.Failed},
			{"failed to collecting", report.Failed, report.Collecting},
		}
		for _, tc := range cases {
			By(tc.name)
			run := report.Run{Status: tc.from}
			err := run.Transition(tc.to, now)
			Expect(errors.Is(err, report.ErrInvalidTransition)).To(BeTrue())
		}

		run := report.Run{Status: report.Processing}
		Expect(errors.Is(run.ApplyResult(scenario.ProcessResult{Outcome: "unknown"}, now), report.ErrInvalidOutcome)).To(BeTrue())
		run = report.Run{Status: report.Processing}
		Expect(run.ApplyResult(scenario.ProcessResult{Outcome: scenario.Failed, ErrorMessage: "broken"}, now)).To(Succeed())
		Expect(run.Status).To(Equal(report.Failed))
		Expect(run.ErrorMessage).To(Equal("broken"))
		Expect(run.FinishedAt).NotTo(BeNil())
	})

	It("waits for a deadline and keeps transitions idempotent", func() {
		now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		run := report.Run{Status: report.Scheduled}
		for _, status := range []report.Status{report.Scheduled, report.Collecting, report.Processing, report.WaitingForDeadline, report.Processing, report.Completed, report.Completed} {
			Expect(run.Transition(status, now)).To(Succeed())
		}
	})
})
