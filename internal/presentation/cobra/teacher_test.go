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

type teacherContactStub struct {
	result application.TeacherContactResult
	err    error
	userID string
	phone  string
}

type teacherDirectoryStub struct {
	teachers []teacher.Recipient
	err      error
}

func (s teacherDirectoryStub) List(context.Context) ([]teacher.Recipient, error) {
	return s.teachers, s.err
}

type teacherPhoneStub struct {
	id    int64
	phone string
	err   error
}

func (s *teacherPhoneStub) SetPhone(_ context.Context, id int64, phone string) (teacher.Channel, error) {
	s.id, s.phone = id, phone
	return teacher.Channel{RecipientID: id, Destination: phone}, s.err
}

func (s *teacherContactStub) RegisterPhone(_ context.Context, userID, phone string) (application.TeacherContactResult, error) {
	s.userID, s.phone = userID, phone
	return s.result, s.err
}

func TestTeacherPhoneSetCommand(t *testing.T) {
	service := &teacherContactStub{result: application.TeacherContactResult{Recipient: teacher.Recipient{ID: 3, Name: "Teacher", Email: "teacher@example.com"}, Channel: teacher.Channel{Destination: "+380000000"}}}
	command := cli.NewTeacherCommand(service)
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{"phone-set", "classroom-user-1", "--phone", "+380000000"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if service.userID != "classroom-user-1" || service.phone != "+380000000" || !bytes.Contains(output.Bytes(), []byte("teacher@example.com")) {
		t.Fatalf("service=%#v output=%q", service, output.String())
	}
}

func TestTeacherPhoneSetCommandPropagatesErrorsAndRequiresPhone(t *testing.T) {
	command := cli.NewTeacherCommand(&teacherContactStub{err: errors.New("profile unavailable")})
	command.SetArgs([]string{"phone-set", "user-1", "--phone", "+380000000"})
	if err := command.Execute(); err == nil || err.Error() != "profile unavailable" {
		t.Fatalf("error=%v", err)
	}
	command = cli.NewTeacherCommand(&teacherContactStub{})
	command.SetArgs([]string{"phone-set", "user-1"})
	if err := command.Execute(); err == nil {
		t.Fatal("missing phone unexpectedly succeeded")
	}
}

func TestTeacherCommandWithPhoneListsTeachersAndSetsPhone(t *testing.T) {
	phone := &teacherPhoneStub{}
	command := cli.NewTeacherCommandWithPhone(phone, teacherDirectoryStub{teachers: []teacher.Recipient{{ID: 4, Name: "Teacher", Email: "teacher@example.com", ClassroomID: "classroom-1"}}})
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{"list"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(output.Bytes(), []byte("TEACHER ID  NAME  EMAIL  CLASSROOM ID")) || !bytes.Contains(output.Bytes(), []byte("teacher@example.com")) {
		t.Fatalf("list output=%q", output.String())
	}
	command.SetArgs([]string{"phone-set", "4", "--phone", "+380501234567"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if phone.id != 4 || phone.phone != "+380501234567" || !bytes.Contains(output.Bytes(), []byte("TEACHER ID  PHONE")) {
		t.Fatalf("phone service=%#v output=%q", phone, output.String())
	}
}

func TestTeacherCommandWithPhoneValidatesIDAndDirectoryErrors(t *testing.T) {
	command := cli.NewTeacherCommandWithPhone(&teacherPhoneStub{}, teacherDirectoryStub{err: errors.New("directory unavailable")})
	command.SetArgs([]string{"list"})
	if err := command.Execute(); err == nil || err.Error() != "directory unavailable" {
		t.Fatalf("directory error=%v", err)
	}
	command = cli.NewTeacherCommandWithPhone(&teacherPhoneStub{}, teacherDirectoryStub{})
	command.SetArgs([]string{"phone-set", "not-an-id", "--phone", "+380501234567"})
	if err := command.Execute(); err == nil || err.Error() != "teacher-id must be a positive integer" {
		t.Fatalf("ID error=%v", err)
	}
}
