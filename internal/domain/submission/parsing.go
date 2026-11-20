package submission

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
	UnsupportedMaterial ParseIssue = "unsupported_material"
	ParseFailed         ParseIssue = "parse_failed"
)

// ParseResult describes the parse of the downloaded snapshot, independently
// of later retrieval attempts. A nil result means parsing was not attempted.
type ParseResult struct {
	ParserType string          `json:"parser_type,omitempty"`
	Issue      ParseIssue      `json:"issue,omitempty"`
	Document   *ParsedDocument `json:"document,omitempty"`
}

// MaterialParse records the outcome for one binding and snapshot. An empty
// source path describes an unsupported source with no local export.
type MaterialParse struct {
	Source DocumentSource `json:"source"`
	Result ParseResult    `json:"result"`
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
