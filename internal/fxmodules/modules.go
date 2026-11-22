package fxmodules

import (
	"context"
	"github.com/alex/assistent/internal/adapters/classroom"
	"github.com/alex/assistent/internal/adapters/drive"
	"github.com/alex/assistent/internal/adapters/notification"
	"github.com/alex/assistent/internal/adapters/parser"
	"github.com/alex/assistent/internal/adapters/renderer"
	"github.com/alex/assistent/internal/adapters/storage"
	"github.com/alex/assistent/internal/application"
	"github.com/alex/assistent/internal/domain/ports"
	"github.com/alex/assistent/internal/domain/scenario"
	"github.com/alex/assistent/internal/domain/task"
	"github.com/alex/assistent/internal/platform/config"
	"github.com/alex/assistent/internal/platform/database"
	"github.com/alex/assistent/internal/platform/locking"
	cli "github.com/alex/assistent/internal/presentation/cobra"
	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"
	"go.uber.org/zap"
)

var PlatformModule = fx.Module("platform",
	fx.Provide(config.Load, config.Database, config.Drive, config.Email, config.Telegram, config.Deadline, database.NewSQLite, locking.NewProcessLock),
)

var StorageModule = fx.Module("storage", fx.Provide(storage.NewScenarioRepository, storage.NewTaskRepository, storage.NewTaskBindingRepository, storage.NewParticipantRosterRepository, storage.NewMissingSubmitterRepository, storage.NewScenarioCourseBindingRepository, storage.NewScenarioCourseDataRepository, storage.NewSubmissionRepository, storage.NewPollingStateRepository, storage.NewReportRunRepository, fx.Annotate(storage.NewReportOutputRepository, fx.As(new(ports.ReportOutputRepository))), fx.Annotate(storage.NewReportResultRepository, fx.As(new(ports.ReportResultRepository))), fx.Annotate(storage.NewReportDeliveryRepository, fx.As(new(ports.ReportDeliveryRepository))), fx.Annotate(storage.NewReminderRepository, fx.As(new(ports.ReminderRepository))), fx.Annotate(storage.NewTeacherRepository, fx.As(new(ports.TeacherRepository))), fx.Annotate(storage.NewTelegramPairingRepository, fx.As(new(ports.TelegramPairingRepository))), fx.Annotate(storage.NewHistoryRepository, fx.As(new(ports.HistoryRepository)))))

var ClassroomStorageModule = fx.Module("classroom-storage", fx.Provide(storage.NewClassroomCourseRepository))

var ApplicationModule = fx.Module("application", fx.Provide(
	compiledScenarios,
	application.NewPollingServiceWithScenarioCollectionAndRosterAndMissingAndReminders,
	application.NewReminderService,
	application.NewHeadNotificationService,
	application.NewDeliveryService,
	application.NewTelegramPairingService,
	fx.Annotate(application.NewTeacherServiceWithDirectoryAndTelegram, fx.As(new(cli.TeacherListService)), fx.As(new(cli.TelegramPairingService))),
	fx.Annotate(application.NewScenarioCollectionServiceWithReportNotifier),
	fx.Annotate(application.NewReportService, fx.As(new(cli.ReportService))),
	fx.Annotate(application.NewClassroomService, fx.As(new(cli.ClassroomService))),
	fx.Annotate(application.NewCourseTaskService, fx.As(new(cli.CourseTaskService))),
))

var PollingModule = fx.Module("polling",
	fx.Provide(fx.Annotate(classroom.NewSubmissionSource, fx.As(new(ports.SubmissionSource)), fx.As(new(ports.ClassroomProfileSource)), fx.As(new(ports.ParticipantSource))), drive.NewMaterialRetriever, newMaterialParsing, renderer.NewConfiguredReportRenderer, fx.Annotate(notification.NewTelegramProvider, fx.As(new(ports.ReportReadyTelegramSender)), fx.As(new(ports.TelegramUpdateSource)), fx.As(new(ports.TelegramPairingMessenger))), fx.Annotate(notification.NewReportReadyNotifierWithTelegramAndProfile, fx.As(new(ports.ReportReadyNotifier))), fx.Annotate(notification.NewPreliminaryReportNotifierWithProfile, fx.As(new(ports.PreliminaryReportNotifier))), configuredReminderProviders, configuredDeliveryProviders),
	fx.Invoke(configurePollingDeadlines, configureTelegramPairing),
)

func configuredReminderProviders(email config.EmailConfig, telegram config.TelegramConfig) []ports.ReminderProvider {
	return []ports.ReminderProvider{notification.NewReminderEmailProvider(email), notification.NewTelegramProvider(telegram)}
}

type pollingDeadlineParams struct {
	fx.In
	Service     *application.PollingService
	Deadline    config.DeadlineConfig
	Teachers    ports.TeacherRepository         `optional:"true"`
	Profile     ports.ClassroomProfileSource    `optional:"true"`
	Preliminary ports.PreliminaryReportNotifier `optional:"true"`
}

func configurePollingDeadlines(params pollingDeadlineParams) {
	params.Service.SetDeadlineWindows(task.DeadlineWindows{PreliminaryBefore: params.Deadline.PreliminaryBefore, RefreshBefore: params.Deadline.RefreshBefore})
	params.Service.SetTeacherRepository(params.Teachers)
	params.Service.SetClassroomProfileSource(params.Profile)
	params.Service.SetPreliminaryReportNotifier(params.Preliminary)
}

type telegramPairingParams struct {
	fx.In
	Service   *application.TelegramPairingService `optional:"true"`
	Telegram  config.TelegramConfig               `optional:"true"`
	Lifecycle fx.Lifecycle
}

func configureTelegramPairing(params telegramPairingParams) {
	if params.Telegram.Token == "" || params.Service == nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	params.Lifecycle.Append(fx.Hook{
		OnStart: func(context.Context) error {
			go func() { _ = params.Service.Run(ctx) }()
			return nil
		},
		OnStop: func(context.Context) error {
			cancel()
			return nil
		},
	})
}

func configuredDeliveryProviders(email config.EmailConfig, telegram config.TelegramConfig) []ports.ResultDeliveryProvider {
	return []ports.ResultDeliveryProvider{notification.NewReportDeliveryEmailProvider(email), notification.NewTelegramProvider(telegram)}
}

func newMaterialParsing() *application.MaterialParsing {
	return application.NewMaterialParsingWithAnalyzer(parser.NewMaterialAnalyzer(), parser.Strategies()...)
}

var CLIModule = fx.Module("cli", fx.Provide(cli.NewReportCommand, cli.NewClassroomCommandWithTasks, cli.NewTeacherCommand, cli.NewRootCommandWithClassroomAndTeacher))

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
