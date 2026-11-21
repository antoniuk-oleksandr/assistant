package fxmodules

import (
	"github.com/alex/assistent/internal/adapters/classroom"
	"github.com/alex/assistent/internal/adapters/drive"
	"github.com/alex/assistent/internal/adapters/notification"
	"github.com/alex/assistent/internal/adapters/parser"
	"github.com/alex/assistent/internal/adapters/renderer"
	"github.com/alex/assistent/internal/adapters/storage"
	"github.com/alex/assistent/internal/application"
	"github.com/alex/assistent/internal/domain/ports"
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
	fx.Provide(config.Load, config.Database, config.Drive, config.Email, database.NewSQLite, locking.NewProcessLock),
)

var StorageModule = fx.Module("storage", fx.Provide(storage.NewScenarioRepository, storage.NewTaskRepository, storage.NewTaskBindingRepository, storage.NewParticipantRosterRepository, storage.NewMissingSubmitterRepository, storage.NewScenarioCourseBindingRepository, storage.NewScenarioCourseDataRepository, storage.NewSubmissionRepository, storage.NewPollingStateRepository, storage.NewReportRunRepository, storage.NewTeacherContactRepository, fx.Annotate(storage.NewReportOutputRepository, fx.As(new(ports.ReportOutputRepository))), fx.Annotate(storage.NewReportResultRepository, fx.As(new(ports.ReportResultRepository))), fx.Annotate(storage.NewReportDeliveryRepository, fx.As(new(ports.ReportDeliveryRepository))), fx.Annotate(storage.NewReminderRepository, fx.As(new(ports.ReminderRepository))), fx.Annotate(storage.NewTeacherRepository, fx.As(new(ports.TeacherRepository))), fx.Annotate(storage.NewHistoryRepository, fx.As(new(ports.HistoryRepository)))))

var ClassroomStorageModule = fx.Module("classroom-storage", fx.Provide(storage.NewClassroomCourseRepository))

var ApplicationModule = fx.Module("application", fx.Provide(
	compiledScenarios,
	application.NewPollingServiceWithScenarioCollectionAndRosterAndMissingAndReminders,
	application.NewReminderService,
	fx.Annotate(application.NewScenarioCollectionServiceWithReportNotifier),
	fx.Annotate(application.NewScenarioServiceWithCourseBindingsAndData, fx.As(new(cli.ScenarioService)), fx.As(new(cli.ScenarioTaskBindingService)), fx.As(new(cli.ScenarioCourseDataService))),
	fx.Annotate(application.NewClassroomService, fx.As(new(cli.ClassroomService))),
	fx.Annotate(application.NewTeacherContactService, fx.As(new(cli.TeacherContactService))),
	fx.Annotate(application.NewCourseTaskService, fx.As(new(cli.CourseTaskService))),
))

var PollingModule = fx.Module("polling",
	fx.Provide(fx.Annotate(classroom.NewSubmissionSource, fx.As(new(ports.SubmissionSource)), fx.As(new(ports.ClassroomProfileSource))), drive.NewMaterialRetriever, newMaterialParsing, renderer.NewConfiguredReportRenderer, notification.NewReportReadyNotifier, notification.NewReminderEmailProvider, reminderProviders),
)

func reminderProviders(provider ports.ReminderProvider) []ports.ReminderProvider {
	return []ports.ReminderProvider{provider}
}

func newMaterialParsing() *application.MaterialParsing {
	return application.NewMaterialParsingWithAnalyzer(parser.NewMaterialAnalyzer(), parser.Strategies()...)
}

var CLIModule = fx.Module("cli", fx.Provide(cli.NewScenarioCommandWithBindings, cli.NewClassroomCommandWithTasks, cli.NewTeacherCommand, cli.NewRootCommandWithClassroomAndTeacher))

func compiledScenarios() scenario.Registry {
	return scenario.NewRegistryWithProcessors(
		[]scenario.Implementation{{Name: "science-report", Type: "ScienceReportScenario"}},
		map[string]scenario.CollectionProcessor{"science-report": scenario.ScienceReportCollectionProcessor{}},
	)
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
