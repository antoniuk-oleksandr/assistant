package submission

type State string

const (
	New       State = "new"
	Created   State = "created"
	Submitted State = "submitted"
	Returned  State = "returned"
	Unknown   State = "unknown"
)

type MaterialStatus string

const (
	Pending      MaterialStatus = "pending"
	Parsed       MaterialStatus = "parsed"
	Invalid      MaterialStatus = "invalid"
	AccessDenied MaterialStatus = "access_denied"
	NotFound     MaterialStatus = "not_found"
	Failed       MaterialStatus = "failed"
)

type Material struct {
	ID             int64
	ExternalID     string
	Name           string
	MIMEType       string
	SizeBytes      *int64
	SourceURI      string
	LocalPath      string
	AppendixNumber *int
	Role           string
	Status         MaterialStatus
	Parsing        *ParseResult // unscoped outcome or unsupported-source marker from retrieval
	ParseOutcomes  []MaterialParse
}

type Submission struct {
	ID             int64
	TaskID         int64
	TaskExternalID string
	ExternalID     string
	SubmitterID    string
	SubmitterEmail string
	SubmitterName  string
	State          State
	SubmittedAt    string
	CreatedAt      string
	UpdatedAt      string
	Late           bool
	Materials      []Material
}
