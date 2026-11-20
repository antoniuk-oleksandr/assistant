package fxmodules

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/alex/assistent/internal/adapters/storage"
	"github.com/alex/assistent/internal/application"
	"github.com/alex/assistent/internal/domain/scenario"
	"github.com/alex/assistent/internal/platform/config"
	"github.com/alex/assistent/internal/platform/database"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"
)

func TestModules(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Fx Modules")
}

func TestNewFxLogger(t *testing.T) {
	logger, err := NewFxLogger()
	if err != nil {
		t.Fatalf("NewFxLogger() error = %v", err)
	}
	if logger == nil {
		t.Fatal("NewFxLogger() returned nil logger")
	}
	if _, ok := logger.(*fxevent.ZapLogger); !ok {
		t.Fatalf("logger type = %T, want *fxevent.ZapLogger", logger)
	}
}

var _ = Describe("application modules", func() {
	It("registers the compiled science report implementation", func() {
		implementation, err := compiledScenarios().Resolve("science-report")
		Expect(err).NotTo(HaveOccurred())
		Expect(implementation).To(Equal(scenario.Implementation{Name: "science-report", Type: "ScienceReportScenario"}))
	})

	It("creates an error-level Fx logger", func() {
		logger, err := NewFxLogger()
		Expect(err).NotTo(HaveOccurred())
		Expect(logger).NotTo(BeNil())
	})

	It("wires the polling module into an application lifecycle", func(ctx SpecContext) {
		app := fx.New(
			fx.NopLogger,
			fx.Supply(config.DatabaseConfig{SQLitePath: filepath.Join(GinkgoT().TempDir(), "state.db")}),
			fx.Provide(database.NewSQLite, storage.NewClassroomCourseRepository, storage.NewTaskRepository, storage.NewTaskBindingRepository, storage.NewSubmissionRepository, application.NewPollingService),
			PollingModule,
		)
		Expect(app.Start(ctx)).To(Succeed())
		Expect(app.Stop(ctx)).To(Succeed())
	})

	It("invokes the default polling refresh through Fx", func(ctx SpecContext) {
		app := fx.New(
			fx.NopLogger,
			fx.Supply(config.DatabaseConfig{SQLitePath: filepath.Join(GinkgoT().TempDir(), "state.db")}),
			fx.Provide(database.NewSQLite, storage.NewClassroomCourseRepository, storage.NewTaskRepository, storage.NewTaskBindingRepository, storage.NewSubmissionRepository, application.NewPollingService),
			PollingModule,
			fx.Invoke(func(polling *application.PollingService, lifecycle fx.Lifecycle) {
				lifecycle.Append(fx.Hook{OnStart: func(context.Context) error { return polling.Refresh(context.Background()) }})
			}),
		)
		Expect(app.Start(ctx)).To(Succeed())
		Expect(app.Stop(ctx)).To(Succeed())
	})
})
