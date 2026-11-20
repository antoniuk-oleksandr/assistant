package renderer

import (
	"context"
	"fmt"
	"time"

	"github.com/alex/assistent/internal/domain/ports"
	"github.com/alex/assistent/internal/domain/report"
	"github.com/alex/assistent/internal/domain/scenario"
	"github.com/alex/assistent/internal/platform/config"
)

// NewConfiguredReportRenderer makes the runtime choice explicit. Production
// configuration defaults to Google Sheets; REPORT_RENDERER=local-xlsx selects
// the local fallback.
func NewConfiguredReportRenderer(cfg config.Config) (ports.ReportRenderer, error) {
	// Fx unit tests and embedders may supply a partial Config directly. The
	// production loader always fills this field with "google-sheets".
	if cfg.Report.Renderer == "" {
		return NewLocalXLSXRenderer(), nil
	}
	if cfg.Report.Renderer != "google-sheets" {
		if cfg.Report.Renderer != "" && cfg.Report.Renderer != "local-xlsx" {
			return nil, fmt.Errorf("unsupported REPORT_RENDERER %q", cfg.Report.Renderer)
		}
		return NewLocalXLSXRenderer(), nil
	}
	if cfg.Sheets.Title == "" {
		cfg.Sheets.Title = "Scientific activity report"
	}
	client, err := newGoogleSheetsClient(cfg.Sheets)
	if err != nil {
		return nil, err
	}
	return &GoogleSheetsReportRenderer{client: client, title: cfg.Sheets.Title}, nil
}

type GoogleSheetsReportRenderer struct {
	client sheetsClient
	title  string
}

func (r *GoogleSheetsReportRenderer) RenderAndSave(ctx context.Context, data scenario.ReportData) (report.Output, error) {
	spreadsheetID, err := r.client.CreateSpreadsheet(ctx, r.title)
	if err != nil {
		return report.Output{}, err
	}
	sections := reportSections(data)
	names := make([]string, 0, len(sections))
	for _, section := range sections {
		names = append(names, section.name)
	}
	if err := r.client.AddSheets(ctx, spreadsheetID, names); err != nil {
		return report.Output{}, err
	}
	for _, section := range sections {
		if err := r.client.WriteValues(ctx, spreadsheetID, section.name, section.rows); err != nil {
			return report.Output{}, err
		}
	}
	return report.Output{Format: "google-sheets", Path: "https://docs.google.com/spreadsheets/d/" + spreadsheetID + "/edit", Version: 1, CreatedAt: time.Now().UTC()}, nil
}

type reportSection struct {
	name string
	rows [][]any
}

func reportSections(data scenario.ReportData) []reportSection {
	appendixRows := [][]any{{"Appendix", "Numeric total", "Text values", "Source count", "Conflict count"}}
	for _, appendix := range data.Appendices {
		appendixRows = append(appendixRows, []any{appendix.Number, appendix.NumericTotal, join(appendix.TextValues), len(appendix.Sources), len(appendix.Conflicts)})
	}
	evidenceRows := sourceRows(data.SupportingEvidence)
	invalidRows := sourceRows(data.InvalidMaterials)
	missingRows := [][]any{{"Submission"}}
	for _, item := range data.MissingSubmissions {
		missingRows = append(missingRows, []any{item})
	}
	conflictRows := [][]any{{"Appendix", "Submission", "Material", "Values"}}
	for _, conflict := range data.Conflicts {
		conflictRows = append(conflictRows, []any{conflict.Appendix, conflict.Source.SubmissionID, conflict.Source.MaterialID, join(conflict.Values)})
	}
	return []reportSection{{"Appendices", appendixRows}, {"Supporting Evidence", evidenceRows}, {"Invalid Materials", invalidRows}, {"Missing Submissions", missingRows}, {"Conflicts", conflictRows}}
}

func sourceRows(sources []scenario.ReportSource) [][]any {
	rows := [][]any{{"Submission", "Submitter", "Material", "Name", "URI", "Local path", "Reason"}}
	for _, source := range sources {
		rows = append(rows, []any{source.SubmissionID, source.Submitter, source.MaterialID, source.Name, source.URI, source.LocalPath, source.Reason})
	}
	return rows
}
