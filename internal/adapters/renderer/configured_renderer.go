package renderer

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/alex/assistent/internal/domain/ports"
	"github.com/alex/assistent/internal/domain/report"
	"github.com/alex/assistent/internal/domain/scenario"
	"github.com/alex/assistent/internal/platform/config"
	"github.com/xuri/excelize/v2"
)

// NewConfiguredReportRenderer makes the runtime choice explicit. Production
// configuration defaults to Google Sheets; the local renderer is selected by
// the typed ReportConfig value.
func NewConfiguredReportRenderer(reportCfg config.ReportConfig, sheetsCfg config.SheetsConfig, driveCfg config.DriveConfig) (ports.ReportRenderer, error) {
	if reportCfg.Renderer == "" {
		return NewLocalXLSXRenderer(), nil
	}
	if reportCfg.Renderer != "google-sheets" {
		if reportCfg.Renderer != "" && reportCfg.Renderer != "local-xlsx" {
			return nil, fmt.Errorf("unsupported REPORT_RENDERER %q", reportCfg.Renderer)
		}
		return NewLocalXLSXRenderer(), nil
	}
	if sheetsCfg.Title == "" {
		sheetsCfg.Title = "Scientific activity report"
	}
	client, err := newGoogleSheetsClient(sheetsCfg)
	if err != nil {
		return nil, err
	}
	templateAppendices, templateSlashRows, templateColumnValues := loadTemplateLayout(reportCfg.TemplatePath)
	uploader, err := newGoogleDriveTemplateUploader(driveCfg)
	if err != nil {
		return nil, err
	}
	return &GoogleSheetsReportRenderer{client: client, uploader: uploader, title: sheetsCfg.Title, templateAppendices: templateAppendices, templateSlashRows: templateSlashRows, templateColumnValues: templateColumnValues, templatePath: reportCfg.TemplatePath}, nil
}

type GoogleSheetsReportRenderer struct {
	client               sheetsClient
	uploader             templateUploader
	title                string
	templateAppendices   map[int]int
	templateSlashRows    []int
	templateColumnValues [][]any
	templatePath         string
}

var templateAlwaysBlankRows = map[int]struct{}{
	12: {}, 17: {}, 18: {}, 20: {}, 21: {}, 22: {}, 33: {}, 41: {}, 42: {}, 43: {}, 44: {}, 46: {}, 47: {}, 49: {}, 51: {}, 53: {}, 55: {}, 57: {}, 59: {}, 68: {}, 69: {}, 76: {}, 78: {}, 86: {}, 87: {}, 97: {}, 103: {}, 106: {}, 114: {}, 116: {}, 118: {}, 120: {}, 122: {}, 124: {}, 126: {}, 128: {}, 130: {}, 132: {},
}

