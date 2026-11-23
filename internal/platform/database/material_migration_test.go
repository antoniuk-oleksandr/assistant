package database_test

import (
	"context"
	"database/sql"
	"fmt"
	. "github.com/onsi/ginkgo/v2"
	"path/filepath"
	"time"

	"github.com/alex/assistent/internal/adapters/storage"
	"github.com/alex/assistent/internal/domain/submission"
	"github.com/alex/assistent/internal/platform/config"
	"github.com/alex/assistent/internal/platform/database"
	. "github.com/onsi/gomega"
	"go.uber.org/fx"
)

type materialmigrationGinkgoTest struct{}

func (*materialmigrationGinkgoTest) TempDir() string          { return GinkgoT().TempDir() }
func (*materialmigrationGinkgoTest) Setenv(key, value string) { GinkgoT().Setenv(key, value) }
func (*materialmigrationGinkgoTest) Cleanup(fn func())        { DeferCleanup(fn) }
func (*materialmigrationGinkgoTest) Helper()                  {}
func (*materialmigrationGinkgoTest) Fatal(args ...any) {
	Expect(false).To(BeTrue(), fmt.Sprint(args...))
}
func (*materialmigrationGinkgoTest) Fatalf(format string, args ...any) {
	Expect(false).To(BeTrue(), fmt.Sprintf(format, args...))
}
func (*materialmigrationGinkgoTest) Errorf(format string, args ...any) {
	Expect(false).To(BeTrue(), fmt.Sprintf(format, args...))
}
func (*materialmigrationGinkgoTest) Run(name string, fn func(*materialmigrationGinkgoTest)) bool {
	By(name)
	fn(&materialmigrationGinkgoTest{})
	return true
}

func testSQLiteMigratesExistingMaterialRowsAndPersistsSizeAcrossRestart(t *materialmigrationGinkgoTest) {
	g := NewWithT(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	path := filepath.Join(t.TempDir(), "legacy.db")
	legacy, err := sql.Open("sqlite3", path)
	g.Expect(err).NotTo(HaveOccurred())
	defer legacy.Close()
	_, err = legacy.ExecContext(ctx, `
CREATE TABLE materials (
    id INTEGER PRIMARY KEY,
    submission_id INTEGER NOT NULL REFERENCES submissions(id) ON DELETE CASCADE,
    external_material_id TEXT,
    name TEXT NOT NULL,
    mime_type TEXT,
    source_uri TEXT,
    local_path TEXT,
    appendix_number INTEGER,
    role TEXT NOT NULL DEFAULT 'supporting_evidence',
    status TEXT NOT NULL CHECK (status IN ('pending', 'parsed', 'invalid', 'access_denied', 'not_found', 'failed')),
    created_at TEXT NOT NULL
);
INSERT INTO materials (id, submission_id, external_material_id, name, source_uri, status, created_at)
VALUES (17, 1, 'legacy-file', 'legacy.pdf', 'https://drive.google.com/file/d/legacy-file/view', 'not_found', '2026-11-19T10:00:00Z');`)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(legacy.Close()).To(Succeed())

	start := func() (*fx.App, *sql.DB) {
		var db *sql.DB
		app := fx.New(fx.NopLogger, fx.Supply(config.DatabaseConfig{SQLitePath: path}), fx.Provide(database.NewSQLite), fx.Populate(&db))
		g.Expect(app.Start(ctx)).To(Succeed())
		t.Cleanup(func() { g.Expect(app.Stop(context.Background())).To(Succeed()) })
		return app, db
	}
	app, db := start()
	// Populate the parent rows to read the migrated material through the repository.
	_, err = db.ExecContext(ctx, `
INSERT INTO tasks (id, provider, external_task_id, course_id, title, discovered_at, last_seen_at, status)
VALUES (1, 'google-classroom', 'task-1', 'course-1', 'Report', '2026-11-19T10:00:00Z', '2026-11-19T10:00:00Z', 'bound');
INSERT INTO submissions (id, task_id, external_submission_id, state, first_seen_at, last_seen_at)
VALUES (1, 1, 'submission-1', 'submitted', '2026-11-19T10:00:00Z', '2026-11-19T10:00:00Z');`)
	g.Expect(err).NotTo(HaveOccurred())
	readMaterial := func(db *sql.DB) submission.Material {
		items, err := storage.NewSubmissionRepository(db).ListByTask(ctx, 1)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(items).To(HaveLen(1))
		g.Expect(items[0].State).To(Equal(submission.Submitted))
		g.Expect(items[0].Materials).To(HaveLen(1))
		return items[0].Materials[0]
	}
	material := readMaterial(db)
	g.Expect(material.ID).To(Equal(int64(17)))
	g.Expect(material.ExternalID).To(Equal("legacy-file"))
	g.Expect(material.Name).To(Equal("legacy.pdf"))
	g.Expect(material.SourceURI).To(Equal("https://drive.google.com/file/d/legacy-file/view"))
	g.Expect(material.Status).To(Equal(submission.NotFound))
	g.Expect(material.SizeBytes).To(BeNil())

	size := int64(0)
	material.SizeBytes = &size
	g.Expect(storage.NewSubmissionRepository(db).Upsert(ctx, submission.Submission{
		TaskID: 1, ExternalID: "submission-1", State: submission.Submitted, Materials: []submission.Material{material},
	})).To(Succeed())
	g.Expect(app.Stop(ctx)).To(Succeed())
	_, reopened := start()
	g.Expect(readMaterial(reopened)).To(Equal(material))
}

var _ = Describe("material_migration legacy tests", func() {
	It("runs the converted standard-library cases", func() {
		t := &materialmigrationGinkgoTest{}
		testSQLiteMigratesExistingMaterialRowsAndPersistsSizeAcrossRestart(t)
	})
})
