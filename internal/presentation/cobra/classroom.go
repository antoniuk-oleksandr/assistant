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

type ClassroomCommand struct{ *cobra.Command }

func NewClassroomCommand(service ClassroomService) *ClassroomCommand {
	command := &cobra.Command{Use: "classroom", Short: "Manage registered Classroom courses"}
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
	}}, classroomAction("add", "Register a Classroom course", service.Add), classroomAction("remove", "Disable a Classroom course", service.Remove))
	return &ClassroomCommand{Command: command}
}
func classroomAction(verb, description string, run func(context.Context, string) error) *cobra.Command {
	return &cobra.Command{Use: verb + " <course-id-or-url>", Short: description, Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error { return run(cmd.Context(), args[0]) }}
}
