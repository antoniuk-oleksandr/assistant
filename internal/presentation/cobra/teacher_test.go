package cli_test

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/alex/assistent/internal/application"
	"github.com/alex/assistent/internal/domain/teacher"
	cli "github.com/alex/assistent/internal/presentation/cobra"
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

func TestTeacherListCommand(t *testing.T) {
	directory := &teacherDirectoryStub{teachers: []teacher.Recipient{{ID: 4, Name: "Teacher", Email: "teacher@example.com", ClassroomID: "classroom-1"}}}
	command := cli.NewTeacherCommand(&teacherPairingStub{}, directory)
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{"list", "course-1"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if directory.course != "course-1" || !bytes.Contains(output.Bytes(), []byte("TEACHER ID  NAME  EMAIL  CLASSROOM ID")) || !bytes.Contains(output.Bytes(), []byte("teacher@example.com")) {
		t.Fatalf("output=%q", output.String())
	}
}

func TestTeacherTelegramBindCommandPrintsPairingLink(t *testing.T) {
	command := cli.NewTeacherCommand(&teacherPairingStub{link: "https://t.me/my_bot?start=pair_token"}, &teacherDirectoryStub{})
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{"telegram-bind", "4"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(output.Bytes(), []byte("TELEGRAM PAIRING LINK")) || !bytes.Contains(output.Bytes(), []byte("https://t.me/my_bot?start=pair_token")) {
		t.Fatalf("output=%q", output.String())
	}
}

func TestTeacherCommandPropagatesDirectoryErrors(t *testing.T) {
	command := cli.NewTeacherCommand(&teacherPairingStub{}, &teacherDirectoryStub{err: errors.New("directory unavailable")})
	command.SetArgs([]string{"list", "course-1"})
	if err := command.Execute(); err == nil || err.Error() != "directory unavailable" {
		t.Fatalf("error=%v", err)
	}
}
