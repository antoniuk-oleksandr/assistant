package renderer

import (
	"context"
	"errors"
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
