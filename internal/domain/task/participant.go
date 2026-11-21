package task

// Participant is an expected submitter in a Classroom course roster.
// ExternalID is the stable Classroom user ID; name and email are optional
// profile data and must not be used as the identity key.
type Participant struct {
	ExternalID string
	Name       string
	Email      string
}
