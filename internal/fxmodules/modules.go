package fxmodules

import (
	"github.com/alex/assistent/internal/adapters/classroom"
	"github.com/alex/assistent/internal/adapters/drive"
	"github.com/alex/assistent/internal/adapters/storage"
	"github.com/alex/assistent/internal/application"
	"github.com/alex/assistent/internal/domain/scenario"
	"github.com/alex/assistent/internal/platform/config"
	"github.com/alex/assistent/internal/platform/database"
	"github.com/alex/assistent/internal/platform/locking"
	cli "github.com/alex/assistent/internal/presentation/cobra"
	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"
	"go.uber.org/zap"
)

var PlatformModule = fx.Module("platform",
	fx.Provide(config.Load, config.Database, config.Drive, database.NewSQLite, locking.NewProcessLock),
)

var StorageModule = fx.Module("storage", fx.Provide(storage.NewScenarioRepository, storage.NewTaskRepository, storage.NewTaskBindingRepository, storage.NewSubmissionRepository, storage.NewPollingStateRepository))

var ClassroomStorageModule = fx.Module("classroom-storage", fx.Provide(storage.NewClassroomCourseRepository))

var ApplicationModule = fx.Module("application", fx.Provide(
	compiledScenarios,
	application.NewPollingServiceWithMaterials,
	fx.Annotate(application.NewScenarioServiceWithBindings, fx.As(new(cli.ScenarioService)), fx.As(new(cli.ScenarioTaskBindingService))),
	fx.Annotate(application.NewClassroomService, fx.As(new(cli.ClassroomService))),
))

var PollingModule = fx.Module("polling",
	fx.Provide(classroom.NewSubmissionSource, drive.NewMaterialRetriever),
)

var CLIModule = fx.Module("cli", fx.Provide(cli.NewScenarioCommand, cli.NewClassroomCommand, cli.NewRootCommandWithClassroom))

func compiledScenarios() scenario.Registry {
	return scenario.NewRegistry(scenario.Implementation{Name: "science-report", Type: "ScienceReportScenario"})
}

func NewFxLogger() (fxevent.Logger, error) {
	cfg := zap.NewProductionConfig()
	cfg.Level.SetLevel(zap.ErrorLevel)
	logger, err := cfg.Build()
	if err != nil {
		return nil, err
	}
	return &fxevent.ZapLogger{Logger: logger}, nil
}
