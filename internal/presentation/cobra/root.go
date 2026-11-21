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

func NewRootCommandWithClassroom(scenarioCommand *cobra.Command, classroomCommand *ClassroomCommand) *RootCommand {
	root := NewRootCommand(scenarioCommand)
	root.AddCommand(classroomCommand.Command)
	return root
}

func NewRootCommandWithClassroomAndTeacher(scenarioCommand *cobra.Command, classroomCommand *ClassroomCommand, teacherCommand *TeacherCommand) *RootCommand {
	root := NewRootCommandWithClassroom(scenarioCommand, classroomCommand)
	root.AddCommand(teacherCommand.Command)
	return root
}

type RootCommand struct {
	*cobra.Command
}
