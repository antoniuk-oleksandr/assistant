package cli

import "github.com/spf13/cobra"

func NewRootCommand(reportCommand *cobra.Command) *RootCommand {
	root := &cobra.Command{
		Use:           "report-system",
		Short:         "Manage scheduled report collection",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(reportCommand)
	return &RootCommand{Command: root}
}

func NewRootCommandWithClassroom(reportCommand *cobra.Command, classroomCommand *ClassroomCommand) *RootCommand {
	root := NewRootCommand(reportCommand)
	root.AddCommand(classroomCommand.Command)
	return root
}

func NewRootCommandWithClassroomAndTeacher(reportCommand *cobra.Command, classroomCommand *ClassroomCommand, teacherCommand *TeacherCommand) *RootCommand {
	root := NewRootCommandWithClassroom(reportCommand, classroomCommand)
	root.AddCommand(teacherCommand.Command)
	return root
}

type RootCommand struct {
	*cobra.Command
}
