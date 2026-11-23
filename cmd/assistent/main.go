package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/alex/assistent/internal/app"
	"github.com/alex/assistent/internal/application"
	"github.com/alex/assistent/internal/fxmodules"
	"github.com/alex/assistent/internal/platform/locking"
	cli "github.com/alex/assistent/internal/presentation/cobra"
	"go.uber.org/fx"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	var root *cli.RootCommand
	var polling *application.PollingService
	var processLock *locking.ProcessLock
	fxApp := fx.New(
		fx.WithLogger(fxmodules.NewFxLogger),
		fxmodules.PlatformModule,
		fxmodules.StorageModule,
		fxmodules.ClassroomStorageModule,
		fxmodules.ApplicationModule,
		fxmodules.PollingModule,
		fxmodules.CLIModule,
		fx.Populate(&root, &polling, &processLock),
	)
	if err := fxApp.Start(ctx); err != nil {
		os.Exit(1)
	}
	err := app.RunMode(ctx, os.Args[1:], root, polling, processLock)
	if stopErr := fxApp.Stop(context.Background()); err == nil {
		err = stopErr
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
