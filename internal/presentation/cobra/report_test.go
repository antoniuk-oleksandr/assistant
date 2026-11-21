package cli_test

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/alex/assistent/internal/application"
	"github.com/alex/assistent/internal/domain/scenario"
	cli "github.com/alex/assistent/internal/presentation/cobra"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestReportCommands(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Report Commands")
}

type reportServiceStub struct {
	calls []string
	err   error
	items []application.TaskBindingResult
	data  []scenario.CourseDataField
}

func (s *reportServiceStub) Configure(_ context.Context, courseID string) error {
	s.calls = append(s.calls, "configure:"+courseID)
	return s.err
}
func (s *reportServiceStub) BindTask(_ context.Context, taskID string) error {
	s.calls = append(s.calls, "bind:"+taskID)
	return s.err
}
func (s *reportServiceStub) UnbindTask(_ context.Context, taskID string) error {
	s.calls = append(s.calls, "unbind:"+taskID)
	return s.err
}
func (s *reportServiceStub) ListTasks(context.Context) ([]application.TaskBindingResult, error) {
	s.calls = append(s.calls, "list")
	return s.items, s.err
}
func (s *reportServiceStub) SetData(_ context.Context, courseID string, _ map[string]string) error {
	s.calls = append(s.calls, "set:"+courseID)
	return s.err
}
func (s *reportServiceStub) ListData(_ context.Context, courseID string) ([]scenario.CourseDataField, error) {
	s.calls = append(s.calls, "data-list:"+courseID)
	return s.data, s.err
}

var _ = Describe("report commands", func() {
	It("configures, binds, and unbinds the report workflow", func() {
		service := &reportServiceStub{}
		command := cli.NewReportCommand(service)
		for _, args := range [][]string{{"configure", "course-1"}, {"task-bind", "task-1"}, {"task-unbind", "task-1"}} {
			command.SetArgs(args)
			Expect(command.Execute()).To(Succeed())
		}
		Expect(service.calls).To(Equal([]string{"configure:course-1", "bind:task-1", "unbind:task-1"}))
	})

	It("prints report tasks and metadata", func() {
		service := &reportServiceStub{items: []application.TaskBindingResult{{TaskID: "task-1", CourseID: "course-1", Title: "Report", Status: "active"}}, data: []scenario.CourseDataField{{Name: "department_name", Value: "Science"}}}
		command := cli.NewReportCommand(service)
		var output bytes.Buffer
		command.SetOut(&output)
		command.SetArgs([]string{"task-list"})
		Expect(command.Execute()).To(Succeed())
		Expect(output.String()).To(ContainSubstring("task-1"))
		output.Reset()
		command.SetArgs([]string{"data-list", "course-1"})
		Expect(command.Execute()).To(Succeed())
		Expect(output.String()).To(ContainSubstring("department_name"))
		Expect(service.calls).To(Equal([]string{"list", "data-list:course-1"}))
	})

	It("parses repeated metadata fields and rejects malformed input", func() {
		service := &reportServiceStub{}
		command := cli.NewReportCommand(service)
		command.SetArgs([]string{"data-set", "course-1", "--field", "department_name=Science", "--field", "department_email="})
		Expect(command.Execute()).To(Succeed())
		command = cli.NewReportCommand(service)
		command.SetArgs([]string{"data-set", "course-1", "--field", "invalid"})
		Expect(command.Execute()).To(MatchError(ContainSubstring("name=value")))
		command = cli.NewReportCommand(service)
		command.SetArgs([]string{"data-set", "course-1"})
		Expect(command.Execute()).To(MatchError(ContainSubstring("at least one")))
	})

	It("propagates service errors", func() {
		service := &reportServiceStub{err: errors.New("report failed")}
		command := cli.NewReportCommand(service)
		command.SetArgs([]string{"configure", "course-1"})
		Expect(command.Execute()).To(MatchError("report failed"))
	})
})
