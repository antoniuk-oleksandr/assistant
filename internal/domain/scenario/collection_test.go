package scenario_test

import (
	"testing"

	"github.com/alex/assistent/internal/domain/scenario"
	"github.com/alex/assistent/internal/domain/submission"
)

func TestScienceReportCollectionProcessor(t *testing.T) {
	badStatuses := []submission.MaterialStatus{submission.Invalid, submission.AccessDenied, submission.NotFound, submission.Failed}
	for _, status := range badStatuses {
		t.Run(string(status)+" produces a partial result", func(t *testing.T) {
			result, err := (scenario.ScienceReportCollectionProcessor{}).Process(scenario.Collection{Items: []submission.Submission{
				{State: submission.Submitted, Materials: []submission.Material{{Status: status}}},
			}})
			if err != nil || result.Outcome != scenario.Partial || result.Processed != 1 || result.Failed != 1 {
				t.Fatalf("result = %#v, err = %v", result, err)
			}
		})
	}

	result, err := (scenario.ScienceReportCollectionProcessor{}).Process(scenario.Collection{Items: []submission.Submission{
		{State: submission.New},
		{State: submission.Returned},
	}})
	if err != nil || result.Outcome != scenario.Failed || result.Failed != 2 || result.ErrorMessage == "" {
		t.Fatalf("no submitted result = %#v, err = %v", result, err)
	}

	result, err = (scenario.ScienceReportCollectionProcessor{}).Process(scenario.Collection{Items: []submission.Submission{
		{State: submission.Submitted, Materials: []submission.Material{{Status: submission.Parsed}}},
	}})
	if err != nil || result.Outcome != scenario.Completed || result.Processed != 1 || result.Failed != 0 {
		t.Fatalf("completed result = %#v, err = %v", result, err)
	}
}

func TestScienceReportCollectionProcessorAggregatesAppendicesAndSources(t *testing.T) {
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
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}
	if result.Outcome != scenario.Partial || result.Processed != 1 || result.Failed != 2 || result.ReportData == nil {
		t.Fatalf("result = %#v", result)
	}
	report := result.ReportData
	if report.ScenarioID != 11 || report.TaskID != 22 || len(report.Appendices) != 28 {
		t.Fatalf("report identity/appendix count = %#v", report)
	}
	if report.Appendices[0].NumericTotal != 4 || len(report.Appendices[0].TextValues) != 1 || report.Appendices[0].TextValues[0] != "alpha" {
		t.Fatalf("appendix 1 = %#v", report.Appendices[0])
	}
	if report.Appendices[1].NumericTotal != 3.5 || len(report.Appendices[1].TextValues) != 1 || report.Appendices[1].TextValues[0] != "beta" {
		t.Fatalf("appendix 2 = %#v", report.Appendices[1])
	}
	if report.Appendices[27].NumericTotal != 0 || len(report.Appendices[27].TextValues) != 2 || !sameStrings(report.Appendices[27].TextValues, []string{"count=4", "title=award"}) || len(report.Appendices[27].Sources) != 1 {
		t.Fatalf("appendix 28 = %#v", report.Appendices[27])
	}
	if len(report.SupportingEvidence) != 1 || report.SupportingEvidence[0].MaterialID != "evidence" || report.SupportingEvidence[0].URI != "https://example.test/evidence" {
		t.Fatalf("supporting evidence = %#v", report.SupportingEvidence)
	}
	if len(report.MissingSubmissions) != 1 || report.MissingSubmissions[0] != "missing" || len(report.InvalidMaterials) != 1 || report.InvalidMaterials[0].MaterialID != "bad" || report.InvalidMaterials[0].Reason != string(submission.Invalid) {
		t.Fatalf("missing/invalid = %#v / %#v", report.MissingSubmissions, report.InvalidMaterials)
	}
}

func sameStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	seen := make(map[string]bool, len(left))
	for _, value := range left {
		seen[value] = true
	}
	for _, value := range right {
		if !seen[value] {
			return false
		}
	}
	return true
}

func TestScienceReportCollectionProcessorRecordsConflictingLocatedValues(t *testing.T) {
	appendix := 1
	result, err := (scenario.ScienceReportCollectionProcessor{}).Process(scenario.Collection{Items: []submission.Submission{{
		ExternalID: "submission-1", State: submission.Submitted, Materials: []submission.Material{{
			Status: submission.Parsed, Role: "appendix", ParseOutcomes: []submission.MaterialParse{{Result: submission.ParseResult{Document: &submission.ParsedDocument{
				Source: submission.DocumentSource{AppendixNumber: &appendix},
				Fields: []submission.Field{{Value: "first", Location: submission.Location{Paragraph: 1}}, {Value: "second", Location: submission.Location{Paragraph: 1}}},
			}}}},
		}},
	}}})
	if err != nil || result.ReportData == nil || len(result.ReportData.Appendices[0].Conflicts) != 1 {
		t.Fatalf("result = %#v, err = %v", result, err)
	}
	conflict := result.ReportData.Appendices[0].Conflicts[0]
	if conflict.Appendix != 1 || len(conflict.Values) != 2 || conflict.Values[0] != "first" || conflict.Values[1] != "second" {
		t.Fatalf("conflict = %#v", conflict)
	}
}

func TestScienceReportCollectionProcessorCountsDOCXTableRecords(t *testing.T) {
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
			}}},
		}}}}}},
	}}})
	if err != nil || result.ReportData == nil {
		t.Fatalf("Process() = %#v, %v", result, err)
	}
	if got := result.ReportData.Appendices[0].NumericTotal; got != 2 {
		t.Fatalf("numeric total = %v, want 2", got)
	}
}

func TestRegistryResolvesProcessors(t *testing.T) {
	processor := scenario.ScienceReportCollectionProcessor{}
	registry := scenario.NewRegistryWithProcessors(
		[]scenario.Implementation{{Name: "science-report", Type: "ScienceReportScenario"}},
		map[string]scenario.CollectionProcessor{"science-report": processor},
	)
	resolved, err := registry.ResolveProcessor("science-report")
	if err != nil || resolved == nil {
		t.Fatalf("ResolveProcessor() = %v, %v", resolved, err)
	}
	if _, err := scenario.NewRegistryWithProcessors([]scenario.Implementation{{Name: "science-report"}}, nil).ResolveProcessor("science-report"); err == nil {
		t.Fatal("ResolveProcessor() unexpectedly succeeded without a processor")
	}
	for _, name := range []string{"unknown", "missing-processor"} {
		registry := scenario.NewRegistryWithProcessors([]scenario.Implementation{{Name: "other"}}, nil)
		if _, err := registry.ResolveProcessor(name); err == nil {
			t.Fatalf("ResolveProcessor(%q) unexpectedly succeeded", name)
		}
	}
}
