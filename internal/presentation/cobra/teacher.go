package cli

import (
	"context"
	"fmt"

	"github.com/alex/assistent/internal/application"
	"github.com/alex/assistent/internal/domain/teacher"
	"github.com/spf13/cobra"
)

type TeacherContactService interface {
	RegisterPhone(context.Context, string, string) (application.TeacherContactResult, error)
}

type TeacherPhoneService interface {
	SetPhone(context.Context, int64, string) (teacher.Channel, error)
}

type TeacherListService interface {
	List(context.Context) ([]teacher.Recipient, error)
}

type TeacherCommand struct{ *cobra.Command }

func NewTeacherCommand(service TeacherContactService) *TeacherCommand {
	return newTeacherCommand(service)
}

func NewTeacherCommandWithPhone(service TeacherPhoneService, directory TeacherListService) *TeacherCommand {
	var phone string
	command := &cobra.Command{Use: "teacher", Short: "Manage teacher contacts"}
	list := &cobra.Command{
		Use:   "list",
		Short: "List teachers discovered from Google Classroom",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			teachers, err := directory.List(cmd.Context())
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintln(cmd.OutOrStdout(), "TEACHER ID  NAME  EMAIL  CLASSROOM ID"); err != nil {
				return err
			}
			for _, item := range teachers {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%d  %s  %s  %s\n", item.ID, item.Name, item.Email, item.ClassroomID); err != nil {
					return err
				}
			}
			return nil
		},
	}
	command.AddCommand(list)
	phoneSet := &cobra.Command{
		Use:   "phone-set <teacher-id>",
		Short: "Set the phone number for a discovered teacher",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var id int64
			if _, err := fmt.Sscan(args[0], &id); err != nil || id <= 0 {
				return fmt.Errorf("teacher-id must be a positive integer")
			}
			channel, err := service.SetPhone(cmd.Context(), id, phone)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "TEACHER ID  PHONE\n%d  %s\n", channel.RecipientID, channel.Destination)
			return err
		},
	}
	phoneSet.Flags().StringVar(&phone, "phone", "", "phone number to register")
	_ = phoneSet.MarkFlagRequired("phone")
	command.AddCommand(phoneSet)
	return &TeacherCommand{Command: command}
}

func newTeacherCommand(service TeacherContactService) *TeacherCommand {
	var phone string
	command := &cobra.Command{Use: "teacher", Short: "Manage teacher contacts"}
	phoneSet := &cobra.Command{
		Use:   "phone-set <teacher-id>",
		Short: "Register a phone number for a synchronized teacher",
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
