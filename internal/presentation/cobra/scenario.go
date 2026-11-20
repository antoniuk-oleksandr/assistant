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

type ScenarioTaskBindingService interface {
	BindTask(context.Context, string, string) error
	UnbindTask(context.Context, string) error
	ListTaskBindings(context.Context, string) ([]application.TaskBindingResult, error)
}

func NewScenarioCommand(service ScenarioService, bindingServices ...ScenarioTaskBindingService) *cobra.Command {
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
	if len(bindingServices) > 0 {
		binding := bindingServices[0]
		command.AddCommand(
			bindingAction("task-bind", "Bind a task to a scenario", func(ctx context.Context, args []string) error { return binding.BindTask(ctx, args[0], args[1]) }),
			&cobra.Command{Use: "task-unbind <task-id>", Short: "Unbind a task from all scenarios", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error { return binding.UnbindTask(cmd.Context(), args[0]) }},
			&cobra.Command{Use: "task-list <scenario-name>", Short: "List tasks bound to a scenario", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error { return listTaskBindings(cmd, binding, args[0]) }},
		)
	}
	return command
}

func bindingAction(use, description string, run func(context.Context, []string) error) *cobra.Command {
	return &cobra.Command{Use: use + " <scenario-name> <task-id>", Short: description, Args: cobra.ExactArgs(2), RunE: func(cmd *cobra.Command, args []string) error { return run(cmd.Context(), args) }}
}

func listTaskBindings(cmd *cobra.Command, service ScenarioTaskBindingService, name string) error {
	items, err := service.ListTaskBindings(cmd.Context(), name)
	if err != nil {
		return err
	}
	output := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintln(output, "TASK ID	COURSE ID	TITLE	STATUS"); err != nil {
		return err
	}
	for _, item := range items {
		if _, err := fmt.Fprintf(output, "%s	%s	%s	%s\n", item.TaskID, item.CourseID, item.Title, item.Status); err != nil {
			return err
		}
	}
	return output.Flush()
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
