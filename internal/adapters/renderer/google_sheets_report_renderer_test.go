package renderer

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/alex/assistent/internal/domain/scenario"
	"github.com/alex/assistent/internal/platform/config"
)

type fakeSheetsClient struct {
	id        string
	title     string
	sheets    []string
	values    map[string][][]any
	createErr error
	addErr    error
	writeErr  error
	ranges    []sheetValueRange
	clear     []string
	rangeErr  error
	clearErr  error
}

func (f *fakeSheetsClient) CreateSpreadsheet(_ context.Context, title string) (string, error) {
	f.title = title
	if f.createErr != nil {
		return "", f.createErr
	}
	return f.id, nil
}
func (f *fakeSheetsClient) AddSheets(_ context.Context, _ string, names []string) error {
	if f.addErr != nil {
		return f.addErr
	}
	f.sheets = append(f.sheets, names...)
	return nil
}
func (f *fakeSheetsClient) WriteValues(_ context.Context, _ string, sheet string, rows [][]any) error {
	if f.writeErr != nil {
		return f.writeErr
	}
	if f.values == nil {
		f.values = map[string][][]any{}
	}
	f.values[sheet] = rows
	return nil
}

func (f *fakeSheetsClient) WriteRanges(_ context.Context, _ string, ranges []sheetValueRange) error {
	if f.rangeErr != nil {
		return f.rangeErr
	}
	f.ranges = ranges
	return nil
}

func (f *fakeSheetsClient) ClearRanges(_ context.Context, _ string, ranges []string) error {
	if f.clearErr != nil {
		return f.clearErr
	}
	f.clear = ranges
	return nil
}

type fakeTemplateUploader struct {
	id    string
	err   error
	title string
	path  string
}

func (f *fakeTemplateUploader) UploadSpreadsheet(_ context.Context, title, path string) (string, error) {
	f.title, f.path = title, path
	if f.err != nil {
		return "", f.err
	}
	return f.id, nil
}

func TestGoogleSheetsReportRendererCreatesAggregate(t *testing.T) {
	client := &fakeSheetsClient{id: "sheet-123"}
	renderer := &GoogleSheetsReportRenderer{client: client, title: "Report"}
	data := scenario.ReportData{MissingSubmissions: []string{"missing"}}
	data.Appendices[0].Number = 1
	data.Appendices[0].NumericTotal = 3
	output, err := renderer.RenderAndSave(context.Background(), data)
	if err != nil {
		t.Fatalf("RenderAndSave() error = %v", err)
	}
	if output.Format != "google-sheets" || !strings.Contains(output.Path, "/sheet-123/edit") {
		t.Fatalf("output = %#v", output)
	}
	if client.title != "Report" || len(client.sheets) != 5 || client.values["Appendices"][1][1] != float64(3) {
		t.Fatalf("client state = %#v", client)
	}
}

