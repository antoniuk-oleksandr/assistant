package cli

import (
	"context"
	"fmt"
	"text/tabwriter"

	"github.com/alex/assistent/internal/application"
	"github.com/alex/assistent/internal/presentation/mappers"
	"github.com/spf13/cobra"
)

type ScenarioService interface {
	Add(context.Context, string) error
	List(context.Context) ([]application.ScenarioResult, error)
	Enable(context.Context, string) error
	Disable(context.Context, string) error
	Remove(context.Context, string) error
}

func NewScenarioCommand(service ScenarioService) *cobra.Command {
	command := &cobra.Command{Use: "scenario", Short: "Manage collection scenarios"}
	command.AddCommand(
		&cobra.Command{
			Use: "list", Short: "List registered scenarios", Args: cobra.NoArgs,
			RunE: func(cmd *cobra.Command, _ []string) error { return listScenarios(cmd, service) },
		},
		scenarioAction("add", "Register a compiled scenario", service.Add),
		scenarioAction("enable", "Enable a registered scenario", service.Enable),
		scenarioAction("disable", "Disable a registered scenario", service.Disable),
		scenarioAction("remove", "Remove a registered scenario", service.Remove),
	)
	return command
}

func scenarioAction(verb, description string, run func(context.Context, string) error) *cobra.Command {
	return &cobra.Command{
		Use: verb + " <name>", Short: description, Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error { return run(cmd.Context(), args[0]) },
	}
}

func listScenarios(cmd *cobra.Command, service ScenarioService) error {
	items, err := service.List(cmd.Context())
	if err != nil {
		return err
	}
	output := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintln(output, "NAME	STATUS	IMPLEMENTATION	CREATED AT	UPDATED AT	LAST RUN"); err != nil {
		return err
	}
	for _, item := range items {
		row := mappers.ScenarioToDTO(item)
		if _, err := fmt.Fprintf(output, "%s	%s	%s	%s	%s	%s\n",
			row.Name, row.Status, row.ImplementationType, row.CreatedAt, row.UpdatedAt, row.LastRunAt); err != nil {
			return err
		}
	}
	return output.Flush()
}
