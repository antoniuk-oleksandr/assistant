package database_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/alex/assistent/internal/platform/config"
	"github.com/alex/assistent/internal/platform/database"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/fx"
)

func TestSQLite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "SQLite Database")
}

var _ = Describe("SQLite initialization", func() {
	It("rejects a database path whose parent is a file", func() {
		parent := filepath.Join(GinkgoT().TempDir(), "not-a-directory")
		Expect(os.WriteFile(parent, []byte("file"), 0600)).To(Succeed())

		var db *sql.DB
		app := fx.New(fx.NopLogger, fx.Supply(config.DatabaseConfig{SQLitePath: filepath.Join(parent, "state.db")}), fx.Provide(database.NewSQLite), fx.Populate(&db))
		Expect(app.Start(context.Background())).To(MatchError(ContainSubstring("create SQLite directory")))
	})

	It("creates the scenario schema in an in-memory database", func(ctx SpecContext) {
		var db *sql.DB
		app := fx.New(fx.NopLogger, fx.Supply(config.DatabaseConfig{SQLitePath: ":memory:"}), fx.Provide(database.NewSQLite), fx.Populate(&db))
		Expect(app.Start(ctx)).To(Succeed())
		DeferCleanup(app.Stop)

		var table string
		Expect(db.QueryRowContext(ctx, "SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'scenarios'").Scan(&table)).To(Succeed())
		Expect(table).To(Equal("scenarios"))
	})

	It("creates parent directories for a file database and persists the schema", func(ctx SpecContext) {
		path := filepath.Join(GinkgoT().TempDir(), "nested", "state.db")
		var db *sql.DB
		app := fx.New(fx.NopLogger, fx.Supply(config.DatabaseConfig{SQLitePath: path}), fx.Provide(database.NewSQLite), fx.Populate(&db))
		Expect(app.Start(ctx)).To(Succeed())
		DeferCleanup(app.Stop)
		Expect(db.PingContext(ctx)).To(Succeed())
		var count int
		Expect(db.QueryRowContext(ctx, "SELECT count(*) FROM scenarios").Scan(&count)).To(Succeed())
		Expect(count).To(Equal(0))
	})

})
