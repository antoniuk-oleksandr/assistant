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
