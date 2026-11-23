package cli_test

import (
	"bytes"
	"context"
	"errors"

	"github.com/alex/assistent/internal/application"
	"github.com/alex/assistent/internal/domain/teacher"
	cli "github.com/alex/assistent/internal/presentation/cobra"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type teacherDirectoryStub struct {
	teachers []teacher.Recipient
	err      error
	course   string
}

func (s *teacherDirectoryStub) ListForCourse(_ context.Context, course string) ([]application.TeacherDirectoryEntry, error) {
	s.course = course
	entries := make([]application.TeacherDirectoryEntry, 0, len(s.teachers))
	for _, item := range s.teachers {
		entries = append(entries, application.TeacherDirectoryEntry{Recipient: item})
	}
	return entries, s.err
}

type teacherPairingStub struct{ link string }

func (s *teacherPairingStub) CreateTelegramPairing(context.Context, string) (string, error) {
	return s.link, nil
}

type failingTeacherWriter struct{}

func (failingTeacherWriter) Write([]byte) (int, error) { return 0, errors.New("output failed") }

type rejectingTeacherWriter struct{ value string }

func (w rejectingTeacherWriter) Write(data []byte) (int, error) {
	if bytes.Contains(data, []byte(w.value)) {
		return 0, errors.New("row output failed")
	}
	return len(data), nil
}

var _ = Describe("Teacher commands", func() {
	It("lists teachers", func() {
		directory := &teacherDirectoryStub{teachers: []teacher.Recipient{{ID: 4, Name: "Teacher", Email: "teacher@example.com", ClassroomID: "classroom-1"}}}
		command := cli.NewTeacherCommand(&teacherPairingStub{}, directory)
		var output bytes.Buffer
		command.SetOut(&output)
		command.SetArgs([]string{"list", "course-1"})
		Expect(command.Execute()).To(Succeed())
		Expect(directory.course).To(Equal("course-1"))
		Expect(output.String()).To(ContainSubstring("TEACHER ID  NAME  EMAIL  CLASSROOM ID"))
		Expect(output.String()).To(ContainSubstring("teacher@example.com"))
	})

	It("prints a Telegram pairing link", func() {
		command := cli.NewTeacherCommand(&teacherPairingStub{link: "https://t.me/my_bot?start=pair_token"}, &teacherDirectoryStub{})
		var output bytes.Buffer
		command.SetOut(&output)
		command.SetArgs([]string{"telegram-bind", "4"})
		Expect(command.Execute()).To(Succeed())
		Expect(output.String()).To(ContainSubstring("TELEGRAM PAIRING LINK"))
		Expect(output.String()).To(ContainSubstring("https://t.me/my_bot?start=pair_token"))
	})

	It("propagates directory errors", func() {
		command := cli.NewTeacherCommand(&teacherPairingStub{}, &teacherDirectoryStub{err: errors.New("directory unavailable")})
		command.SetArgs([]string{"list", "course-1"})
		Expect(command.Execute()).To(MatchError("directory unavailable"))
	})

	It("propagates output errors", func() {
		command := cli.NewTeacherCommand(&teacherPairingStub{link: "link"}, &teacherDirectoryStub{teachers: []teacher.Recipient{{ID: 1}}})
		command.SetOut(failingTeacherWriter{})
		command.SetArgs([]string{"list", "course-1"})
		Expect(command.Execute()).To(MatchError("output failed"))
		command = cli.NewTeacherCommand(&teacherPairingStub{link: "link"}, &teacherDirectoryStub{})
		command.SetOut(failingTeacherWriter{})
		command.SetArgs([]string{"telegram-bind", "1"})
		Expect(command.Execute()).To(MatchError("output failed"))
	})

	It("propagates row output errors", func() {
		command := cli.NewTeacherCommand(&teacherPairingStub{}, &teacherDirectoryStub{teachers: []teacher.Recipient{{ID: 1, Name: "Teacher"}}})
		command.SetOut(rejectingTeacherWriter{value: "Teacher"})
		command.SetArgs([]string{"list", "course-1"})
		Expect(command.Execute()).To(MatchError("row output failed"))
	})
})
