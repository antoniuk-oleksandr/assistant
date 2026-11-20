package fxmodules

import (
	"github.com/alex/assistent/internal/adapters/storage"
	"github.com/alex/assistent/internal/application"
	"github.com/alex/assistent/internal/domain/scenario"
	"github.com/alex/assistent/internal/platform/config"
	"github.com/alex/assistent/internal/platform/database"
	cli "github.com/alex/assistent/internal/presentation/cobra"
	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"
	"go.uber.org/zap"
)

var PlatformModule = fx.Module("platform",
	fx.Provide(config.Load, config.Database, database.NewSQLite),
)

var StorageModule = fx.Module("storage", fx.Provide(storage.NewScenarioRepository))

var ApplicationModule = fx.Module("application", fx.Provide(
	compiledScenarios,
	fx.Annotate(application.NewScenarioService, fx.As(new(cli.ScenarioService))),
))

var CLIModule = fx.Module("cli", fx.Provide(cli.NewScenarioCommand, cli.NewRootCommand))

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
