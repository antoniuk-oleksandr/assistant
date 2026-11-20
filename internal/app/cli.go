package app

import (
	"context"
	"errors"

	"github.com/alex/assistent/internal/application"
	"github.com/alex/assistent/internal/fxmodules"
	"github.com/alex/assistent/internal/platform/locking"
	cli "github.com/alex/assistent/internal/presentation/cobra"
	"go.uber.org/fx"
)

func RunCLI(ctx context.Context, args []string) (err error) {
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
	startupCtx, cancel := context.WithTimeout(ctx, fxApp.StartTimeout())
	err = fxApp.Start(startupCtx)
	cancel()
	if err != nil {
		return err
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), fxApp.StopTimeout())
		defer cancel()
		err = errors.Join(err, fxApp.Stop(shutdownCtx))
	}()
	if len(args) == 0 {
		if err := processLock.Acquire(); err != nil {
			return err
		}
		defer processLock.Release()
		if err := polling.Run(ctx, application.DefaultPollInterval); err != nil {
			return err
		}
		return nil
	}
	root.SetArgs(append([]string{}, args...))
	return root.ExecuteContext(ctx)
}
