package app

import (
	"context"

	"github.com/alex/assistent/internal/application"
	"github.com/alex/assistent/internal/platform/locking"
	cli "github.com/alex/assistent/internal/presentation/cobra"
)

func RunMode(ctx context.Context, args []string, root *cli.RootCommand, polling *application.PollingService, processLock *locking.ProcessLock) error {
	if len(args) == 0 {
		if err := processLock.Acquire(); err != nil {
			return err
		}
		defer processLock.Release()
		if err := polling.RunScheduled(ctx); err != nil {
			return err
		}
		return nil
	}
	root.SetArgs(append([]string{}, args...))
	return root.ExecuteContext(ctx)
}
