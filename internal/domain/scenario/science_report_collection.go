package scenario

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/alex/assistent/internal/domain/submission"
)

const scienceAppendixCount = 28

// ScienceReportCollectionProcessor applies the scientific-report rules to the
// normalized collection and returns structured data for an outer-layer
// renderer. It does not know about files, JSON, SQLite, or Google APIs.
type ScienceReportCollectionProcessor struct{}

type ReportData struct {
	ScenarioID         int64                                `json:"scenario_id"`
	TaskID             int64                                `json:"task_id"`
	Appendices         [scienceAppendixCount]ReportAppendix `json:"appendices"`
	SupportingEvidence []ReportSource                       `json:"supporting_evidence"`
	MissingSubmissions []string                             `json:"missing_submissions"`
	InvalidMaterials   []ReportSource                       `json:"invalid_materials"`
	Conflicts          []ReportConflict                     `json:"conflicts"`
	CourseData         map[string]string                    `json:"course_data,omitempty"`
}

type ReportConflict struct {
	Appendix int          `json:"appendix"`
	Source   ReportSource `json:"source"`
	Values   []string     `json:"values"`
}

type ReportAppendix struct {
	Number       int              `json:"number"`
	NumericTotal float64          `json:"numeric_total"`
	TextValues   []string         `json:"text_values"`
	Sources      []ReportSource   `json:"sources"`
	Conflicts    []ReportConflict `json:"conflicts"`
}

type ReportSource struct {
	SubmissionID string `json:"submission_id"`
	Submitter    string `json:"submitter"`
	MaterialID   string `json:"material_id"`
	Name         string `json:"name"`
	URI          string `json:"uri,omitempty"`
	LocalPath    string `json:"local_path,omitempty"`
	Reason       string `json:"reason,omitempty"`
}

func (ScienceReportCollectionProcessor) Process(collection Collection) (ProcessResult, error) {
	report := ReportData{ScenarioID: collection.ScenarioID, TaskID: collection.TaskID}
	for index := range report.Appendices {
		report.Appendices[index].Number = index + 1
		report.Appendices[index].TextValues = []string{}
		report.Appendices[index].Sources = []ReportSource{}
	}

	result := ProcessResult{}
	for _, item := range collection.Items {
		if item.State != submission.Submitted {
			result.Failed++
			if item.ExternalID != "" {
				report.MissingSubmissions = append(report.MissingSubmissions, item.ExternalID)
			}
			continue
		}
		result.Processed++
		for _, material := range item.Materials {
			source := makeScienceSource(item, material)
			if isInvalid(material.Status) {
				result.Failed++
				source.Reason = string(material.Status)
				report.InvalidMaterials = append(report.InvalidMaterials, source)
				continue
			}
			for _, parsed := range material.ParseOutcomes {
				if parsed.EligibleForAggregation() {
					aggregateParsed(&report, source, parsed)
				}
			}
			if material.Parsing != nil && material.Parsing.EligibleForAggregation() {
				aggregateParseResult(&report, source, *material.Parsing)
			}
			if material.Role != "appendix" {
				report.SupportingEvidence = append(report.SupportingEvidence, source)
			}
		}
	}
	if result.Processed == 0 {
		result.Outcome = Failed
		result.ErrorMessage = "no submitted submissions available for processing"
		result.ReportData = &report
		return result, nil
	}
	if result.Failed > 0 {
		result.Outcome = Partial
	} else {
		result.Outcome = Completed
	}
	result.ReportData = &report
	return result, nil
}

func aggregateParsed(report *ReportData, source ReportSource, parsed submission.MaterialParse) {
	aggregateParseResult(report, source, parsed.Result)
}

func aggregateParseResult(report *ReportData, source ReportSource, parsed submission.ParseResult) {
	if parsed.Document != nil {
		appendix := parsed.Document.Source.AppendixNumber
		if !validAppendix(appendix) {
			return
		}
		reportAppendix := &report.Appendices[*appendix-1]
		aggregateFields(reportAppendix, source, parsed.Document.Fields)
		rows := countDocumentRows(parsed.Document.Tables)
		if rows > 0 {
			reportAppendix.NumericTotal += float64(rows)
			if len(reportAppendix.Sources) == 0 {
				reportAppendix.Sources = append(reportAppendix.Sources, source)
			}
		}
		return
	}
	if parsed.Analysis != nil && validAppendix(parsed.Analysis.Proposal.Appendix) {
		appendix := *parsed.Analysis.Proposal.Appendix
		fields := make([]submission.Field, 0, len(parsed.Analysis.Proposal.Fields))
		for key, value := range parsed.Analysis.Proposal.Fields {
			fields = append(fields, submission.Field{Value: key + "=" + fmt.Sprint(value)})
		}
		aggregateFields(&report.Appendices[appendix-1], source, fields)
	}
}

// countDocumentRows counts populated records in DOCX tables. The first
// populated row is a header; section labels and numeric-only header rows are
// not records. Scientific-report appendices use this count for their main
// "всього" value, while the remaining cell contents stay as source evidence.
func countDocumentRows(tables []submission.Table) int {
	count := 0
	for _, table := range tables {
		headerSeen := false
		for _, row := range table.Rows {
			values := make([]string, 0, len(row))
			nonNumeric := false
			for _, field := range row {
				value := strings.TrimSpace(field.Value)
				if value == "" {
					continue
				}
				values = append(values, value)
				if _, err := strconv.ParseFloat(strings.ReplaceAll(value, ",", "."), 64); err != nil {
					nonNumeric = true
				}
			}
			if len(values) == 0 {
				continue
			}
			if !headerSeen {
				headerSeen = true
				continue
			}
			if len(values) >= 2 && nonNumeric {
				count++
			}
		}
	}
	return count
}

func aggregateFields(appendix *ReportAppendix, source ReportSource, fields []submission.Field) {
	seen := make(map[string]string)
	for _, field := range fields {
		value := strings.TrimSpace(field.Value)
		if value == "" {
			continue
		}
		location := fmt.Sprintf("%d:%d:%d:%d", field.Location.Paragraph, field.Location.Table, field.Location.Row, field.Location.Column)
		if location != "0:0:0:0" {
			if previous, ok := seen[location]; ok && previous != value {
				appendix.Conflicts = append(appendix.Conflicts, ReportConflict{Appendix: appendix.Number, Source: source, Values: []string{previous, value}})
			}
			seen[location] = value
		}
		if number, err := strconv.ParseFloat(value, 64); err == nil {
			appendix.NumericTotal += number
		} else if !contains(appendix.TextValues, value) {
			appendix.TextValues = append(appendix.TextValues, value)
		}
	}
	appendix.Sources = append(appendix.Sources, source)
}

func makeScienceSource(item submission.Submission, material submission.Material) ReportSource {
	return ReportSource{SubmissionID: item.ExternalID, Submitter: firstNonEmpty(item.SubmitterEmail, item.SubmitterName, item.SubmitterID), MaterialID: material.ExternalID, Name: material.Name, URI: material.SourceURI, LocalPath: material.LocalPath}
}

func isInvalid(status submission.MaterialStatus) bool {
	switch status {
	case submission.Invalid, submission.AccessDenied, submission.NotFound, submission.Failed:
		return true
	default:
		return false
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func validAppendix(number *int) bool {
	return number != nil && *number >= 1 && *number <= scienceAppendixCount
}
