package sqlite_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/alex/assistent/internal/adapters/storage"
	"github.com/alex/assistent/internal/domain/submission"
	"github.com/alex/assistent/internal/domain/task"
	"github.com/alex/assistent/internal/platform/config"
	"github.com/alex/assistent/internal/platform/database"
	"github.com/alex/assistent/tests/support/fixtures"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/fx"
)

func TestSQLiteMaterialParsing(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "SQLite material parsing")
}

var _ = Describe("persisted material parsing", func() {
	var path string
	BeforeEach(func() { path = filepath.Join(GinkgoT().TempDir(), "reports.db") })

	start := func(ctx context.Context, path string) (*fx.App, *sql.DB) {
		var db *sql.DB
		app := fx.New(fx.NopLogger, fx.Supply(config.DatabaseConfig{SQLitePath: path}), fx.Provide(database.NewSQLite), fx.Populate(&db))
		Expect(app.Start(ctx)).To(Succeed())
		DeferCleanup(func() { Expect(app.Stop(context.Background())).To(Succeed()) })
		return app, db
	}

	It("migrates existing materials and preserves parsing outcomes and their source across restart", func(ctx SpecContext) {
		app, db := start(ctx, path)
		origin := fixtures.MaterialOrigin()
		tasks := storage.NewTaskRepository(db)
		Expect(tasks.Register(ctx, task.Task{Provider: "google-classroom", CourseID: origin.CourseID, ExternalID: origin.TaskExternalID, Title: "Report"})).To(Succeed())
		items, err := tasks.List(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(items).To(HaveLen(1))
		taskID := items[0].ID
		item := fixtures.SubmittedMaterial()
		current := submission.Submission{
			TaskID: taskID, ExternalID: origin.SubmissionExternalID, State: submission.Submitted,
			Materials: []submission.Material{item},
		}
		Expect(storage.NewSubmissionRepository(db).Upsert(ctx, current)).To(Succeed())
		// Simulate the schema from before issue #7, retaining the already-stored row.
		_, err = db.ExecContext(ctx, `DROP TABLE material_parses`)
		Expect(err).NotTo(HaveOccurred())
		_, err = db.ExecContext(ctx, `ALTER TABLE materials DROP COLUMN parse_result`)
		Expect(err).NotTo(HaveOccurred())
		Expect(app.Stop(ctx)).To(Succeed())

		app, db = start(ctx, path)
		repo := storage.NewSubmissionRepository(db)
		stored, err := repo.ListByTask(ctx, taskID)
		Expect(err).NotTo(HaveOccurred())
		Expect(stored).To(HaveLen(1))
		Expect(stored[0].Materials).To(HaveLen(1))
		legacy := stored[0].Materials[0]
		Expect(legacy.ID).NotTo(BeZero())
		item.ID = legacy.ID
		Expect(legacy).To(Equal(item))

		item.Status = submission.Parsed
		item.Parsing = &submission.ParseResult{ParserType: "docx", Document: &submission.ParsedDocument{
			Source: submission.DocumentSource{
				ParseContext: origin, ExternalID: item.ExternalID, Name: item.Name, URI: item.SourceURI,
				MIMEType: item.MIMEType, LocalPath: item.LocalPath, Role: item.Role, AppendixNumber: item.AppendixNumber,
			},
			Fields:   []submission.Field{{Value: "Summary", Location: submission.Location{Paragraph: 1}}},
			Tables:   []submission.Table{{Index: 1, Rows: [][]submission.Field{{{Value: "12", Location: submission.Location{Table: 1, Row: 1, Column: 1}}}}}},
			Warnings: []string{"Missing optional field"},
		}}
		failed := fixtures.SubmittedMaterial()
		failed.ExternalID, failed.Name, failed.LocalPath, failed.Status = "broken", "Broken.docx", "snapshots/broken.docx", submission.Failed
		failed.Parsing = &submission.ParseResult{ParserType: "docx", Issue: submission.ParseFailed}
		unsupported := fixtures.SubmittedMaterial()
		unsupported.ExternalID, unsupported.Name, unsupported.MIMEType, unsupported.LocalPath = "pdf", "Evidence.pdf", "application/pdf", "snapshots/evidence.pdf"
		unsupported.Parsing = &submission.ParseResult{Issue: submission.UnsupportedMaterial}
		current.Materials = []submission.Material{item, failed, unsupported}
		Expect(repo.Upsert(ctx, current)).To(Succeed())
		Expect(repo.Upsert(ctx, current)).To(Succeed())
		Expect(app.Stop(ctx)).To(Succeed())

		_, db = start(ctx, path)
		repo = storage.NewSubmissionRepository(db)
		stored, err = repo.ListByTask(ctx, taskID)
		Expect(err).NotTo(HaveOccurred())
		Expect(stored).To(HaveLen(1))
		Expect(stored[0].Materials).To(HaveLen(3))
		failed.ID, unsupported.ID = stored[0].Materials[1].ID, stored[0].Materials[2].ID
		Expect(stored[0].Materials).To(Equal([]submission.Material{item, failed, unsupported}))

		// A later access failure must not erase the last downloaded snapshot or parse.
		denied := item
		denied.Status, denied.LocalPath, denied.Parsing = submission.AccessDenied, "", nil
		current.Materials = []submission.Material{denied}
		Expect(repo.Upsert(ctx, current)).To(Succeed())
		stored, err = repo.ListByTask(ctx, taskID)
		Expect(err).NotTo(HaveOccurred())
		item.Status = submission.AccessDenied
		Expect(stored).To(HaveLen(1))
		Expect(stored[0].Materials).To(Equal([]submission.Material{item, failed, unsupported}))
	})

	It("keeps distinct binding and snapshot outcomes after replacement and restart", func(ctx SpecContext) {
		app, db := start(ctx, path)
		origin := fixtures.MaterialOrigin()
		tasks := storage.NewTaskRepository(db)
		Expect(tasks.Register(ctx, task.Task{Provider: "google-classroom", CourseID: origin.CourseID, ExternalID: origin.TaskExternalID, Title: "Report"})).To(Succeed())
		registered, err := tasks.List(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(registered).To(HaveLen(1))
		current := submission.Submission{TaskID: registered[0].ID, ExternalID: origin.SubmissionExternalID, State: submission.Submitted}
		repo := storage.NewSubmissionRepository(db)
		first := fixtures.SubmittedMaterial()
		firstSource := first.DocumentSource(origin)
		legacy := submission.ParseResult{ParserType: "docx", Document: &submission.ParsedDocument{Source: firstSource}}
		first.Parsing = &legacy
		current.Materials = []submission.Material{first}
		Expect(repo.Upsert(ctx, current)).To(Succeed())

		origin.ScenarioID = 17
		bindingA := first.DocumentSource(origin)
		origin.ScenarioID, origin.ScenarioName = 18, "other-report"
		bindingB := first.DocumentSource(origin)
		a := submission.MaterialParse{Source: bindingA, Result: submission.ParseResult{ParserType: "google-docs", Issue: submission.ParseFailed}}
		b := submission.MaterialParse{Source: bindingB, Result: submission.ParseResult{Issue: submission.UnsupportedMaterial}}
		first.Parsing = nil
		first.ParseOutcomes = []submission.MaterialParse{a}
		current.Materials = []submission.Material{first}
		Expect(repo.Upsert(ctx, current)).To(Succeed())
		first.ParseOutcomes = []submission.MaterialParse{b}
		current.Materials = []submission.Material{first}
		Expect(repo.Upsert(ctx, current)).To(Succeed())

		next := first
		next.LocalPath = "snapshots/file-1-revised.docx"
		nextSource := bindingA
		nextSource.LocalPath = next.LocalPath
		revised := submission.MaterialParse{Source: nextSource, Result: submission.ParseResult{ParserType: "google-docs", Document: &submission.ParsedDocument{Source: nextSource,
			Fields: []submission.Field{{Value: "Revised", Location: submission.Location{Paragraph: 1}}},
		}}}
		next.ParseOutcomes = []submission.MaterialParse{revised}
		current.Materials = []submission.Material{next}
		Expect(repo.Upsert(ctx, current)).To(Succeed())
		Expect(repo.Upsert(ctx, current)).To(Succeed())
		Expect(app.Stop(ctx)).To(Succeed())

		_, db = start(ctx, path)
		stored, err := storage.NewSubmissionRepository(db).ListByTask(ctx, current.TaskID)
		Expect(err).NotTo(HaveOccurred())
		Expect(stored).To(HaveLen(1))
		Expect(stored[0].Materials).To(HaveLen(1))
		got := stored[0].Materials[0]
		Expect(got.LocalPath).To(Equal(next.LocalPath))
		Expect(got.Parsing).To(BeNil())
		Expect(got.ParseOutcomes).To(ConsistOf(submission.MaterialParse{Source: firstSource, Result: legacy}, a, b, revised))
	})

	It("retains invalid and low-confidence analysis proposals without making them eligible after restart", func(ctx SpecContext) {
		app, db := start(ctx, path)
		origin := fixtures.MaterialOrigin()
		tasks := storage.NewTaskRepository(db)
		Expect(tasks.Register(ctx, task.Task{Provider: "google-classroom", CourseID: origin.CourseID, ExternalID: origin.TaskExternalID, Title: "Report"})).To(Succeed())
		registered, err := tasks.List(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(registered).To(HaveLen(1))

		repo := storage.NewSubmissionRepository(db)
		current := submission.Submission{TaskID: registered[0].ID, ExternalID: origin.SubmissionExternalID, State: submission.Submitted}
		invalidMaterial := fixtures.SubmittedMaterial()
		invalidMaterial.ExternalID, invalidMaterial.Name, invalidMaterial.LocalPath = "invalid-analysis", "Invalid analysis", "snapshots/invalid"
		invalidProposal := submission.ExtractionProposal{
			SchemaVersion: "material-analysis.legacy", DocumentType: "appendix",
			Fields:               map[string]any{"title": "Candidate", "tags": []any{"invalid", "retained"}},
			IsSupportingEvidence: true, Confidence: 0.99,
		}
		invalid := submission.MaterialParse{
			Source: invalidMaterial.DocumentSource(origin),
			Result: submission.ParseResult{Issue: submission.AnalysisInvalid, Analysis: &submission.AnalysisResult{Proposal: invalidProposal}},
		}

		lowMaterial := fixtures.SubmittedMaterial()
		lowMaterial.ExternalID, lowMaterial.Name, lowMaterial.LocalPath = "low-analysis", "Low confidence analysis", "snapshots/low"
		appendix := 12
		lowProposal := submission.ExtractionProposal{
			SchemaVersion: submission.AnalysisSchemaVersion, Appendix: &appendix, DocumentType: "supporting-evidence",
			Fields:               map[string]any{"authors": []any{"Ada", "Grace"}, "peer_reviewed": true},
			IsSupportingEvidence: true, Confidence: 0.5,
		}
		low := submission.MaterialParse{
			Source: lowMaterial.DocumentSource(origin),
			Result: submission.ParseResult{Issue: submission.AnalysisLowConfidence, Analysis: &submission.AnalysisResult{
				Proposal: lowProposal, Validated: true, LowConfidence: true,
			}},
		}
		invalidMaterial.ParseOutcomes = []submission.MaterialParse{invalid}
		lowMaterial.ParseOutcomes = []submission.MaterialParse{low}
		current.Materials = []submission.Material{invalidMaterial, lowMaterial}
		Expect(repo.Upsert(ctx, current)).To(Succeed())
		Expect(invalid.EligibleForAggregation()).To(BeFalse())
		Expect(low.EligibleForAggregation()).To(BeFalse())
		Expect(app.Stop(ctx)).To(Succeed())

		_, db = start(ctx, path)
		stored, err := storage.NewSubmissionRepository(db).ListByTask(ctx, current.TaskID)
		Expect(err).NotTo(HaveOccurred())
		Expect(stored).To(HaveLen(1))
		Expect(stored[0].Materials).To(HaveLen(2))

		gotInvalid, gotLow := stored[0].Materials[0].ParseOutcomes[0], stored[0].Materials[1].ParseOutcomes[0]
		Expect(gotInvalid).To(Equal(invalid))
		Expect(gotInvalid.Result.Analysis).NotTo(BeNil())
		Expect(gotInvalid.Result.Analysis.Proposal.Fields).To(Equal(invalidProposal.Fields))
		Expect(gotInvalid.Result.Analysis.Proposal.IsSupportingEvidence).To(BeTrue())
		Expect(gotInvalid.EligibleForAggregation()).To(BeFalse())
		Expect(gotLow).To(Equal(low))
		Expect(gotLow.Result.Analysis).NotTo(BeNil())
		Expect(gotLow.Result.Analysis.Proposal.Fields).To(Equal(lowProposal.Fields))
		Expect(gotLow.Result.Analysis.Proposal.IsSupportingEvidence).To(BeTrue())
		Expect(gotLow.Result.Analysis.LowConfidence).To(BeTrue())
		Expect(gotLow.EligibleForAggregation()).To(BeFalse())
	})
})
