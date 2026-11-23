package cli

import (
	"context"
	"fmt"
	"text/tabwriter"

	"github.com/alex/assistent/internal/application"
	"github.com/spf13/cobra"
)

type ReportService interface {
	Configure(context.Context, string) error
	BindTask(context.Context, string) error
	UnbindTask(context.Context, string) error
	ListTasks(context.Context) ([]application.TaskBindingResult, error)
	SetData(context.Context, string, map[string]string) error
	ListData(context.Context, string) ([]application.CourseDataResult, error)
}

func NewReportCommand(service ReportService) *cobra.Command {
	command := &cobra.Command{Use: "report", Short: "Manage the departmental report workflow"}
	command.AddCommand(
		reportAction("configure", "Configure reporting for a registered course", service.Configure),
		reportAction("task-bind", "Bind a Classroom task to the report", service.BindTask),
		reportAction("task-unbind", "Stop processing a Classroom task", service.UnbindTask),
		newReportTaskListCommand(service),
		newReportDataSetCommand(service),
		newReportDataListCommand(service),
	)
	return command
}

func reportAction(use, description string, run func(context.Context, string) error) *cobra.Command {
	return &cobra.Command{Use: use + " <value>", Short: description, Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		return run(cmd.Context(), args[0])
	}}
}

func newReportTaskListCommand(service ReportService) *cobra.Command {
	return &cobra.Command{Use: "task-list", Short: "List tasks processed by the report", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		items, err := service.ListTasks(cmd.Context())
		if err != nil {
			return err
		}
		output := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
		if _, err := fmt.Fprintln(output, "TASK ID\tCOURSE ID\tTITLE\tSTATUS"); err != nil {
			return err
		}
		for _, item := range items {
			if _, err := fmt.Fprintf(output, "%s\t%s\t%s\t%s\n", item.TaskID, item.CourseID, item.Title, item.Status); err != nil {
				return err
			}
		}
		return output.Flush()
	}}
}

func newReportDataSetCommand(service ReportService) *cobra.Command {
	var values []string
	command := &cobra.Command{Use: "data-set <course-id>", Short: "Store report metadata for a course", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		fields := make(map[string]string, len(values))
		for _, value := range values {
			name, fieldValue, ok := splitReportField(value)
			if !ok {
				return fmt.Errorf("field must use name=value: %q", value)
			}
			fields[name] = fieldValue
		}
		if len(fields) == 0 {
			return fmt.Errorf("at least one --field name=value is required")
		}
		return service.SetData(cmd.Context(), args[0], fields)
	}}
	command.Flags().StringSliceVar(&values, "field", nil, "report field in name=value form (repeatable)")
	return command
}

func newReportDataListCommand(service ReportService) *cobra.Command {
	return &cobra.Command{Use: "data-list <course-id>", Short: "List stored report metadata", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		items, err := service.ListData(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		output := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
		if _, err := fmt.Fprintln(output, "FIELD\tVALUE"); err != nil {
			return err
		}
		for _, item := range items {
			if _, err := fmt.Fprintf(output, "%s\t%s\n", item.Name, item.Value); err != nil {
				return err
			}
		}
		return output.Flush()
	}}
}

func splitReportField(value string) (string, string, bool) {
	for index := 0; index < len(value); index++ {
		if value[index] == '=' {
			if index == 0 {
				return "", "", false
			}
			return value[:index], value[index+1:], true
		}
	}
	return "", "", false
}
