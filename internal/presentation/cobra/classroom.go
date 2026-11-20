package cli

import (
	"context"
	"fmt"
	"github.com/alex/assistent/internal/application"
	"github.com/alex/assistent/internal/presentation/mappers"
	"github.com/spf13/cobra"
	"text/tabwriter"
)

type ClassroomService interface {
	Add(context.Context, string) error
	List(context.Context) ([]application.ClassroomCourseResult, error)
	Remove(context.Context, string) error
}

type CourseTaskService interface {
	List(context.Context, string) ([]application.CourseTaskResult, error)
}

type ClassroomCommand struct{ *cobra.Command }

func NewClassroomCommand(service ClassroomService) *ClassroomCommand {
	command := &cobra.Command{Use: "course", Aliases: []string{"classroom"}, Short: "Manage registered courses"}
	command.AddCommand(&cobra.Command{Use: "list", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		items, err := service.List(cmd.Context())
		if err != nil {
			return err
		}
		output := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
		if _, err := fmt.Fprintln(output, "COURSE ID\tSOURCE\tSTATUS\tCREATED AT\tUPDATED AT"); err != nil {
			return err
		}
		for _, item := range items {
			row := mappers.ClassroomCourseToDTO(item)
			if _, err := fmt.Fprintf(output, "%s\t%s\t%s\t%s\t%s\n", row.ExternalID, row.SourceReference, row.Status, row.CreatedAt, row.UpdatedAt); err != nil {
				return err
			}
		}
		return output.Flush()
	}}, classroomAction("add", "Register a course", service.Add), classroomAction("remove", "Disable a course", service.Remove))
	return &ClassroomCommand{Command: command}
}

func NewClassroomCommandWithTasks(service ClassroomService, tasks CourseTaskService) *ClassroomCommand {
	command := NewClassroomCommand(service)
	command.AddCommand(&cobra.Command{Use: "tasks <course-id-or-url>", Short: "List discovered tasks for a course", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		items, err := tasks.List(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		output := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
		if _, err := fmt.Fprintln(output, "TASK ID\tCOURSE ID\tTITLE\tSTATUS"); err != nil {
			return err
		}
		for _, item := range items {
			if _, err := fmt.Fprintf(output, "%s\t%s\t%s\t%s\n", item.ExternalID, item.CourseID, item.Title, item.Status); err != nil {
				return err
			}
		}
		return output.Flush()
	}})
	return command
}
func classroomAction(verb, description string, run func(context.Context, string) error) *cobra.Command {
	return &cobra.Command{Use: verb + " <course-id-or-url>", Short: description, Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error { return run(cmd.Context(), args[0]) }}
}
