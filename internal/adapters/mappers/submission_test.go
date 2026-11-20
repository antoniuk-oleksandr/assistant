package mappers_test

import (
	"testing"

	"github.com/alex/assistent/internal/adapters/mappers"
	"github.com/alex/assistent/internal/adapters/models"
	"github.com/alex/assistent/internal/domain/submission"
)

func TestToSubmissionPreservesMetadata(t *testing.T) {
	appendix := 7
	result := mappers.ToSubmission(models.SubmissionRow{ID: 4, TaskID: 8, ExternalID: "submission-1", SubmitterID: "user-1", SubmitterEmail: "person@example.com", SubmitterName: "Person", State: "submitted", SubmittedAt: "submitted", CreatedAt: "created", UpdatedAt: "updated", Late: true}, []models.MaterialRow{{ID: 5, ExternalID: "file-1", Name: "evidence.pdf", MIMEType: "application/pdf", SourceURI: "https://drive.example/file-1", LocalPath: "/tmp/evidence.pdf", AppendixNumber: &appendix, Role: "supporting_evidence", Status: "access_denied"}})
	if result.State != submission.Submitted || result.SubmitterEmail != "person@example.com" || !result.Late || len(result.Materials) != 1 {
		t.Fatalf("mapped submission lost metadata: %#v", result)
	}
	material := result.Materials[0]
	if material.ExternalID != "file-1" || material.Name != "evidence.pdf" || material.MIMEType != "application/pdf" || material.SourceURI != "https://drive.example/file-1" || material.LocalPath != "/tmp/evidence.pdf" || material.AppendixNumber == nil || *material.AppendixNumber != appendix || material.Status != submission.AccessDenied {
		t.Fatalf("mapped material lost metadata: %#v", material)
	}
}
