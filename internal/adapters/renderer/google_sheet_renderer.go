package renderer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/alex/assistent/internal/domain/report"
	"github.com/alex/assistent/internal/domain/scenario"
	"github.com/xuri/excelize/v2"
)

// LocalXLSXRenderer writes the provider-neutral spreadsheet artifact used by
// the scientific report. The artifact can be opened or uploaded to Google
// Sheets without changing scenario or application code.
type LocalXLSXRenderer struct {
	directory string
}

func NewLocalXLSXRenderer() *LocalXLSXRenderer {
	return &LocalXLSXRenderer{directory: os.TempDir()}
}

func (r *LocalXLSXRenderer) RenderAndSave(ctx context.Context, data scenario.ReportData) (report.Output, error) {
	if err := ctx.Err(); err != nil {
		return report.Output{}, err
	}
	if err := os.MkdirAll(r.directory, 0700); err != nil {
		return report.Output{}, fmt.Errorf("create spreadsheet output directory: %w", err)
	}
	book := excelize.NewFile()
	defer func() { _ = book.Close() }()
	writeAppendices(book, data)
	writeSources(book, "Supporting Evidence", data.SupportingEvidence)
	writeSources(book, "Invalid Materials", data.InvalidMaterials)
	writeMissing(book, data.MissingSubmissions)
	writeConflicts(book, data.Conflicts)
	_ = book.DeleteSheet("Sheet1")
	file, err := os.CreateTemp(r.directory, "science-report-*.xlsx")
	if err != nil {
		return report.Output{}, fmt.Errorf("create spreadsheet: %w", err)
	}
	path := file.Name()
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return report.Output{}, fmt.Errorf("close spreadsheet: %w", err)
	}
	if err := book.SaveAs(path); err != nil {
		_ = os.Remove(path)
		return report.Output{}, fmt.Errorf("save spreadsheet: %w", err)
	}
	return report.Output{Format: "google-sheets", Path: filepath.Clean(path), Version: 1, CreatedAt: time.Now().UTC()}, nil
}

func writeAppendices(book *excelize.File, data scenario.ReportData) {
	const sheet = "Appendices"
	_, _ = book.NewSheet(sheet)
	values := [][]any{{"Appendix", "Numeric total", "Text values", "Source count", "Conflict count"}}
	for _, appendix := range data.Appendices {
		values = append(values, []any{appendix.Number, appendix.NumericTotal, join(appendix.TextValues), len(appendix.Sources), len(appendix.Conflicts)})
	}
	writeRows(book, sheet, values)
}

func writeSources(book *excelize.File, sheet string, sources []scenario.ReportSource) {
	_, _ = book.NewSheet(sheet)
	values := [][]any{{"Submission", "Submitter", "Material", "Name", "URI", "Local path", "Reason"}}
	for _, source := range sources {
		values = append(values, []any{source.SubmissionID, source.Submitter, source.MaterialID, source.Name, source.URI, source.LocalPath, source.Reason})
	}
	writeRows(book, sheet, values)
}

func writeMissing(book *excelize.File, missing []string) {
	_, _ = book.NewSheet("Missing Submissions")
	values := [][]any{{"Submission"}}
	for _, item := range missing {
		values = append(values, []any{item})
	}
	writeRows(book, "Missing Submissions", values)
}

func writeConflicts(book *excelize.File, conflicts []scenario.ReportConflict) {
	_, _ = book.NewSheet("Conflicts")
	values := [][]any{{"Appendix", "Submission", "Material", "Values"}}
	for _, conflict := range conflicts {
		values = append(values, []any{conflict.Appendix, conflict.Source.SubmissionID, conflict.Source.MaterialID, join(conflict.Values)})
	}
	writeRows(book, "Conflicts", values)
}

func writeRows(book *excelize.File, sheet string, rows [][]any) {
	for rowIndex, row := range rows {
		for columnIndex, value := range row {
			cell, err := excelize.CoordinatesToCellName(columnIndex+1, rowIndex+1)
			if err != nil {
				continue
			}
			_ = book.SetCellValue(sheet, cell, value)
		}
	}
}

func join(values []string) string {
	result := ""
	for index, value := range values {
		if index > 0 {
			result += "; "
		}
		result += value
	}
	return result
}
