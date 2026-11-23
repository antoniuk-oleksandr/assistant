package mappers_test

import (
	"github.com/alex/assistent/internal/adapters/mappers"
	"github.com/alex/assistent/internal/adapters/models"
	"github.com/alex/assistent/internal/domain/submission"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Submission mapping", func() {
	It("preserves submission and material metadata", func() {
		appendix := 7
		result := mappers.ToSubmission(models.SubmissionRow{ID: 4, TaskID: 8, ExternalID: "submission-1", SubmitterID: "user-1", SubmitterEmail: "person@example.com", SubmitterName: "Person", State: "submitted", SubmittedAt: "submitted", CreatedAt: "created", UpdatedAt: "updated", Late: true}, []models.MaterialRow{{ID: 5, ExternalID: "file-1", Name: "evidence.pdf", MIMEType: "application/pdf", SourceURI: "https://drive.example/file-1", LocalPath: "/tmp/evidence.pdf", AppendixNumber: &appendix, Role: "supporting_evidence", Status: "access_denied"}})
		Expect(result.State).To(Equal(submission.Submitted))
		Expect(result.SubmitterEmail).To(Equal("person@example.com"))
		Expect(result.Late).To(BeTrue())
		Expect(result.Materials).To(HaveLen(1))
		material := result.Materials[0]
		Expect(material.ExternalID).To(Equal("file-1"))
		Expect(material.Name).To(Equal("evidence.pdf"))
		Expect(material.MIMEType).To(Equal("application/pdf"))
		Expect(material.SourceURI).To(Equal("https://drive.example/file-1"))
		Expect(material.LocalPath).To(Equal("/tmp/evidence.pdf"))
		Expect(material.AppendixNumber).NotTo(BeNil())
		Expect(*material.AppendixNumber).To(Equal(appendix))
		Expect(material.Status).To(Equal(submission.AccessDenied))
	})
})
