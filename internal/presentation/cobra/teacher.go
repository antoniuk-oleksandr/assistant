package cli

import (
	"context"
	"fmt"

	"github.com/alex/assistent/internal/application"
	"github.com/spf13/cobra"
)

type TelegramPairingService interface {
	CreateTelegramPairing(context.Context, string) (string, error)
	WaitForTelegramPairing(context.Context, string) error
}

type TeacherListService interface {
	ListForCourse(context.Context, string) ([]application.TeacherDirectoryEntry, error)
}

type TeacherCommand struct{ *cobra.Command }

func NewTeacherCommand(pairing TelegramPairingService, directory TeacherListService) *TeacherCommand {
	command := &cobra.Command{Use: "teacher", Short: "Manage teacher contacts"}
	list := &cobra.Command{
		Use:   "list <course-id-or-url>",
		Short: "List teachers discovered from Google Classroom",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			teachers, err := directory.ListForCourse(cmd.Context(), args[0])
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
	telegramBind := &cobra.Command{
		Use:   "telegram-bind <teacher-id-or-classroom-user-id>",
		Short: "Generate a private Telegram binding link for a discovered teacher",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			link, err := pairing.CreateTelegramPairing(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if _, err = fmt.Fprintf(cmd.OutOrStdout(), "TELEGRAM PAIRING LINK\n%s\n", link); err != nil {
				return err
			}
			return pairing.WaitForTelegramPairing(cmd.Context(), link)
		},
	}
	command.AddCommand(telegramBind)
	return &TeacherCommand{Command: command}
}
