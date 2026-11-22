package renderer

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alex/assistent/internal/domain/scenario"
)

func TestLocalReportRendererWritesStructuredReportPackage(t *testing.T) {
	renderer := NewLocalReportRenderer()
	data := scenario.ReportData{ScenarioID: 11, TaskID: 22}
	data.Appendices[0].Number = 1
	data.Appendices[0].TextValues = []string{"value"}

	output, err := renderer.RenderAndSave(context.Background(), data)
	if err != nil {
		t.Fatalf("RenderAndSave() error = %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(output.Path) })
	if output.Format != "local-package" || output.Version != 1 || output.Path == "" {
		t.Fatalf("output = %#v", output)
	}
	if filepath.Base(output.Path) == output.Path {
		t.Fatalf("output path is not absolute/cleaned: %q", output.Path)
	}
	contents, err := os.ReadFile(output.Path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	var got scenario.ReportData
	if err := json.Unmarshal(contents, &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if got.ScenarioID != data.ScenarioID || got.TaskID != data.TaskID || got.Appendices[0].TextValues[0] != "value" {
		t.Fatalf("rendered data = %#v", got)
	}
}

func TestLocalReportRendererReportsOutputDirectoryFailure(t *testing.T) {
	parent := t.TempDir()
	invalidDirectory := filepath.Join(parent, "not-a-directory")
	if err := os.WriteFile(invalidDirectory, []byte("file"), 0600); err != nil {
		t.Fatal(err)
	}
	renderer := NewLocalReportRenderer()
	renderer.directory = invalidDirectory
	if _, err := renderer.RenderAndSave(context.Background(), scenario.ReportData{}); err == nil {
		t.Fatal("RenderAndSave() unexpectedly succeeded")
	}
}

func TestLocalXLSXRendererHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := NewLocalXLSXRenderer().RenderAndSave(ctx, scenario.ReportData{}); err == nil {
		t.Fatal("canceled XLSX render unexpectedly succeeded")
	}
}

func TestLocalXLSXRendererWritesAllReportSections(t *testing.T) {
	renderer := NewLocalXLSXRenderer()
	data := scenario.ReportData{
		Appendices:         [28]scenario.ReportAppendix{{Number: 1, NumericTotal: 2, TextValues: []string{"A", "B"}}},
		SupportingEvidence: []scenario.ReportSource{{SubmissionID: "s1", Submitter: "person", MaterialID: "m1", Name: "file", URI: "uri", LocalPath: "path", Reason: "reason"}},
		InvalidMaterials:   []scenario.ReportSource{{Name: "bad", Reason: "invalid"}},
		MissingSubmissions: []string{"missing"},
		Conflicts:          []scenario.ReportConflict{{Appendix: 1, Values: []string{"one", "two"}}},
	}
	output, err := renderer.RenderAndSave(context.Background(), data)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(output.Path)
	if output.Format != "google-sheets" || output.Path == "" {
		t.Fatalf("output = %#v", output)
	}
}

func TestLocalReportRendererReportsInjectedFilesystemFailures(t *testing.T) {
	originalMarshal, originalMkdir, originalCreate := localMarshalIndent, localMkdirAll, localCreateTemp
	originalWrite, originalClose := localWriteFile, localCloseFile
	t.Cleanup(func() {
		localMarshalIndent, localMkdirAll, localCreateTemp = originalMarshal, originalMkdir, originalCreate
		localWriteFile, localCloseFile = originalWrite, originalClose
	})
	localMarshalIndent = func(any, string, string) ([]byte, error) { return nil, errors.New("marshal failed") }
	if _, err := NewLocalReportRenderer().RenderAndSave(context.Background(), scenario.ReportData{}); err == nil || !strings.Contains(err.Error(), "marshal report package") {
		t.Fatalf("marshal error = %v", err)
	}
	localMarshalIndent = originalMarshal
	localMkdirAll = func(string, os.FileMode) error { return errors.New("mkdir failed") }
	if _, err := NewLocalReportRenderer().RenderAndSave(context.Background(), scenario.ReportData{}); err == nil || !strings.Contains(err.Error(), "create report output directory") {
		t.Fatalf("mkdir error = %v", err)
	}
	localMkdirAll = originalMkdir
	localCreateTemp = func(string, string) (*os.File, error) { return nil, errors.New("temp failed") }
	if _, err := NewLocalReportRenderer().RenderAndSave(context.Background(), scenario.ReportData{}); err == nil || !strings.Contains(err.Error(), "create report package") {
		t.Fatalf("temp error = %v", err)
	}
	localCreateTemp = originalCreate
	localWriteFile = func(*os.File, []byte) (int, error) { return 0, errors.New("write failed") }
	if _, err := NewLocalReportRenderer().RenderAndSave(context.Background(), scenario.ReportData{}); err == nil || !strings.Contains(err.Error(), "write report package") {
		t.Fatalf("write error = %v", err)
	}
	localWriteFile = originalWrite
	localCloseFile = func(*os.File) error { return errors.New("close failed") }
	if _, err := NewLocalReportRenderer().RenderAndSave(context.Background(), scenario.ReportData{}); err == nil || !strings.Contains(err.Error(), "close report package") {
		t.Fatalf("close error = %v", err)
	}
}
