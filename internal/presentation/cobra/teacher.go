package cli

import (
	"context"
	"fmt"

	"github.com/alex/assistent/internal/application"
	"github.com/spf13/cobra"
)

type TeacherContactService interface {
	RegisterPhone(context.Context, string, string) (application.TeacherContactResult, error)
}

type TeacherCommand struct{ *cobra.Command }

func NewTeacherCommand(service TeacherContactService) *TeacherCommand {
	var phone string
	command := &cobra.Command{Use: "teacher", Short: "Manage teacher contacts"}
	phoneSet := &cobra.Command{
		Use:   "phone-set <classroom-user-id>",
		Short: "Resolve a Classroom user and register a phone channel",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := service.RegisterPhone(cmd.Context(), args[0], phone)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "TEACHER ID  NAME  EMAIL  PHONE\n%d  %s  %s  %s\n", result.Recipient.ID, result.Recipient.Name, result.Recipient.Email, result.Channel.Destination)
			return err
		},
	}
	phoneSet.Flags().StringVar(&phone, "phone", "", "phone number to register")
	_ = phoneSet.MarkFlagRequired("phone")
	command.AddCommand(phoneSet)
	return &TeacherCommand{Command: command}
}
