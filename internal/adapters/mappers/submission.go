package mappers

import (
	"github.com/alex/assistent/internal/adapters/models"
	"github.com/alex/assistent/internal/domain/submission"
)

func ToSubmission(row models.SubmissionRow, materials []models.MaterialRow) submission.Submission {
	result := submission.Submission{ID: row.ID, TaskID: row.TaskID, ExternalID: row.ExternalID, SubmitterID: row.SubmitterID, SubmitterEmail: row.SubmitterEmail, SubmitterName: row.SubmitterName, State: submission.State(row.State), SubmittedAt: row.SubmittedAt, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, Late: row.Late}
	result.Materials = make([]submission.Material, 0, len(materials))
	for _, material := range materials {
		result.Materials = append(result.Materials, submission.Material{ID: material.ID, ExternalID: material.ExternalID, Name: material.Name, MIMEType: material.MIMEType, SizeBytes: material.SizeBytes, SourceURI: material.SourceURI, LocalPath: material.LocalPath, AppendixNumber: material.AppendixNumber, Role: material.Role, Status: submission.MaterialStatus(material.Status)})
	}
	return result
}
