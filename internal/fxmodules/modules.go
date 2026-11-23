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
	"github.com/alex/assistent/internal/platform/logging"
	cli "github.com/alex/assistent/internal/presentation/cobra"
	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"
	"go.uber.org/zap"
	"time"
)

var PlatformModule = fx.Module("platform",
	fx.Provide(config.Load, config.Database, config.Classroom, config.Drive, config.Sheets, config.Email, config.Telegram, config.Deadline, config.Report, config.Logging, database.NewSQLite, locking.NewProcessLock, logging.New),
)

var StorageModule = fx.Module("storage", fx.Provide(storage.NewScenarioRepository, storage.NewTaskRepository, storage.NewTaskBindingRepository, storage.NewParticipantRosterRepository, storage.NewMissingSubmitterRepository, storage.NewScenarioCourseBindingRepository, storage.NewScenarioCourseDataRepository, storage.NewSubmissionRepository, storage.NewPollingStateRepository, storage.NewReportRunRepository, fx.Annotate(storage.NewReportOutputRepository, fx.As(new(ports.ReportOutputRepository))), fx.Annotate(storage.NewReportResultRepository, fx.As(new(ports.ReportResultRepository))), fx.Annotate(storage.NewReportDeliveryRepository, fx.As(new(ports.ReportDeliveryRepository))), fx.Annotate(storage.NewReminderRepository, fx.As(new(ports.ReminderRepository))), fx.Annotate(storage.NewTeacherRepository, fx.As(new(ports.TeacherRepository))), fx.Annotate(storage.NewTelegramPairingRepository, fx.As(new(ports.TelegramPairingRepository))), fx.Annotate(storage.NewHistoryRepository, fx.As(new(ports.HistoryRepository)))))

var ClassroomStorageModule = fx.Module("classroom-storage", fx.Provide(storage.NewClassroomCourseRepository))

var ApplicationModule = fx.Module("application", fx.Provide(
	compiledScenarios,
	telegramSettings,
	application.NewPollingServiceWithScenarioCollectionAndRosterAndMissingAndReminders,
	application.NewReminderService,
	application.NewDeliveryService,
	application.NewTelegramPairingService,
	fx.Annotate(application.NewTeacherServiceWithDirectoryAndTelegram, fx.As(new(cli.TeacherListService)), fx.As(new(cli.TelegramPairingService))),
	fx.Annotate(application.NewScenarioCollectionServiceWithReportNotifier),
	fx.Annotate(application.NewReportService, fx.As(new(cli.ReportService))),
	fx.Annotate(application.NewClassroomService, fx.As(new(cli.ClassroomService))),
	fx.Annotate(application.NewCourseTaskService, fx.As(new(cli.CourseTaskService))),
))

func telegramSettings(cfg config.TelegramConfig) application.TelegramSettings {
	return application.TelegramSettings{BotUsername: cfg.Username}
}

var PollingModule = fx.Module("polling",
	fx.Provide(fx.Annotate(classroom.NewSubmissionSource, fx.As(new(ports.SubmissionSource)), fx.As(new(ports.ClassroomProfileSource)), fx.As(new(ports.ParticipantSource))), drive.NewMaterialRetriever, newMaterialParsing, renderer.NewConfiguredReportRenderer, fx.Annotate(notification.NewTelegramProvider, fx.As(new(ports.ReportReadyTelegramSender)), fx.As(new(ports.TelegramUpdateSource)), fx.As(new(ports.TelegramPairingMessenger))), fx.Annotate(notification.NewReportReadyNotifierWithTelegramAndProfile, fx.As(new(ports.ReportReadyNotifier))), fx.Annotate(notification.NewPreliminaryReportNotifierWithProfile, fx.As(new(ports.PreliminaryReportNotifier))), configuredReminderProviders, configuredDeliveryProviders),
	fx.Invoke(configurePollingDeadlines, configureTelegramPairing, configureApplicationLogging, configureReminderTemplate),
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

type applicationLoggingParams struct {
	fx.In
	Logger    *zap.Logger
	Polling   *application.PollingService
	Collector *application.ScenarioCollectionService
}

type reminderTemplateParams struct {
	fx.In
	Service *application.ReminderService
	Report  config.ReportConfig
}

func configureReminderTemplate(params reminderTemplateParams) {
	params.Service.SetTemplatePath(params.Report.ReminderPath)
}

func configureApplicationLogging(params applicationLoggingParams) {
	params.Polling.SetLogger(params.Logger)
	params.Collector.SetLogger(params.Logger)
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
	Logger    *zap.Logger                         `optional:"true"`
	Lifecycle fx.Lifecycle
}

func configureTelegramPairing(params telegramPairingParams) {
	if params.Telegram.Token == "" || params.Service == nil {
		return
	}
	var cancel context.CancelFunc
	params.Lifecycle.Append(fx.Hook{
		OnStart: func(startContext context.Context) error {
			var ctx context.Context
			ctx, cancel = context.WithCancel(startContext)
			go runTelegramPairing(ctx, params.Service, params.Logger)
			return nil
		},
		OnStop: func(context.Context) error {
			if cancel != nil {
				cancel()
			}
			return nil
		},
	})
}

func runTelegramPairing(ctx context.Context, service *application.TelegramPairingService, logger *zap.Logger) {
	if logger == nil {
		logger = zap.NewNop()
	}
	for {
		if err := service.Run(ctx); err != nil && ctx.Err() == nil {
			logger.Error("Telegram pairing listener stopped; retrying", zap.Error(err))
		}
		if ctx.Err() != nil {
			return
		}
		timer := time.NewTimer(time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
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

func NewFxLogger(logger *zap.Logger) fxevent.Logger {
	return &fxevent.ZapLogger{Logger: logger}
}
