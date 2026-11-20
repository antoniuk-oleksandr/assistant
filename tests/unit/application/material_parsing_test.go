package application_test

import (
	"context"
	"errors"
	"math"
	"testing"

	"github.com/alex/assistent/internal/application"
	"github.com/alex/assistent/internal/domain/classroom"
	"github.com/alex/assistent/internal/domain/submission"
	"github.com/alex/assistent/internal/domain/task"
	"github.com/alex/assistent/mocks"
	"github.com/alex/assistent/tests/support/fixtures"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"
)

type materialAnalyzerFunc func(context.Context, submission.Material, submission.ParseContext) (submission.ExtractionProposal, error)

func (f materialAnalyzerFunc) Analyze(ctx context.Context, item submission.Material, origin submission.ParseContext) (submission.ExtractionProposal, error) {
	return f(ctx, item, origin)
}

func TestMaterialParsing(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Submitted material parsing")
}

var _ = Describe("Material parsing", func() {
	var (
		ctrl   *gomock.Controller
		ctx    context.Context
		item   submission.Material
		origin submission.ParseContext
	)
	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		ctx = context.Background()
		item = fixtures.SubmittedMaterial()
		origin = fixtures.MaterialOrigin()
	})

	DescribeTable("selects a context-specific parser ahead of the format fallback",
		func(scenarioType, parserType string) {
			origin.ScenarioType = scenarioType
			specific := mocks.NewMockDocumentParser(ctrl)
			fallback := mocks.NewMockDocumentParser(ctrl)
			matches := scenarioType == "scientific-report"
			specific.EXPECT().CanHandle(item, origin).Return(matches)
			selected := specific
			if !matches {
				fallback.EXPECT().CanHandle(item, origin).Return(true)
				selected = fallback
			}
			doc := submission.ParsedDocument{
				Source:   submission.DocumentSource{Name: "not the original name"},
				Fields:   []submission.Field{{Value: "12", Location: submission.Location{Paragraph: 2}}},
				Warnings: []string{"Missing optional field"},
			}
			selected.EXPECT().Type().Return(parserType)
			selected.EXPECT().Parse(ctx, item, origin).Return(doc, nil)

			result, err := application.NewMaterialParsing(specific, fallback).Parse(ctx, item, origin)
			Expect(err).NotTo(HaveOccurred())
			doc.Source = submission.DocumentSource{
				ParseContext: origin, ExternalID: item.ExternalID, Name: item.Name, URI: item.SourceURI,
				MIMEType: item.MIMEType, LocalPath: item.LocalPath, Role: item.Role, AppendixNumber: item.AppendixNumber,
			}
			Expect(result).To(Equal(submission.ParseResult{ParserType: parserType, Document: &doc}))
		},
		Entry("the scientific report binding", "scientific-report", "scientific-docx"),
		Entry("another binding with the same attachment", "other", "docx"),
	)

	It("returns cancellation rather than recording a parser failure", func() {
		ctx, cancel := context.WithCancel(ctx)
		DeferCleanup(cancel)
		selected := mocks.NewMockDocumentParser(ctrl)
		selected.EXPECT().CanHandle(item, origin).Return(true)
		selected.EXPECT().Type().Return("docx")
		selected.EXPECT().Parse(ctx, item, origin).DoAndReturn(func(context.Context, submission.Material, submission.ParseContext) (submission.ParsedDocument, error) {
			cancel()
			return submission.ParsedDocument{}, context.Canceled
		})
		result, err := application.NewMaterialParsing(selected).Parse(ctx, item, origin)
		Expect(err).To(MatchError(context.Canceled))
		Expect(result).To(Equal(submission.ParseResult{}))
	})

	It("records an analyzer failure as a parse issue", func() {
		analyzer := materialAnalyzerFunc(func(context.Context, submission.Material, submission.ParseContext) (submission.ExtractionProposal, error) {
			return submission.ExtractionProposal{}, errors.New("analysis unavailable")
		})

		result, err := application.NewMaterialParsingWithAnalyzer(analyzer).Parse(ctx, item, origin)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(submission.ParseResult{Issue: submission.ParseFailed}))
	})

	It("returns cancellation rather than recording an analyzer failure", func() {
		analyzer := materialAnalyzerFunc(func(ctx context.Context, _ submission.Material, _ submission.ParseContext) (submission.ExtractionProposal, error) {
			return submission.ExtractionProposal{}, ctx.Err()
		})
		ctx, cancel := context.WithCancel(ctx)
		cancel()

		result, err := application.NewMaterialParsingWithAnalyzer(analyzer).Parse(ctx, item, origin)
		Expect(err).To(MatchError(context.Canceled))
		Expect(result).To(Equal(submission.ParseResult{}))
	})

	DescribeTable("validates analysis for an unknown material before aggregation",
		func(proposal submission.ExtractionProposal, expectedIssue submission.ParseIssue, expectedValidated, expectedLowConfidence, expectedEligible bool) {
			analyzer := materialAnalyzerFunc(func(context.Context, submission.Material, submission.ParseContext) (submission.ExtractionProposal, error) {
				return proposal, nil
			})

			result, err := application.NewMaterialParsingWithAnalyzer(analyzer).Parse(ctx, item, origin)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Issue).To(Equal(expectedIssue))
			Expect(result.EligibleForAggregation()).To(Equal(expectedEligible))
			if expectedValidated {
				Expect(result.Analysis).NotTo(BeNil())
				Expect(result.Analysis.Validated).To(BeTrue())
				Expect(result.Analysis.LowConfidence).To(Equal(expectedLowConfidence))
				Expect(result.Analysis.Proposal.Fields).To(Equal(proposal.Fields))
			} else {
				Expect(result.Analysis).NotTo(BeNil())
				Expect(result.Analysis.Validated).To(BeFalse())
				Expect(result.Analysis.Proposal).To(Equal(proposal))
			}
		},
		Entry("accepts validated high-confidence structured analysis", submission.ExtractionProposal{
			SchemaVersion: submission.AnalysisSchemaVersion, DocumentType: "certificate",
			Fields: map[string]any{"title": "Research award"}, Confidence: 0.9,
		}, submission.ParseIssue(""), true, false, true),
		Entry("retains validated low-confidence analysis but excludes it", submission.ExtractionProposal{
			SchemaVersion: submission.AnalysisSchemaVersion, DocumentType: "certificate",
			Fields: map[string]any{"title": "Research award"}, Confidence: 0.5,
		}, submission.AnalysisLowConfidence, true, true, false),
		Entry("excludes an unvalidated analysis", submission.ExtractionProposal{
			SchemaVersion: "unknown", DocumentType: "certificate",
			Fields: map[string]any{"title": "Research award"}, Confidence: 0.9,
		}, submission.AnalysisInvalid, false, false, false),
	)

	DescribeTable("rejects malformed proposals even when analyzer flags claim validity",
		func(proposal submission.ExtractionProposal) {
			result := submission.ParseResult{Analysis: &submission.AnalysisResult{
				Proposal: proposal, Validated: true, LowConfidence: false,
			}}
			Expect(result.EligibleForAggregation()).To(BeFalse())
		},
		Entry("missing schema version", submission.ExtractionProposal{
			DocumentType: "certificate", Fields: map[string]any{"title": "Award"}, Confidence: 0.9,
		}),
		Entry("missing document type", submission.ExtractionProposal{
			SchemaVersion: submission.AnalysisSchemaVersion, Fields: map[string]any{"title": "Award"}, Confidence: 0.9,
		}),
		Entry("nil fields", submission.ExtractionProposal{
			SchemaVersion: submission.AnalysisSchemaVersion, DocumentType: "certificate", Confidence: 0.9,
		}),
		Entry("appendix below range", submission.ExtractionProposal{
			SchemaVersion: submission.AnalysisSchemaVersion, Appendix: intPointer(0), DocumentType: "certificate",
			Fields: map[string]any{"title": "Award"}, Confidence: 0.9,
		}),
		Entry("appendix above range", submission.ExtractionProposal{
			SchemaVersion: submission.AnalysisSchemaVersion, Appendix: intPointer(29), DocumentType: "certificate",
			Fields: map[string]any{"title": "Award"}, Confidence: 0.9,
		}),
		Entry("NaN confidence", submission.ExtractionProposal{
			SchemaVersion: submission.AnalysisSchemaVersion, DocumentType: "certificate",
			Fields: map[string]any{"title": "Award"}, Confidence: math.NaN(),
		}),
		Entry("positive infinity confidence", submission.ExtractionProposal{
			SchemaVersion: submission.AnalysisSchemaVersion, DocumentType: "certificate",
			Fields: map[string]any{"title": "Award"}, Confidence: math.Inf(1),
		}),
		Entry("negative infinity confidence", submission.ExtractionProposal{
			SchemaVersion: submission.AnalysisSchemaVersion, DocumentType: "certificate",
			Fields: map[string]any{"title": "Award"}, Confidence: math.Inf(-1),
		}),
	)

	DescribeTable("validates confidence boundaries",
		func(confidence float64, expectedLowConfidence bool) {
			result, err := submission.ValidateExtractionProposal(submission.ExtractionProposal{
				SchemaVersion: submission.AnalysisSchemaVersion, DocumentType: "certificate",
				Fields: map[string]any{"title": "Award"}, Confidence: confidence,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Validated).To(BeTrue())
			Expect(result.LowConfidence).To(Equal(expectedLowConfidence))
		},
		Entry("minimum accepted confidence", submission.MinimumAnalysisConfidence, false),
		Entry("just below minimum confidence", submission.MinimumAnalysisConfidence-0.001, true),
	)

	DescribeTable("stores failures and unsupported attachments, continues parsing, and skips unsubmitted or inaccessible files", func(scenarioID int64) {
		origin.ScenarioID = scenarioID
		activeTask := task.Task{ID: 23, CourseID: origin.CourseID, ExternalID: origin.TaskExternalID}
		binding := task.Binding{ScenarioID: scenarioID, ScenarioName: origin.ScenarioName, ScenarioType: origin.ScenarioType, Task: activeTask}
		courses := mocks.NewMockClassroomCourseRepository(ctrl)
		source := mocks.NewMockSubmissionSource(ctrl)
		bindings := mocks.NewMockTaskBindingRepository(ctrl)
		repo := mocks.NewMockSubmissionRepository(ctrl)
		retriever := mocks.NewMockMaterialRetriever(ctrl)
		selected := mocks.NewMockDocumentParser(ctrl)
		courses.EXPECT().List(ctx).Return([]classroom.Course{{ExternalID: origin.CourseID, Status: classroom.Active}}, nil)
		source.EXPECT().ListTasks(ctx, origin.CourseID).Return(nil, nil)
		bindings.EXPECT().Active(ctx).Return([]task.Binding{binding}, nil)

		broken, unsupported, denied, good := item, item, item, item
		broken.ExternalID = "broken"
		unsupported.ExternalID, unsupported.Name, unsupported.MIMEType, unsupported.LocalPath = "pdf", "Evidence.pdf", "application/pdf", "snapshots/evidence.pdf"
		denied.ExternalID, denied.LocalPath, denied.Status = "denied", "", submission.AccessDenied
		good.ExternalID = "good"
		slides := submission.Material{ExternalID: "slides", Name: "Slides", MIMEType: "application/vnd.google-apps.presentation", Status: submission.Pending,
			Parsing: &submission.ParseResult{Issue: submission.UnsupportedMaterial}}
		snapshots := []submission.Material{broken, unsupported, denied, good, slides}
		initial := append([]submission.Material(nil), snapshots...)
		for i := range initial {
			initial[i].LocalPath, initial[i].Status = "", submission.Pending
			initial[i].Parsing = nil
			retriever.EXPECT().Retrieve(ctx, initial[i]).Return(snapshots[i])
		}
		submitted := submission.Submission{
			ExternalID: origin.SubmissionExternalID, State: submission.Submitted, SubmittedAt: origin.SubmittedAt,
			SubmitterID: origin.SubmitterID, SubmitterName: origin.SubmitterName, SubmitterEmail: origin.SubmitterEmail,
			Materials: initial,
		}
		draft := submission.Submission{ExternalID: "draft", State: submission.Created, Materials: []submission.Material{{ExternalID: "draft-file", Status: submission.Pending}}}
		source.EXPECT().ListSubmissions(ctx, activeTask.ExternalID).Return([]submission.Submission{submitted, draft}, nil)

		selected.EXPECT().CanHandle(broken, origin).Return(true)
		selected.EXPECT().Parse(ctx, broken, origin).Return(submission.ParsedDocument{}, errors.New("malformed archive"))
		selected.EXPECT().CanHandle(unsupported, origin).Return(false)
		selected.EXPECT().CanHandle(good, origin).Return(true)
		selected.EXPECT().Type().Return("docx").Times(2)
		document := submission.ParsedDocument{Fields: []submission.Field{{Value: "Intact", Location: submission.Location{Paragraph: 1}}}}
		selected.EXPECT().Parse(ctx, good, origin).Return(document, nil)

		expected := submitted
		expected.TaskID, expected.TaskExternalID = activeTask.ID, activeTask.ExternalID
		broken.Status, broken.Parsing = submission.Failed, &submission.ParseResult{ParserType: "docx", Issue: submission.ParseFailed}
		unsupported.Parsing = &submission.ParseResult{Issue: submission.UnsupportedMaterial}
		document.Source = submission.DocumentSource{
			ParseContext: origin, ExternalID: good.ExternalID, Name: good.Name, URI: good.SourceURI,
			MIMEType: good.MIMEType, LocalPath: good.LocalPath, Role: good.Role, AppendixNumber: good.AppendixNumber,
		}
		good.Status, good.Parsing = submission.Parsed, &submission.ParseResult{ParserType: "docx", Document: &document}
		expected.Materials = []submission.Material{broken, unsupported, denied, good, slides}
		if scenarioID != 0 {
			for i := range expected.Materials {
				m := &expected.Materials[i]
				if m.Parsing == nil {
					continue
				}
				m.Status = submission.Pending
				m.ParseOutcomes = []submission.MaterialParse{{Source: m.DocumentSource(origin), Result: *m.Parsing}}
				m.Parsing = nil
			}
		}
		repo.EXPECT().Upsert(ctx, expected).Return(nil)
		draft.TaskID, draft.TaskExternalID = activeTask.ID, activeTask.ExternalID
		repo.EXPECT().Upsert(ctx, draft).Return(nil)

		polling := application.NewPollingServiceWithParsing(courses, source, nil, bindings, repo, nil, retriever, application.NewMaterialParsing(selected))
		Expect(polling.Refresh(ctx)).To(Succeed())
	}, Entry("legacy unscoped outcomes", int64(0)), Entry("binding-scoped outcomes", int64(17)))
})

func intPointer(value int) *int {
	return &value
}
