package app

import (
	"context"
	"errors"

	"github.com/alex/assistent/internal/fxmodules"
	cli "github.com/alex/assistent/internal/presentation/cobra"
	"go.uber.org/fx"
)

func RunCLI(ctx context.Context, args []string) (err error) {
	var root *cli.RootCommand
	application := fx.New(
		fx.WithLogger(fxmodules.NewFxLogger),
		fxmodules.PlatformModule,
		fxmodules.StorageModule,
		fxmodules.ClassroomStorageModule,
		fxmodules.ApplicationModule,
		fxmodules.CLIModule,
		fx.Populate(&root),
	)
	startupCtx, cancel := context.WithTimeout(ctx, application.StartTimeout())
	err = application.Start(startupCtx)
	cancel()
	if err != nil {
		return err
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), application.StopTimeout())
		defer cancel()
		err = errors.Join(err, application.Stop(shutdownCtx))
	}()
	root.SetArgs(append([]string{}, args...))
	return root.ExecuteContext(ctx)
}
