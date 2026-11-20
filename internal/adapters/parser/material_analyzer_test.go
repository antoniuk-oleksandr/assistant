package parser_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/alex/assistent/internal/adapters/parser"
	"github.com/alex/assistent/internal/domain/submission"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestMaterialAnalyzer(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Material analyzer")
}

var _ = Describe("Material analyzer", func() {
	It("classifies a certificate from metadata and retains auditable context", func() {
		analyzer := parser.NewMaterialAnalyzer()
		proposal, err := analyzer.Analyze(context.Background(), submission.Material{
			ExternalID: "material-1",
			Name:       "Award certificate.bin",
			MIMEType:   "application/octet-stream",
			SourceURI:  "https://example.test/material-1",
			Role:       "supporting",
		}, submission.ParseContext{ScenarioName: "science-report", ScenarioType: "ScienceReportScenario"})

		Expect(err).NotTo(HaveOccurred())
		Expect(proposal.SchemaVersion).To(Equal(submission.AnalysisSchemaVersion))
		Expect(proposal.DocumentType).To(Equal("certificate"))
		Expect(proposal.IsSupportingEvidence).To(BeTrue())
		Expect(proposal.Confidence).To(Equal(0.25))
		Expect(proposal.Fields).To(HaveKeyWithValue("source_uri", "https://example.test/material-1"))
		Expect(proposal.Fields).To(HaveKeyWithValue("scenario_name", "science-report"))
		Expect(proposal.Fields).To(HaveKeyWithValue("classification_basis", []string{"certificate name/content signal"}))
	})

	It("uses bounded local text signals when classifying an otherwise unknown material", func() {
		path := filepath.Join(GinkgoT().TempDir(), "opaque.txt")
		Expect(os.WriteFile(path, []byte("publication evidence"), 0o600)).To(Succeed())

		proposal, err := parser.NewMaterialAnalyzer().Analyze(context.Background(), submission.Material{
			Name:      "opaque.txt",
			MIMEType:  "text/plain",
			LocalPath: path,
		}, submission.ParseContext{})

		Expect(err).NotTo(HaveOccurred())
		Expect(proposal.DocumentType).To(Equal("publication"))
		Expect(proposal.Fields).To(HaveKeyWithValue("content_available", true))
		Expect(proposal.Fields).To(HaveKeyWithValue("classification_basis", []string{"publication name/content signal"}))
	})

	It("identifies a valid appendix from bound scientific-report context", func() {
		appendix := 3
		proposal, err := parser.NewMaterialAnalyzer().Analyze(context.Background(), submission.Material{
			Name:           "unlabeled.bin",
			MIMEType:       "application/octet-stream",
			AppendixNumber: &appendix,
			Role:           "appendix",
		}, submission.ParseContext{ScenarioName: "science-report", ScenarioType: "ScienceReportScenario"})

		Expect(err).NotTo(HaveOccurred())
		Expect(proposal.DocumentType).To(Equal("scientific_report_appendix"))
		Expect(proposal.Appendix).NotTo(BeNil())
		Expect(*proposal.Appendix).To(Equal(3))
		Expect(proposal.Fields).To(HaveKeyWithValue("appendix", 3))
		Expect(proposal.Fields).To(HaveKeyWithValue("classification_basis", []string{"material appendix number"}))
		Expect(proposal.IsSupportingEvidence).To(BeFalse())
	})

	It("retains a neutral unknown-material classification without scenario context", func() {
		proposal, err := parser.NewMaterialAnalyzer().Analyze(context.Background(), submission.Material{}, submission.ParseContext{})

		Expect(err).NotTo(HaveOccurred())
		Expect(proposal.DocumentType).To(Equal("unknown_material"))
		Expect(proposal.Fields).To(HaveKeyWithValue("classification_basis", []string{"material metadata only"}))
		Expect(proposal.Fields).To(HaveKeyWithValue("content_available", false))
	})

	It("returns context cancellation before inspecting the material", func() {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := parser.NewMaterialAnalyzer().Analyze(ctx, submission.Material{LocalPath: "/not-read"}, submission.ParseContext{})
		Expect(err).To(MatchError(context.Canceled))
	})
})
