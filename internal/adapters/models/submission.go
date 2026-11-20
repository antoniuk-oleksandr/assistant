package models

type SubmissionRow struct {
	ID, TaskID                                             int64
	ExternalID, SubmitterID, SubmitterEmail, SubmitterName string
	State, SubmittedAt, CreatedAt, UpdatedAt               string
	Late                                                   bool
}

type MaterialRow struct {
	ID, SubmissionID                                               int64
	ExternalID, Name, MIMEType, SourceURI, LocalPath, Role, Status string
	SizeBytes                                                      *int64
	AppendixNumber                                                 *int
	ParseResult                                                    *string
}
