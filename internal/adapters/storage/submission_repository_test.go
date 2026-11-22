package storage_test

import (
	"context"
	"database/sql"
	"path/filepath"

	"github.com/alex/assistent/internal/adapters/storage"
	"github.com/alex/assistent/internal/domain/ports"
	"github.com/alex/assistent/internal/domain/submission"
	"github.com/alex/assistent/internal/domain/task"
	"github.com/alex/assistent/internal/platform/config"
	"github.com/alex/assistent/internal/platform/database"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/fx"
)

var _ = Describe("SQLite submission repository", func() {
	var (
		db   *sql.DB
		repo ports.SubmissionRepository
	)

	BeforeEach(func(ctx SpecContext) {
		cfg := config.DatabaseConfig{SQLitePath: filepath.Join(GinkgoT().TempDir(), "state.db")}
		app := fx.New(fx.NopLogger, fx.Supply(cfg), fx.Provide(database.NewSQLite, storage.NewTaskRepository, storage.NewSubmissionRepository), fx.Populate(&db, &repo))
		Expect(app.Start(ctx)).To(Succeed())
		// ListByTask loads materials while iterating submissions. Allow the test
		// database to use a second connection for that nested read.
		db.SetMaxOpenConns(2)
		DeferCleanup(app.Stop)
	})

	It("persists submitter, state, timestamps, late flag, and attachment metadata", func(ctx SpecContext) {
		Expect(storage.NewTaskRepository(db).Register(ctx, task.Task{Provider: "google-classroom", ExternalID: "task-1", CourseID: "course-1", Title: "Report"})).To(Succeed())
		appendix := 3
		item := submission.Submission{
			TaskExternalID: "task-1", ExternalID: "submission-1", SubmitterID: "user-1", SubmitterEmail: "person@example.com", SubmitterName: "Person", State: submission.Submitted,
			SubmittedAt: "2026-11-19T10:30:00Z", CreatedAt: "2026-11-19T10:00:00Z", UpdatedAt: "2026-11-19T10:35:00Z", Late: true,
			Materials: []submission.Material{
				{ExternalID: "file-1", Name: "report.docx", MIMEType: "application/vnd.openxmlformats-officedocument.wordprocessingml.document", SourceURI: "https://drive.example/file-1", LocalPath: "/data/report.docx", AppendixNumber: &appendix, Role: "appendix", Status: submission.AccessDenied},
				{Name: "notes.txt", SourceURI: "https://drive.example/notes"},
			},
		}
		Expect(repo.Upsert(ctx, item)).To(Succeed())

		items, err := repo.ListByTask(ctx, 1)
		Expect(err).NotTo(HaveOccurred())
		Expect(items).To(HaveLen(1))
		Expect(items[0].ExternalID).To(Equal(item.ExternalID))
		Expect(items[0].SubmitterID).To(Equal(item.SubmitterID))
		Expect(items[0].SubmitterEmail).To(Equal(item.SubmitterEmail))
		Expect(items[0].SubmitterName).To(Equal(item.SubmitterName))
		Expect(items[0].State).To(Equal(item.State))
		Expect(items[0].SubmittedAt).To(Equal(item.SubmittedAt))
		Expect(items[0].CreatedAt).To(Equal(item.CreatedAt))
		Expect(items[0].UpdatedAt).To(Equal(item.UpdatedAt))
		Expect(items[0].Late).To(BeTrue())
		Expect(items[0].Materials).To(HaveLen(2))
		Expect(items[0].Materials[0].ExternalID).To(Equal(item.Materials[0].ExternalID))
		Expect(items[0].Materials[0].Name).To(Equal(item.Materials[0].Name))
		Expect(items[0].Materials[0].MIMEType).To(Equal(item.Materials[0].MIMEType))
		Expect(items[0].Materials[0].SourceURI).To(Equal(item.Materials[0].SourceURI))
		Expect(items[0].Materials[0].LocalPath).To(Equal(item.Materials[0].LocalPath))
		Expect(items[0].Materials[0].AppendixNumber).To(Equal(item.Materials[0].AppendixNumber))
		Expect(items[0].Materials[0].Role).To(Equal(item.Materials[0].Role))
		Expect(items[0].Materials[0].Status).To(Equal(item.Materials[0].Status))
		Expect(items[0].Materials[1].Name).To(Equal("notes.txt"))
		Expect(items[0].Materials[1].SourceURI).To(Equal("https://drive.example/notes"))
		Expect(items[0].Materials[1].Role).To(Equal("supporting_evidence"))
		Expect(items[0].Materials[1].Status).To(Equal(submission.Pending))
	})

	It("deduplicates repeated submissions and updates state and material metadata", func(ctx SpecContext) {
		taskRepo := storage.NewTaskRepository(db)
		Expect(taskRepo.Register(ctx, task.Task{Provider: "google-classroom", ExternalID: "task-1", CourseID: "course-1", Title: "Report"})).To(Succeed())
		first := submission.Submission{TaskExternalID: "task-1", ExternalID: "submission-1", State: submission.Created, SubmitterEmail: "old@example.com", Materials: []submission.Material{{ExternalID: "file-1", Name: "old.docx", Status: submission.Pending}}}
		second := first
		second.State = submission.Submitted
		second.SubmitterEmail = "new@example.com"
		second.SubmittedAt = "2026-11-19T10:30:00Z"
		second.Materials = []submission.Material{{ExternalID: "file-1", Name: "new.docx", MIMEType: "application/pdf", SourceURI: "https://drive.example/file-1", LocalPath: "/data/new.pdf", Role: "supporting_evidence", Status: submission.Parsed}}
		Expect(repo.Upsert(ctx, first)).To(Succeed())
		Expect(repo.Upsert(ctx, second)).To(Succeed())

		items, err := repo.ListByTask(ctx, 1)
		Expect(err).NotTo(HaveOccurred())
		Expect(items).To(HaveLen(1))
		Expect(items[0].State).To(Equal(submission.Submitted))
		Expect(items[0].SubmitterEmail).To(Equal("new@example.com"))
		Expect(items[0].Materials).To(HaveLen(1))
		Expect(items[0].Materials[0].Name).To(Equal("new.docx"))
		Expect(items[0].Materials[0].Status).To(Equal(submission.Parsed))
		var submissions, materials int
		Expect(db.QueryRowContext(ctx, "SELECT count(*) FROM submissions").Scan(&submissions)).To(Succeed())
		Expect(db.QueryRowContext(ctx, "SELECT count(*) FROM materials").Scan(&materials)).To(Succeed())
		Expect(submissions).To(Equal(1))
		Expect(materials).To(Equal(1))
	})

	It("returns errors for missing tasks, canceled contexts, and closed databases", func(ctx SpecContext) {
		var err error
		Expect(repo.Upsert(ctx, submission.Submission{TaskExternalID: "missing", ExternalID: "submission-1"})).To(MatchError(ContainSubstring(`find task for submission "submission-1"`)))
		Expect(repo.Upsert(ctx, submission.Submission{TaskID: 1, ExternalID: "submission-direct"})).To(MatchError(ContainSubstring(`upsert submission "submission-direct"`)))
		Expect(storage.NewTaskRepository(db).Register(ctx, task.Task{Provider: "google-classroom", ExternalID: "task-1", CourseID: "course-1", Title: "Report"})).To(Succeed())
		_, err = db.ExecContext(ctx, `CREATE TRIGGER remove_submission_after_insert AFTER INSERT ON submissions BEGIN DELETE FROM submissions WHERE id = NEW.id; END`)
		Expect(err).NotTo(HaveOccurred())
		Expect(repo.Upsert(ctx, submission.Submission{TaskExternalID: "task-1", ExternalID: "submission-removed"})).To(MatchError(ContainSubstring(`find submission "submission-removed"`)))
		canceled, cancel := context.WithCancel(ctx)
		cancel()
		Expect(repo.Upsert(canceled, submission.Submission{TaskExternalID: "missing", ExternalID: "submission-2"})).To(MatchError(ContainSubstring("begin submission upsert: context canceled")))
		_, err = repo.ListByTask(canceled, 1)
		Expect(err).To(MatchError(ContainSubstring("list submissions: context canceled")))
		Expect(db.Close()).To(Succeed())
		Expect(repo.Upsert(ctx, submission.Submission{TaskExternalID: "task-1", ExternalID: "submission-3"})).To(MatchError(ContainSubstring("begin submission upsert: sql: database is closed")))
		_, err = repo.ListByTask(ctx, 1)
		Expect(err).To(MatchError(ContainSubstring("list submissions: sql: database is closed")))
	})

	It("reports malformed submission rows while listing", func(ctx SpecContext) {
		_, err := db.ExecContext(ctx, `INSERT INTO tasks
			(provider, external_task_id, course_id, title, discovered_at, last_seen_at, status)
			VALUES ('google-classroom', 'task-malformed', 'course-1', 'Report', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z', 'discovered')`)
		Expect(err).NotTo(HaveOccurred())
		_, err = db.ExecContext(ctx, `INSERT INTO submissions
			(task_id, external_submission_id, state, late, first_seen_at, last_seen_at)
			VALUES (1, 'submission-malformed', 'new', 'not-a-bool', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`)
		Expect(err).NotTo(HaveOccurred())
		_, err = repo.ListByTask(ctx, 1)
		Expect(err).To(MatchError(ContainSubstring("scan submission")))
	})

	It("rolls back a snapshot replacement and submission update when another material fails to persist", func(ctx SpecContext) {
		Expect(storage.NewTaskRepository(db).Register(ctx, task.Task{Provider: "google-classroom", ExternalID: "task-1", CourseID: "course-1", Title: "Report"})).To(Succeed())
		size := int64(100)
		original := submission.Material{ExternalID: "file-1", Name: "report.pdf", MIMEType: "application/pdf", SizeBytes: &size,
			SourceURI: "https://drive.google.com/open?id=file-1", LocalPath: "/fixtures/original.pdf", Role: "supporting_evidence", Status: submission.Pending}
		item := submission.Submission{TaskExternalID: "task-1", ExternalID: "report-1", State: submission.Submitted, Materials: []submission.Material{original}}
		Expect(repo.Upsert(ctx, item)).To(Succeed())
		before, err := repo.ListByTask(ctx, 1)
		Expect(err).NotTo(HaveOccurred())
		Expect(before).To(HaveLen(1))
		Expect(before[0].Materials).To(HaveLen(1))
		original.ID = before[0].Materials[0].ID
		Expect(before[0].Materials[0]).To(Equal(original))

		replacement := original
		replacement.Name = "report"
		replacement.MIMEType = "application/vnd.google-apps.document"
		replacement.LocalPath = "/fixtures/replacement.docx"
		replacement.SizeBytes = nil
		badSize := int64(-1)
		item.SubmitterName = "Updated submitter"
		item.Materials = []submission.Material{replacement, {ExternalID: "file-2", Name: "invalid-size", SizeBytes: &badSize}}
		Expect(repo.Upsert(ctx, item)).To(MatchError(ContainSubstring(`upsert material "invalid-size"`)))
		after, err := repo.ListByTask(ctx, 1)
		Expect(err).NotTo(HaveOccurred())
		Expect(after).To(Equal(before))

		item.Materials = []submission.Material{replacement}
		Expect(repo.Upsert(ctx, item)).To(Succeed())
		after, err = repo.ListByTask(ctx, 1)
		Expect(err).NotTo(HaveOccurred())
		Expect(after).To(HaveLen(1))
		Expect(after[0].Materials).To(Equal([]submission.Material{replacement}))
	})

	It("rejects invalid submission and material states", func(ctx SpecContext) {
		Expect(storage.NewTaskRepository(db).Register(ctx, task.Task{Provider: "google-classroom", ExternalID: "task-1", CourseID: "course-1", Title: "Report"})).To(Succeed())

		Expect(repo.Upsert(ctx, submission.Submission{TaskExternalID: "task-1", ExternalID: "invalid-submission", State: "invalid"})).To(MatchError(ContainSubstring(`upsert submission "invalid-submission"`)))
		Expect(repo.Upsert(ctx, submission.Submission{TaskExternalID: "task-1", ExternalID: "invalid-material", Materials: []submission.Material{{Name: "report.docx", Status: "invalid-state"}}})).To(MatchError(ContainSubstring(`upsert material "report.docx"`)))
	})
})
