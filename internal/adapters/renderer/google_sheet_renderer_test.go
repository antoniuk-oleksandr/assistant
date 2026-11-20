package renderer

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/alex/assistent/internal/domain/scenario"
	"github.com/xuri/excelize/v2"
)

func TestGoogleSheetRendererWritesAggregateAndEvidenceSheets(t *testing.T) {
	directory := t.TempDir()
	renderer := NewGoogleSheetRenderer()
	renderer.directory = directory
	data := scenario.ReportData{MissingSubmissions: []string{"missing-1"}, SupportingEvidence: []scenario.ReportSource{{SubmissionID: "s1", MaterialID: "e1", Name: "certificate.pdf"}}, Conflicts: []scenario.ReportConflict{{Appendix: 2, Source: scenario.ReportSource{SubmissionID: "s1", MaterialID: "a2"}, Values: []string{"one", "two"}}}}
	data.Appendices[0].Number = 1
	data.Appendices[0].NumericTotal = 4
	data.Appendices[0].TextValues = []string{"award"}
	output, err := renderer.RenderAndSave(context.Background(), data)
	if err != nil {
		t.Fatalf("RenderAndSave() error = %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(output.Path) })
	if output.Format != "google-sheets" || filepath.Ext(output.Path) != ".xlsx" {
		t.Fatalf("output = %#v", output)
	}
	book, err := excelize.OpenFile(output.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = book.Close() }()
	if got, err := book.GetCellValue("Appendices", "B2"); err != nil || got != "4" {
		t.Fatalf("numeric total = %q", got)
	}
	if got, err := book.GetCellValue("Supporting Evidence", "D2"); err != nil || got != "certificate.pdf" {
		t.Fatalf("evidence = %q", got)
	}
	if got, err := book.GetCellValue("Missing Submissions", "A2"); err != nil || got != "missing-1" {
		t.Fatalf("missing = %q", got)
	}
	if got, err := book.GetCellValue("Conflicts", "D2"); err != nil || got != "one; two" {
		t.Fatalf("conflict = %q", got)
	}
}

func TestGoogleSheetRendererHonorsContextAndDirectoryErrors(t *testing.T) {
	renderer := NewGoogleSheetRenderer()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := renderer.RenderAndSave(ctx, scenario.ReportData{}); err == nil {
		t.Fatal("cancelled render unexpectedly succeeded")
	}
	parent := t.TempDir()
	file := filepath.Join(parent, "not-a-directory")
	if err := os.WriteFile(file, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	renderer.directory = file
	if _, err := renderer.RenderAndSave(context.Background(), scenario.ReportData{}); err == nil {
		t.Fatal("invalid directory render unexpectedly succeeded")
	}
	renderer.directory = "/proc"
	if _, err := renderer.RenderAndSave(context.Background(), scenario.ReportData{}); err == nil {
		t.Fatal("unwritable output render unexpectedly succeeded")
	}
}
