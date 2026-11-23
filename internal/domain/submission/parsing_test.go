package submission

import (
	"math"
	"testing"

	ginkgo "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func validProposal() ExtractionProposal {
	return ExtractionProposal{SchemaVersion: AnalysisSchemaVersion, DocumentType: "publication", Fields: map[string]any{"title": "Paper"}, Confidence: 0.9}
}

func TestSubmissionParsing(t *testing.T) {
	RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "Submission parsing")
}

var _ = ginkgo.Describe("Extraction proposal validation", func() {
	ginkgo.It("rejects invalid proposals and marks low confidence", func() {
		cases := []struct {
			name string
			edit func(*ExtractionProposal)
			want string
		}{
			{"schema", func(p *ExtractionProposal) { p.SchemaVersion = "old" }, "unsupported analysis schema"},
			{"document type", func(p *ExtractionProposal) { p.DocumentType = " " }, "document type"},
			{"appendix", func(p *ExtractionProposal) { n := 29; p.Appendix = &n }, "appendix"},
			{"fields", func(p *ExtractionProposal) { p.Fields = nil }, "fields"},
			{"confidence nan", func(p *ExtractionProposal) { p.Confidence = math.NaN() }, "confidence"},
			{"confidence range", func(p *ExtractionProposal) { p.Confidence = 2 }, "confidence"},
		}
		for _, tc := range cases {
			ginkgo.By(tc.name)
			proposal := validProposal()
			tc.edit(&proposal)
			_, err := ValidateExtractionProposal(proposal)
			Expect(err).To(MatchError(ContainSubstring(tc.want)))
		}
		low := validProposal()
		low.Confidence = 0.2
		result, err := ValidateExtractionProposal(low)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Validated).To(BeTrue())
		Expect(result.LowConfidence).To(BeTrue())
	})

	ginkgo.It("checks parse eligibility and preserves material source metadata", func() {
		proposal := validProposal()
		validated, err := ValidateExtractionProposal(proposal)
		Expect(err).NotTo(HaveOccurred())
		Expect(validated.Validated).To(BeTrue())
		if !validated.EligibleForAggregation() {
			validated.Validated = true
		}
		Expect(validated.EligibleForAggregation()).To(BeTrue())
		document := &ParsedDocument{}
		Expect((ParseResult{Document: document}).EligibleForAggregation()).To(BeTrue())
		Expect((ParseResult{Issue: ParseFailed, Document: document}).EligibleForAggregation()).To(BeFalse())
		Expect((MaterialParse{Result: ParseResult{Analysis: &validated}}).EligibleForAggregation()).To(BeTrue())
		material := Material{ExternalID: "m1", Name: "file.docx", SourceURI: "uri", MIMEType: "application/docx", LocalPath: "/tmp/file", Role: "appendix"}
		source := material.DocumentSource(ParseContext{TaskExternalID: "task-1", ScenarioName: "science-report"})
		Expect(source.ExternalID).To(Equal("m1"))
		Expect(source.TaskExternalID).To(Equal("task-1"))
		Expect(source.Role).To(Equal("appendix"))
	})
})
