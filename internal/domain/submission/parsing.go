package submission

import (
	"fmt"
	"math"
	"strings"
)

// ParseContext identifies the binding and the submission associated with a
// material. Parsers can use it alongside the material's role and appendix.
type ParseContext struct {
	ScenarioID                  int64 `json:",omitempty"`
	ScenarioName, ScenarioType  string
	CourseID, TaskExternalID    string
	SubmissionExternalID        string
	SubmitterID, SubmitterName  string
	SubmitterEmail, SubmittedAt string
}

type ParseIssue string

const (
	UnsupportedMaterial   ParseIssue = "unsupported_material"
	ParseFailed           ParseIssue = "parse_failed"
	AnalysisInvalid       ParseIssue = "analysis_invalid"
	AnalysisLowConfidence ParseIssue = "analysis_low_confidence"
)

const (
	AnalysisSchemaVersion     = "material-analysis.v1"
	MinimumAnalysisConfidence = 0.75
)

// ExtractionProposal is the versioned, provider-neutral contract returned by
// unknown-material analyzers. Fields remain structured JSON so candidate
// values can be retained without being accepted as facts.
type ExtractionProposal struct {
	SchemaVersion        string         `json:"schema_version"`
	Appendix             *int           `json:"appendix,omitempty"`
	DocumentType         string         `json:"document_type"`
	Fields               map[string]any `json:"fields"`
	IsSupportingEvidence bool           `json:"is_supporting_evidence"`
	Confidence           float64        `json:"confidence"`
}

// AnalysisResult records validation separately from the analyzer's proposal.
// A result is eligible for aggregation only when both checks pass.
type AnalysisResult struct {
	Proposal      ExtractionProposal `json:"proposal"`
	Validated     bool               `json:"validated"`
	LowConfidence bool               `json:"low_confidence"`
}

func (r AnalysisResult) EligibleForAggregation() bool {
	validated, err := ValidateExtractionProposal(r.Proposal)
	return err == nil && r.Validated && validated.Validated && !validated.LowConfidence
}

func ValidateExtractionProposal(proposal ExtractionProposal) (AnalysisResult, error) {
	if proposal.SchemaVersion != AnalysisSchemaVersion {
		return AnalysisResult{Proposal: proposal}, fmt.Errorf("unsupported analysis schema %q", proposal.SchemaVersion)
	}
	if strings.TrimSpace(proposal.DocumentType) == "" {
		return AnalysisResult{Proposal: proposal}, fmt.Errorf("analysis document type is required")
	}
	if proposal.Appendix != nil && (*proposal.Appendix < 1 || *proposal.Appendix > 28) {
		return AnalysisResult{Proposal: proposal}, fmt.Errorf("analysis appendix must be between 1 and 28")
	}
	if proposal.Fields == nil {
		return AnalysisResult{Proposal: proposal}, fmt.Errorf("analysis fields are required")
	}
	if math.IsNaN(proposal.Confidence) || math.IsInf(proposal.Confidence, 0) || proposal.Confidence < 0 || proposal.Confidence > 1 {
		return AnalysisResult{Proposal: proposal}, fmt.Errorf("analysis confidence must be between 0 and 1")
	}
	return AnalysisResult{Proposal: proposal, Validated: true, LowConfidence: proposal.Confidence < MinimumAnalysisConfidence}, nil
}

// ParseResult describes the parse of the downloaded snapshot, independently
// of later retrieval attempts. A nil result means parsing was not attempted.
type ParseResult struct {
	ParserType string          `json:"parser_type,omitempty"`
	Issue      ParseIssue      `json:"issue,omitempty"`
	Document   *ParsedDocument `json:"document,omitempty"`
	Analysis   *AnalysisResult `json:"analysis,omitempty"`
}

func (r ParseResult) EligibleForAggregation() bool {
	if r.Analysis != nil {
		return r.Analysis.EligibleForAggregation()
	}
	return r.Document != nil && r.Issue == ""
}

// MaterialParse records the outcome for one binding and snapshot. An empty
// source path describes an unsupported source with no local export.
type MaterialParse struct {
	Source DocumentSource `json:"source"`
	Result ParseResult    `json:"result"`
}

func (m MaterialParse) EligibleForAggregation() bool {
	return m.Result.EligibleForAggregation()
}

type ParsedDocument struct {
	Source   DocumentSource `json:"source"`
	Fields   []Field        `json:"fields,omitempty"`
	Tables   []Table        `json:"tables,omitempty"`
	Warnings []string       `json:"warnings,omitempty"`
}

type DocumentSource struct {
	ParseContext
	ExternalID     string `json:"external_id,omitempty"`
	Name           string `json:"name"`
	URI            string `json:"uri,omitempty"`
	MIMEType       string `json:"mime_type,omitempty"`
	LocalPath      string `json:"local_path"`
	Role           string `json:"role,omitempty"`
	AppendixNumber *int   `json:"appendix_number,omitempty"`
}

func (m Material) DocumentSource(origin ParseContext) DocumentSource {
	return DocumentSource{
		ParseContext: origin, ExternalID: m.ExternalID, Name: m.Name,
		URI: m.SourceURI, MIMEType: m.MIMEType, LocalPath: m.LocalPath,
		Role: m.Role, AppendixNumber: m.AppendixNumber,
	}
}

// Field captures extracted text and its original location. Scenario-specific
// field names, value types, and appendix classification are separate concerns.
type Field struct {
	Value    string   `json:"value"`
	Location Location `json:"location"`
}

type Table struct {
	Name  string    `json:"name,omitempty"`
	Index int       `json:"index"`
	Rows  [][]Field `json:"rows,omitempty"`
}

// Location uses one-based indexes, with zero meaning not applicable.
type Location struct {
	Paragraph int `json:"paragraph,omitempty"`
	Table     int `json:"table,omitempty"`
	Row       int `json:"row,omitempty"`
	Column    int `json:"column,omitempty"`
}