func TestGoogleSheetsReportRendererStopsOnProviderErrors(t *testing.T) {
	providerErr := errors.New("provider unavailable")
	for name, client := range map[string]*fakeSheetsClient{
		"create": {createErr: providerErr},
		"add":    {id: "sheet", addErr: providerErr},
		"write":  {id: "sheet", writeErr: providerErr},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := (&GoogleSheetsReportRenderer{client: client, title: "Report"}).RenderAndSave(context.Background(), scenario.ReportData{})
			if !errors.Is(err, providerErr) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestGoogleSheetsReportRendererImportsTemplateAndUpdatesColumnC(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	templatePath := filepath.Join(filepath.Dir(filename), "..", "..", "..", "templates", "scientific-report", "output_example.xlsx")
	if _, err := os.Stat(templatePath); err != nil {
		t.Fatal(err)
	}
	appendices, slashRows, columnValues := loadTemplateLayoutFromPath(t, templatePath)
	client := &fakeSheetsClient{id: "created-but-replaced"}
	uploader := &fakeTemplateUploader{id: "imported-sheet"}
	renderer := &GoogleSheetsReportRenderer{
		client: client, uploader: uploader, title: "Report", templatePath: templatePath,
		templateAppendices: appendices, templateSlashRows: slashRows, templateColumnValues: columnValues,
	}
	data := scenario.ReportData{}
	data.Appendices[0] = scenario.ReportAppendix{Number: 1, NumericTotal: 4}
	data.Appendices[1] = scenario.ReportAppendix{Number: 2}
	output, err := renderer.RenderAndSave(context.Background(), data)
	if err != nil {
		t.Fatal(err)
	}
	if output.Path != "https://docs.google.com/spreadsheets/d/imported-sheet/edit" {
		t.Fatalf("output path = %q", output.Path)
	}
	if uploader.title != "Report" || uploader.path != templatePath || len(client.ranges) != 1 {
		t.Fatalf("upload/write state = %#v %#v %#v", uploader, client.ranges, client.clear)
	}
	if client.ranges[0].rangeName != "Аркуш1!C13" || len(client.ranges[0].values) != 120 {
		t.Fatalf("range = %#v", client.ranges[0])
	}
	if client.ranges[0].values[appendices[1]-13][0] != float64(4) {
		t.Fatalf("appendix value = %#v", client.ranges[0].values[appendices[1]-13][0])
	}
	if client.ranges[0].values[appendices[2]-13][0] != "/" {
		t.Fatalf("empty appendix value = %#v", client.ranges[0].values[appendices[2]-13][0])
	}
	if len(client.clear) != len(templateAlwaysBlankRows) {
		t.Fatalf("cleared ranges = %d, want %d", len(client.clear), len(templateAlwaysBlankRows))
	}
}

func TestGoogleSheetsReportRendererTemplateErrors(t *testing.T) {
	providerErr := errors.New("provider unavailable")
	base := &GoogleSheetsReportRenderer{
		client:   &fakeSheetsClient{id: "sheet", rangeErr: providerErr},
		uploader: &fakeTemplateUploader{id: "sheet"}, title: "Report", templatePath: "template.xlsx",
		templateAppendices: map[int]int{1: 13}, templateColumnValues: make([][]any, 200),
	}
	if _, err := base.RenderAndSave(context.Background(), scenario.ReportData{}); !errors.Is(err, providerErr) {
		t.Fatalf("write error = %v", err)
	}
	base.client = &fakeSheetsClient{id: "sheet", clearErr: providerErr}
	if _, err := base.RenderAndSave(context.Background(), scenario.ReportData{}); !errors.Is(err, providerErr) {
		t.Fatalf("clear error = %v", err)
	}
	base.client = &fakeSheetsClient{id: "sheet"}
	base.uploader = nil
	if _, err := base.RenderAndSave(context.Background(), scenario.ReportData{}); err == nil || !strings.Contains(err.Error(), "Drive write access") {
		t.Fatalf("missing uploader error = %v", err)
	}
}

func loadTemplateLayoutFromPath(t *testing.T, path string) (map[int]int, []int, [][]any) {
	t.Helper()
	old := os.Getenv("REPORT_TEMPLATE_PATH")
	if err := os.Setenv("REPORT_TEMPLATE_PATH", path); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Setenv("REPORT_TEMPLATE_PATH", old) })
	return loadTemplateLayout()
}

func TestConfiguredReportRendererSelectsExplicitLocalArtifact(t *testing.T) {
	renderer, err := NewConfiguredReportRenderer(config.Config{Report: config.ReportConfig{Renderer: "local-xlsx"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := renderer.(*LocalXLSXRenderer); !ok {
		t.Fatalf("renderer type = %T", renderer)
	}
}

func TestConfiguredReportRendererSelectsGoogleSheets(t *testing.T) {
	renderer, err := NewConfiguredReportRenderer(config.Config{Report: config.ReportConfig{Renderer: "google-sheets"}, Sheets: config.SheetsConfig{ClientID: "client", ClientSecret: "secret", RefreshToken: "refresh"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := renderer.(*GoogleSheetsReportRenderer); !ok {
		t.Fatalf("renderer type = %T", renderer)
	}
}

func TestConfiguredReportRendererRejectsUnknownSelection(t *testing.T) {
	if _, err := NewConfiguredReportRenderer(config.Config{Report: config.ReportConfig{Renderer: "unknown"}}); err == nil {
		t.Fatal("unknown renderer unexpectedly accepted")
	}
}
