package storage_test

import (
	"database/sql"
	"path/filepath"
	"time"

	"github.com/alex/assistent/internal/adapters/storage"
	"github.com/alex/assistent/internal/domain/ports"
	"github.com/alex/assistent/internal/platform/config"
	"github.com/alex/assistent/internal/platform/database"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/fx"
)

var _ = Describe("SQLite polling state repository", func() {
	It("persists the last completed checkpoint across database lifecycles", func(ctx SpecContext) {
		path := filepath.Join(GinkgoT().TempDir(), "state.db")
		checkpoint := time.Date(2026, time.January, 2, 3, 4, 5, 678000000, time.FixedZone("checkpoint", 2*60*60))

		start := func() (*fx.App, *sql.DB, ports.PollingStateRepository) {
			var db *sql.DB
			var repo ports.PollingStateRepository
			app := fx.New(
				fx.NopLogger,
				fx.Supply(config.DatabaseConfig{SQLitePath: path}),
				fx.Provide(database.NewSQLite, storage.NewPollingStateRepository),
				fx.Populate(&db, &repo),
			)
			Expect(app.Start(ctx)).To(Succeed())
			return app, db, repo
		}

		app, db, repo := start()
		Expect(db.PingContext(ctx)).To(Succeed())
		initial, err := repo.LastCheckpoint(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(initial).To(BeZero())
		Expect(repo.SaveCheckpoint(ctx, checkpoint)).To(Succeed())
		Expect(app.Stop(ctx)).To(Succeed())

		app, _, repo = start()
		DeferCleanup(app.Stop)
		recovered, err := repo.LastCheckpoint(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(recovered).To(Equal(checkpoint.UTC()))
	})
})
