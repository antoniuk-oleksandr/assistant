package cli_test

import (
	"bytes"
	"context"
	"errors"

	"time"

	"github.com/alex/assistent/internal/application"
	cli "github.com/alex/assistent/internal/presentation/cobra"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"
)

type classroomServiceStub struct {
	items []application.ClassroomCourseResult
	err   error
	calls []string
}

func (s *classroomServiceStub) Add(_ context.Context, reference string) error {
	s.calls = append(s.calls, "add:"+reference)
	return s.err
}
func (s *classroomServiceStub) List(context.Context) ([]application.ClassroomCourseResult, error) {
	s.calls = append(s.calls, "list")
	return s.items, s.err
}
func (s *classroomServiceStub) Remove(_ context.Context, reference string) error {
	s.calls = append(s.calls, "remove:"+reference)
	return s.err
}

type rejectOutputWriter struct{ rejected string }

func (w rejectOutputWriter) Write(data []byte) (int, error) {
	if bytes.Contains(data, []byte(w.rejected)) {
		return 0, errors.New("row output failed")
	}
	return len(data), nil
}

type failOnWriteWriter struct{ writes int }

func (w *failOnWriteWriter) Write(data []byte) (int, error) {
	w.writes++
	if w.writes == 2 {
		return 0, errors.New("row output failed")
	}
	return len(data), nil
}

var _ = Describe("classroom commands", func() {
	It("invokes add and remove with their references", func() {
		for _, tc := range []struct {
			args []string
			call string
		}{
			{[]string{"add", "123"}, "add:123"},
			{[]string{"remove", "123"}, "remove:123"},
		} {
			service := &classroomServiceStub{}
			command := cli.NewClassroomCommand(service)
			command.SetArgs(tc.args)
			Expect(command.Execute()).To(Succeed())
			Expect(service.calls).To(Equal([]string{tc.call}))
		}
	})

	It("prints registered courses and propagates list errors", func() {
		created := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
		service := &classroomServiceStub{items: []application.ClassroomCourseResult{{ExternalID: "123", SourceReference: "source", Status: "active", CreatedAt: created, UpdatedAt: created}}}
		command := cli.NewClassroomCommand(service)
		var output bytes.Buffer
		command.SetOut(&output)
		command.SetArgs([]string{"list"})
		Expect(command.Execute()).To(Succeed())
		Expect(output.String()).To(ContainSubstring("COURSE ID"))
		Expect(output.String()).To(ContainSubstring("123"))
		Expect(output.String()).To(ContainSubstring("source"))
		Expect(service.calls).To(Equal([]string{"list"}))

		service.err = errors.New("service failed")
		command = cli.NewClassroomCommand(service)
		command.SetArgs([]string{"list"})
		Expect(command.Execute()).To(MatchError("service failed"))
	})

	It("propagates classroom list output errors", func() {
		command := cli.NewClassroomCommand(&classroomServiceStub{items: []application.ClassroomCourseResult{{ExternalID: "123", SourceReference: "source", Status: "active"}}})
		command.SetOut(failingWriter{})
		command.SetArgs([]string{"list"})
		Expect(command.Execute()).To(MatchError("output failed"))
	})

	It("propagates errors while writing a classroom row", func() {
		command := cli.NewClassroomCommand(&classroomServiceStub{items: []application.ClassroomCourseResult{{ExternalID: "123", SourceReference: "source", Status: "active"}}})
		command.SetOut(rejectOutputWriter{rejected: "123"})
		command.SetArgs([]string{"list"})
		Expect(command.Execute()).To(MatchError("row output failed"))
	})

	It("propagates a writer failure on a later classroom row", func() {
		writer := &failOnWriteWriter{}
		command := cli.NewClassroomCommand(&classroomServiceStub{items: []application.ClassroomCourseResult{{ExternalID: "123", SourceReference: "source", Status: "active"}, {ExternalID: "456", SourceReference: "source", Status: "active"}}})
		command.SetOut(writer)
		command.SetArgs([]string{"list"})
		Expect(command.Execute()).To(MatchError("row output failed"))
	})

	It("rejects missing arguments", func() {
		command := cli.NewClassroomCommand(&classroomServiceStub{})
		for _, action := range []string{"add", "remove"} {
			command.SetArgs([]string{action})
			Expect(command.Execute()).To(MatchError(ContainSubstring("accepts 1 arg(s)")))
		}
	})

	It("adds the course command to the root command", func() {
		root := cli.NewRootCommandWithClassroom(&cobra.Command{Use: "scenario"}, cli.NewClassroomCommand(&classroomServiceStub{}))
		root.SetArgs([]string{"course", "add", "123"})
		Expect(root.Execute()).To(Succeed())
	})
})
