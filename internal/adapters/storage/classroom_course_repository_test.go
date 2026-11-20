package storage_test

import (
	"database/sql"
	"encoding/base64"
	"path/filepath"

	"github.com/alex/assistent/internal/adapters/storage"
	"github.com/alex/assistent/internal/application"
	"github.com/alex/assistent/internal/domain/ports"
	"github.com/alex/assistent/internal/platform/config"
	"github.com/alex/assistent/internal/platform/database"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/fx"
)

var _ = Describe("SQLite classroom course registration", func() {
	var (
		db      *sql.DB
		repo    ports.ClassroomCourseRepository
		service *application.ClassroomService
	)

	BeforeEach(func(ctx SpecContext) {
		cfg := config.DatabaseConfig{SQLitePath: filepath.Join(GinkgoT().TempDir(), "state.db")}
		app := fx.New(
			fx.NopLogger,
			fx.Supply(cfg),
			fx.Provide(database.NewSQLite, storage.NewClassroomCourseRepository),
			fx.Populate(&db, &repo),
		)
		Expect(app.Start(ctx)).To(Succeed())
		DeferCleanup(app.Stop)
		service = application.NewClassroomService(repo)
	})

	It("registers a URL as one normalized active course and lists its source", func(ctx SpecContext) {
		reference := "https://classroom.google.com/c/OTAwNDIzOTQ1Mjk3"
		Expect(service.Add(ctx, reference)).To(Succeed())

		courses, err := service.List(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(courses).To(HaveLen(1))
		Expect(courses[0].ExternalID).To(Equal("900423945297"))
		Expect(courses[0].SourceReference).To(Equal(reference))
		Expect(courses[0].Status).To(Equal("active"))
	})

	It("is idempotent and preserves the original reference through disable and reactivation", func(ctx SpecContext) {
		original := "https://classroom.google.com/c/OTAwNDIzOTQ1Mjk3"
		encoded := base64.RawURLEncoding.EncodeToString([]byte("900423945297"))
		Expect(service.Add(ctx, original)).To(Succeed())

		before, err := service.List(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(before).To(HaveLen(1))
		createdAt := before[0].CreatedAt

		Expect(service.Add(ctx, original)).To(Succeed())
		courses, err := service.List(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(courses).To(HaveLen(1))
		Expect(courses[0].CreatedAt).To(Equal(createdAt))
		Expect(courses[0].SourceReference).To(Equal(original))

		Expect(service.Remove(ctx, "900423945297")).To(Succeed())
		disabled, err := service.List(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(disabled).To(HaveLen(1))
		Expect(disabled[0].Status).To(Equal("disabled"))
		Expect(disabled[0].CreatedAt).To(Equal(createdAt))
		Expect(disabled[0].SourceReference).To(Equal(original))

		Expect(service.Add(ctx, encoded)).To(Succeed())
		reactivated, err := service.List(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(reactivated).To(HaveLen(1))
		Expect(reactivated[0].Status).To(Equal("active"))
		Expect(reactivated[0].CreatedAt).To(Equal(createdAt))
		Expect(reactivated[0].SourceReference).To(Equal(original))

		var count int
		Expect(db.QueryRowContext(ctx, "SELECT count(*) FROM classroom_courses").Scan(&count)).To(Succeed())
		Expect(count).To(Equal(1))
	})

	It("rejects disabling an unregistered course", func(ctx SpecContext) {
		Expect(service.Remove(ctx, "900423945297")).To(MatchError(ContainSubstring("InvalidClassroomCourseReference")))
	})
})
