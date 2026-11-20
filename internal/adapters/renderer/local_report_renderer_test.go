package renderer

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
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
