package cli

import "github.com/spf13/cobra"

func NewRootCommand(scenarioCommand *cobra.Command) *RootCommand {
	root := &cobra.Command{
		Use:           "report-system",
		Short:         "Manage scheduled report collection",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(scenarioCommand)
	return &RootCommand{Command: root}
}

type RootCommand struct {
	*cobra.Command
}