func (r *GoogleSheetsReportRenderer) RenderAndSave(ctx context.Context, data scenario.ReportData) (report.Output, error) {
	spreadsheetID, err := r.client.CreateSpreadsheet(ctx, r.title)
	if err != nil {
		return report.Output{}, err
	}
	if len(r.templateAppendices) > 0 {
		if r.uploader == nil {
			return report.Output{}, fmt.Errorf("Google Drive write access is required to import the report template; authorize the Drive OAuth client with %s", driveFileScope)
		}
		spreadsheetID, err = r.uploader.UploadSpreadsheet(ctx, r.title, r.templatePath)
		if err != nil {
			return report.Output{}, err
		}
		writer, ok := r.client.(sheetsRangeWriter)
		if !ok {
			return report.Output{}, fmt.Errorf("Google Sheets client does not support template cell updates")
		}
		updates := make([]sheetValueRange, 0, len(r.templateSlashRows)+len(data.Appendices))
		columnValues := make([][]any, len(r.templateColumnValues))
		for i, value := range r.templateColumnValues {
			columnValues[i] = append([]any(nil), value...)
		}
		for _, row := range r.templateSlashRows {
			columnValues[row-1][0] = "/"
		}
		for _, appendix := range data.Appendices {
			row, exists := r.templateAppendices[appendix.Number]
			if !exists {
				continue
			}
			value := any("/")
			if appendix.NumericTotal > 0 {
				value = appendix.NumericTotal
			}
			columnValues[row-1][0] = value
		}
		// C13 is the first intended value cell. Anchoring there avoids leading
		// empty cells being normalized away by the Sheets values API.
		updates = append(updates, sheetValueRange{rangeName: "Аркуш1!C13", values: columnValues[12:132]})
		updates = append(updates, courseDataRanges(data.CourseData)...)
		if err := writer.WriteRanges(ctx, spreadsheetID, updates); err != nil {
			return report.Output{}, err
		}
		clearer, ok := r.client.(sheetsRangeClearer)
		if !ok {
			return report.Output{}, fmt.Errorf("Google Sheets client does not support template cell clearing")
		}
		blankRanges := make([]string, 0, len(templateAlwaysBlankRows))
		for row := range templateAlwaysBlankRows {
			blankRanges = append(blankRanges, fmt.Sprintf("Аркуш1!C%d", row))
		}
		if err := clearer.ClearRanges(ctx, spreadsheetID, blankRanges); err != nil {
			return report.Output{}, err
		}
		return report.Output{Format: "google-sheets", Path: "https://docs.google.com/spreadsheets/d/" + spreadsheetID + "/edit", Version: 1, CreatedAt: time.Now().UTC()}, nil
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

func courseDataRanges(fields map[string]string) []sheetValueRange {
	department := strings.TrimSpace(fields["department_name"])
	if department != "" {
		department = "кафедри " + department
	}
	return []sheetValueRange{
		{rangeName: "Аркуш1!B4", values: [][]any{{department}}},
		{rangeName: "Аркуш1!C7", values: [][]any{{fields["department_email"]}}},
		{rangeName: "Аркуш1!C8", values: [][]any{{fields["lab_assistant_name"]}}},
		{rangeName: "Аркуш1!C9", values: [][]any{{phoneCellValue(fields["lab_assistant_mobile_phone"])}}},
		{rangeName: "Аркуш1!C10", values: [][]any{{phoneCellValue(fields["department_phone"])}}},
	}
}

func phoneCellValue(value string) any {
	if strings.HasPrefix(value, "+") {
		return "'" + value
	}
	return value
}

func loadTemplateLayout(path string) (map[int]int, []int, [][]any) {
	book, err := excelize.OpenFile(path)
	if err != nil {
		return nil, nil, nil
	}
	defer book.Close()
	starts := make(map[int]int)
	slashRows := make([]int, 0)
	columnValues := make([][]any, 200)
	for row := 1; row <= 200; row++ {
		left, _ := book.GetCellValue("Аркуш1", fmt.Sprintf("B%d", row))
		right, _ := book.GetCellValue("Аркуш1", fmt.Sprintf("C%d", row))
		columnValues[row-1] = []any{right}
		if strings.TrimSpace(left) != "" && strings.TrimSpace(right) == "" {
			if _, alwaysBlank := templateAlwaysBlankRows[row]; alwaysBlank {
				continue
			}
			slashRows = append(slashRows, row)
		}
		value, err := book.GetCellValue("Аркуш1", fmt.Sprintf("A%d", row))
		if err != nil {
			continue
		}
		appendix, err := strconv.Atoi(strings.TrimSpace(value))
		if err == nil && appendix > 0 {
			if _, exists := starts[appendix]; !exists {
				starts[appendix] = row
			}
		}
	}
	for row := range templateAlwaysBlankRows {
		columnValues[row-1][0] = ""
	}
	// Excelize reports the value of a vertically merged A cell at the end of
	// the merge as well. Use the merge anchor so appendices 1–3 do not start at
	// the bottom of their blocks.
	if merges, err := book.GetMergeCells("Аркуш1"); err == nil {
		for _, merged := range merges {
			startAxis := merged.GetStartAxis()
			if !strings.HasPrefix(startAxis, "A") {
				continue
			}
			_, startRow, err := excelize.CellNameToCoordinates(startAxis)
			if err != nil {
				continue
			}
			value, err := book.GetCellValue("Аркуш1", startAxis)
			appendix, parseErr := strconv.Atoi(strings.TrimSpace(value))
			if err == nil && parseErr == nil && appendix > 0 {
				starts[appendix] = startRow
			}
		}
	}
	result := make(map[int]int, len(starts))
	for appendix, start := range starts {
		end := 201
		for other, otherStart := range starts {
			if other != appendix && otherStart > start && otherStart < end {
				end = otherStart
			}
		}
		result[appendix] = start
		fallback := 0
		for row := start; row < end; row++ {
			label, err := book.GetCellValue("Аркуш1", fmt.Sprintf("B%d", row))
			if err != nil {
				continue
			}
			label = strings.ToLower(strings.TrimSpace(label))
			if label == "вказати кількість" || label == "всього" || label == "всього:" {
				result[appendix] = row
				break
			}
			if fallback == 0 && (strings.Contains(label, "вказати кількість") || strings.Contains(label, "всього")) {
				fallback = row
			}
		}
		if result[appendix] == start && fallback != 0 {
			result[appendix] = fallback
		}
		if result[appendix] == start {
			appendixLabel := fmt.Sprintf("додаток %d.", appendix)
			for row := start; row < end; row++ {
				label, err := book.GetCellValue("Аркуш1", fmt.Sprintf("B%d", row))
				if err == nil && strings.ToLower(strings.TrimSpace(label)) == appendixLabel {
					result[appendix] = row
					break
				}
			}
		}
	}
	return result, slashRows, columnValues
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
