package report_test

import (
	"errors"
	"testing"
	"time"

	"github.com/alex/assistent/internal/domain/report"
	"github.com/alex/assistent/internal/domain/scenario"
)

func TestRunLifecycle(t *testing.T) {
	now := time.Date(2026, time.November, 20, 12, 0, 0, 0, time.FixedZone("local", 2*60*60))
	run := report.NewRun(3, 7, now)
	if run.Status != report.Scheduled || !run.CreatedAt.Equal(now.UTC()) {
		t.Fatalf("NewRun() = %#v", run)
	}
	if err := run.Transition(report.Collecting, now); err != nil {
		t.Fatalf("transition to collecting: %v", err)
	}
	if run.StartedAt == nil || !run.StartedAt.Equal(now.UTC()) {
		t.Fatalf("started at = %v", run.StartedAt)
	}
	if err := run.Transition(report.Processing, now); err != nil {
		t.Fatalf("transition to processing: %v", err)
	}
	if err := run.ApplyResult(scenario.ProcessResult{Outcome: scenario.Partial, ResultPath: "/tmp/result", ErrorMessage: "one failed"}, now.Add(time.Minute)); err != nil {
		t.Fatalf("apply partial result: %v", err)
	}
	if run.Status != report.Completed || !run.Partial || run.ResultPath != "/tmp/result" || run.ErrorMessage != "one failed" || run.FinishedAt == nil {
		t.Fatalf("partial result = %#v", run)
	}
}

func TestRunTransitionsAndOutcomesRejectInvalidValues(t *testing.T) {
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
		t.Run(tc.name, func(t *testing.T) {
			run := report.Run{Status: tc.from}
			if err := run.Transition(tc.to, now); !errors.Is(err, report.ErrInvalidTransition) {
				t.Fatalf("error = %v, want invalid transition", err)
			}
		})
	}

	run := report.Run{Status: report.Processing}
	if err := run.ApplyResult(scenario.ProcessResult{Outcome: "unknown"}, now); !errors.Is(err, report.ErrInvalidOutcome) {
		t.Fatalf("invalid outcome error = %v", err)
	}
	run = report.Run{Status: report.Processing}
	if err := run.ApplyResult(scenario.ProcessResult{Outcome: scenario.Failed, ErrorMessage: "broken"}, now); err != nil {
		t.Fatalf("failed result: %v", err)
	}
	if run.Status != report.Failed || run.ErrorMessage != "broken" || run.FinishedAt == nil {
		t.Fatalf("failed result = %#v", run)
	}
}

func TestRunWaitingForDeadlineAndIdempotentTransitions(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	run := report.Run{Status: report.Scheduled}
	if err := run.Transition(report.Scheduled, now); err != nil {
		t.Fatal(err)
	}
	if err := run.Transition(report.Collecting, now); err != nil {
		t.Fatal(err)
	}
	if err := run.Transition(report.Processing, now); err != nil {
		t.Fatal(err)
	}
	if err := run.Transition(report.WaitingForDeadline, now); err != nil {
		t.Fatal(err)
	}
	if err := run.Transition(report.Processing, now); err != nil {
		t.Fatal(err)
	}
	if err := run.Transition(report.Completed, now); err != nil {
		t.Fatal(err)
	}
	if err := run.Transition(report.Completed, now); err != nil {
		t.Fatal(err)
	}
}
