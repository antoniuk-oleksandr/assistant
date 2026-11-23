package scenario_test

import (
	"github.com/alex/assistent/internal/domain/scenario"
	"github.com/alex/assistent/internal/domain/submission"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("science report collection processor", func() {
	It("handles partial, failed, and completed submissions", func() {
		badStatuses := []submission.MaterialStatus{submission.Invalid, submission.AccessDenied, submission.NotFound, submission.Failed}
		for _, status := range badStatuses {
			By(string(status) + " produces a partial result")
			result, err := (scenario.ScienceReportCollectionProcessor{}).Process(scenario.Collection{Items: []submission.Submission{
				{State: submission.Submitted, Materials: []submission.Material{{Status: status}}},
			}})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Outcome).To(Equal(scenario.Partial))
			Expect(result.Processed).To(Equal(1))
			Expect(result.Failed).To(Equal(1))
		}

		result, err := (scenario.ScienceReportCollectionProcessor{}).Process(scenario.Collection{Items: []submission.Submission{
			{State: submission.New},
			{State: submission.Returned},
		}})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Outcome).To(Equal(scenario.Failed))
		Expect(result.Failed).To(Equal(2))
		Expect(result.ErrorMessage).NotTo(BeEmpty())

		result, err = (scenario.ScienceReportCollectionProcessor{}).Process(scenario.Collection{Items: []submission.Submission{
			{State: submission.Submitted, Materials: []submission.Material{{Status: submission.Parsed}}},
		}})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Outcome).To(Equal(scenario.Completed))
		Expect(result.Processed).To(Equal(1))
		Expect(result.Failed).To(Equal(0))
	})

	It("aggregates appendices and sources", func() {
		appendixOne := 1
		appendixTwo := 2
		appendixTwentyEight := 28
		appendixOutOfRange := 29
		result, err := (scenario.ScienceReportCollectionProcessor{}).Process(scenario.Collection{
			ScenarioID: 11,
			TaskID:     22,
			Items: []submission.Submission{
				{ExternalID: "missing", State: submission.Returned},
				{ExternalID: "staff-1", SubmitterEmail: "staff@example.com", State: submission.Submitted, Materials: []submission.Material{
					{
						ExternalID: "appendix-1", Name: "Appendix 1", SourceURI: "https://example.test/a1", LocalPath: "/tmp/a1", Role: "appendix", Status: submission.Parsed,
						ParseOutcomes: []submission.MaterialParse{{Result: submission.ParseResult{Document: &submission.ParsedDocument{
							Source: submission.DocumentSource{AppendixNumber: &appendixOne},
							Fields: []submission.Field{{Value: "2"}, {Value: "2"}, {Value: "alpha"}, {Value: "alpha"}, {Value: " "}},
						}}}},
					},
					{
						ExternalID: "appendix-2", Name: "Appendix 2", Role: "appendix", Status: submission.Parsed,
						Parsing: &submission.ParseResult{Document: &submission.ParsedDocument{
							Source: submission.DocumentSource{AppendixNumber: &appendixTwo},
							Fields: []submission.Field{{Value: "3.5"}, {Value: "beta"}},
						}},
					},
					{
						ExternalID: "evidence", Name: "Certificate", SourceURI: "https://example.test/evidence", Role: "supporting_evidence", Status: submission.Parsed,
						ParseOutcomes: []submission.MaterialParse{{Result: submission.ParseResult{Analysis: &submission.AnalysisResult{
							Proposal: submission.ExtractionProposal{SchemaVersion: submission.AnalysisSchemaVersion, Appendix: &appendixTwentyEight, DocumentType: "certificate", Fields: map[string]any{"count": 4, "title": "award"}, Confidence: 1}, Validated: true,
						}}}},
					},
					{ExternalID: "ignored", Name: "Out of range", Role: "appendix", Status: submission.Parsed,
						Parsing: &submission.ParseResult{Document: &submission.ParsedDocument{Source: submission.DocumentSource{AppendixNumber: &appendixOutOfRange}, Fields: []submission.Field{{Value: "999"}}}},
					},
					{ExternalID: "bad", Name: "Bad material", Role: "appendix", Status: submission.Invalid},
				}},
			},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Outcome).To(Equal(scenario.Partial))
		Expect(result.Processed).To(Equal(1))
		Expect(result.Failed).To(Equal(2))
		Expect(result.ReportData).NotTo(BeNil())
		report := result.ReportData
		Expect(report.ScenarioID).To(Equal(int64(11)))
		Expect(report.TaskID).To(Equal(int64(22)))
		Expect(report.Appendices).To(HaveLen(28))
		Expect(report.Appendices[0].NumericTotal).To(Equal(float64(4)))
		Expect(report.Appendices[0].TextValues).To(Equal([]string{"alpha"}))
		Expect(report.Appendices[1].NumericTotal).To(Equal(float64(3.5)))
		Expect(report.Appendices[1].TextValues).To(Equal([]string{"beta"}))
		Expect(report.Appendices[27].NumericTotal).To(Equal(float64(0)))
		Expect(report.Appendices[27].TextValues).To(ConsistOf("count=4", "title=award"))
		Expect(report.Appendices[27].Sources).To(HaveLen(1))
		Expect(report.SupportingEvidence).To(HaveLen(1))
		Expect(report.SupportingEvidence[0].MaterialID).To(Equal("evidence"))
		Expect(report.SupportingEvidence[0].URI).To(Equal("https://example.test/evidence"))
		Expect(report.MissingSubmissions).To(Equal([]string{"missing"}))
		Expect(report.InvalidMaterials).To(HaveLen(1))
		Expect(report.InvalidMaterials[0].MaterialID).To(Equal("bad"))
		Expect(report.InvalidMaterials[0].Reason).To(Equal(string(submission.Invalid)))
	})

	It("records conflicting located values", func() {
		appendix := 1
		result, err := (scenario.ScienceReportCollectionProcessor{}).Process(scenario.Collection{Items: []submission.Submission{{
			ExternalID: "submission-1", State: submission.Submitted, Materials: []submission.Material{{
				Status: submission.Parsed, Role: "appendix", ParseOutcomes: []submission.MaterialParse{{Result: submission.ParseResult{Document: &submission.ParsedDocument{
					Source: submission.DocumentSource{AppendixNumber: &appendix},
					Fields: []submission.Field{{Value: "first", Location: submission.Location{Paragraph: 1}}, {Value: "second", Location: submission.Location{Paragraph: 1}}},
				}}}},
			}},
		}}})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.ReportData).NotTo(BeNil())
		Expect(result.ReportData.Appendices[0].Conflicts).To(HaveLen(1))
		conflict := result.ReportData.Appendices[0].Conflicts[0]
		Expect(conflict.Appendix).To(Equal(1))
		Expect(conflict.Values).To(Equal([]string{"first", "second"}))
	})

	It("counts DOCX table records", func() {
		appendix := 1
		result, err := (scenario.ScienceReportCollectionProcessor{}).Process(scenario.Collection{Items: []submission.Submission{{
			State: submission.Submitted,
			Materials: []submission.Material{{Status: submission.Parsed, Role: "appendix", ParseOutcomes: []submission.MaterialParse{{Result: submission.ParseResult{Document: &submission.ParsedDocument{
				Source: submission.DocumentSource{AppendixNumber: &appendix},
				Tables: []submission.Table{{Rows: [][]submission.Field{
					{{Value: "№"}, {Value: "ПІБ"}},
					{{Value: "1"}, {Value: "Анна Коваленко"}},
					{{Value: "2"}, {Value: "Петро Петренко"}},
					{{Value: "1"}, {Value: "2"}}, // numeric-only header row
					{{Value: " "}, {Value: ""}},  // empty rows are not records
				}}},
			}}}}}},
		}}})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.ReportData).NotTo(BeNil())
		Expect(result.ReportData.Appendices[0].NumericTotal).To(Equal(float64(2)))
	})

	It("resolves processors from the registry", func() {
		processor := scenario.ScienceReportCollectionProcessor{}
		registry := scenario.NewRegistryWithProcessors(
			[]scenario.Implementation{{Name: "science-report", Type: "ScienceReportScenario"}},
			map[string]scenario.CollectionProcessor{"science-report": processor},
		)
		resolved, err := registry.ResolveProcessor("science-report")
		Expect(err).NotTo(HaveOccurred())
		Expect(resolved).NotTo(BeNil())
		_, err = scenario.NewRegistryWithProcessors([]scenario.Implementation{{Name: "science-report"}}, nil).ResolveProcessor("science-report")
		Expect(err).To(HaveOccurred())
		for _, name := range []string{"unknown", "missing-processor"} {
			registry := scenario.NewRegistryWithProcessors([]scenario.Implementation{{Name: "other"}}, nil)
			_, err := registry.ResolveProcessor(name)
			Expect(err).To(HaveOccurred())
		}
	})

})
