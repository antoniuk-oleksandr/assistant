package report_test

import (
	"testing"
	"time"

	"github.com/alex/assistent/internal/domain/report"
	"github.com/alex/assistent/internal/domain/scenario"
)

func TestResultLifecycle(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	result := report.NewResult(4, 5, 6, scenario.ReportData{ScenarioID: 5, TaskID: 6}, now)
	if result.Version != 1 || result.Status != report.ResultProcessing || !result.CreatedAt.Equal(now) {
		t.Fatalf("new result = %#v", result)
	}
	result.Complete(report.ResultPartial, now.Add(time.Minute), "one invalid material")
	if result.Status != report.ResultPartial || !result.Partial || result.ErrorMessage == "" || !result.UpdatedAt.After(result.CreatedAt) {
		t.Fatalf("completed result = %#v", result)
	}
}
