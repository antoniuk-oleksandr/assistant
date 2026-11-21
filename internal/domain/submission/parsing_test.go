package submission

import (
	"math"
	"strings"
	"testing"
)

func validProposal() ExtractionProposal {
	return ExtractionProposal{SchemaVersion: AnalysisSchemaVersion, DocumentType: "publication", Fields: map[string]any{"title": "Paper"}, Confidence: 0.9}
}

func TestValidateExtractionProposal(t *testing.T) {
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
		t.Run(tc.name, func(t *testing.T) {
			proposal := validProposal()
			tc.edit(&proposal)
			if _, err := ValidateExtractionProposal(proposal); err == nil || !contains(err.Error(), tc.want) {
				t.Fatalf("ValidateExtractionProposal() error = %v", err)
			}
		})
	}
	low := validProposal()
	low.Confidence = 0.2
	result, err := ValidateExtractionProposal(low)
	if err != nil || !result.Validated || !result.LowConfidence {
		t.Fatalf("low-confidence result = %#v, %v", result, err)
	}
}

func TestParseEligibilityAndMaterialSource(t *testing.T) {
	proposal := validProposal()
	validated, err := ValidateExtractionProposal(proposal)
	if err != nil || !validated.Validated {
		t.Fatal(err)
	}
	if !validated.EligibleForAggregation() {
		validated.Validated = true
		if !validated.EligibleForAggregation() {
			t.Fatal("validated proposal should be eligible")
		}
	}
	document := &ParsedDocument{}
	if !(ParseResult{Document: document}).EligibleForAggregation() {
		t.Fatal("parsed document should be eligible")
	}
	if (ParseResult{Issue: ParseFailed, Document: document}).EligibleForAggregation() {
		t.Fatal("failed parse should not be eligible")
	}
	if !(MaterialParse{Result: ParseResult{Analysis: &validated}}).EligibleForAggregation() {
		t.Fatal("valid material analysis should be eligible")
	}
	material := Material{ExternalID: "m1", Name: "file.docx", SourceURI: "uri", MIMEType: "application/docx", LocalPath: "/tmp/file", Role: "appendix"}
	source := material.DocumentSource(ParseContext{TaskExternalID: "task-1", ScenarioName: "science-report"})
	if source.ExternalID != "m1" || source.TaskExternalID != "task-1" || source.Role != "appendix" {
		t.Fatalf("source = %#v", source)
	}
}

func contains(value, part string) bool {
	return strings.Contains(value, part)
}
