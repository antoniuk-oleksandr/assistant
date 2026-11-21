package application_test

import (
	"context"
	"errors"
	"github.com/alex/assistent/internal/application"
	"github.com/alex/assistent/internal/domain/ports"
	"github.com/alex/assistent/internal/domain/teacher"
	"strings"
	"testing"
)

type teacherRepo struct {
	value    teacher.Recipient
	channels []teacher.Channel
	err      error
}

func (r *teacherRepo) Create(_ context.Context, v *teacher.Recipient) error {
	if r.err != nil {
		return r.err
	}
	v.ID = 1
	r.value = *v
	return nil
}
func (r *teacherRepo) Update(_ context.Context, v *teacher.Recipient) error {
	if r.err != nil {
		return r.err
	}
	r.value = *v
	return nil
}
func (r *teacherRepo) Get(context.Context, int64) (teacher.Recipient, error) { return r.value, nil }
func (r *teacherRepo) List(context.Context) ([]teacher.Recipient, error) {
	return []teacher.Recipient{r.value}, nil
}
func (r *teacherRepo) CreateChannel(_ context.Context, v *teacher.Channel) error {
	if r.err != nil {
		return r.err
	}
	v.ID = 1
	r.channels = append(r.channels, *v)
	return nil
}
func (r *teacherRepo) UpdateChannel(_ context.Context, v *teacher.Channel) error {
	if r.err != nil {
		return r.err
	}
	r.channels[0] = *v
	return nil
}
func (r *teacherRepo) ListActiveChannels(context.Context, int64) ([]teacher.Channel, error) {
	out := []teacher.Channel{}
	for _, v := range r.channels {
		if v.Status == teacher.Active {
			out = append(out, v)
		}
	}
	return out, nil
}

func TestTeacherServiceManagesRecipientsAndChannels(t *testing.T) {
	repo := &teacherRepo{}
	service := application.NewTeacherService(repo)
	rec, err := service.Add(context.Background(), "Teacher", "teacher@example.com", "classroom-1")
	if err != nil || rec.Status != teacher.Active {
		t.Fatalf("recipient=%#v err=%v", rec, err)
	}
	if err := service.SetStatus(context.Background(), rec.ID, teacher.Disabled); err != nil {
		t.Fatal(err)
	}
	channel, err := service.AddChannel(context.Background(), rec.ID, "email", "teacher@example.com")
	if err != nil || channel.Status != teacher.Active {
		t.Fatalf("channel=%#v err=%v", channel, err)
	}
	if err := service.SetChannelStatus(context.Background(), channel, teacher.Disabled); err != nil {
		t.Fatal(err)
	}
	if channels, err := service.ActiveChannels(context.Background(), rec.ID); err != nil || len(channels) != 0 {
		t.Fatalf("active channels=%#v err=%v", channels, err)
	}
}

func TestTeacherServicePropagatesRepositoryErrors(t *testing.T) {
	service := application.NewTeacherService(&teacherRepo{err: errors.New("write failed")})
	if _, err := service.Add(context.Background(), "x", "x@example.com", ""); err == nil {
		t.Fatal("add error missing")
	}
	if _, err := service.AddChannel(context.Background(), 1, "email", "x"); err == nil {
		t.Fatal("channel error missing")
	}
}

type profileSourceStub struct {
	profile teacher.Profile
	err     error
}

func (s profileSourceStub) GetProfile(context.Context, string) (teacher.Profile, error) {
	return s.profile, s.err
}

type contactRepositoryStub struct {
	recipient teacher.Recipient
	channel   teacher.Channel
	err       error
}

func (r *contactRepositoryStub) RegisterContact(_ context.Context, recipient *teacher.Recipient, channel *teacher.Channel) error {
	if r.err != nil {
		return r.err
	}
	recipient.ID = 7
	channel.ID = 8
	channel.RecipientID = recipient.ID
	r.recipient, r.channel = *recipient, *channel
	return nil
}

func TestTeacherContactServiceResolvesProfileBeforeAtomicRegistration(t *testing.T) {
	repo := &contactRepositoryStub{}
	service := application.NewTeacherContactService(repo, profileSourceStub{profile: teacher.Profile{ClassroomID: "user-1", Name: "Teacher", Email: "teacher@example.com"}})
	result, err := service.RegisterPhone(context.Background(), "user-1", "+380000000")
	if err != nil || result.Recipient.Email != "teacher@example.com" || result.Channel.Kind != "phone" || result.Channel.RecipientID != 7 {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	if repo.recipient.ClassroomID != "user-1" || repo.channel.Destination != "+380000000" {
		t.Fatalf("stored recipient=%#v channel=%#v", repo.recipient, repo.channel)
	}
}

func TestTeacherContactServiceDoesNotWriteWhenProfileLookupFails(t *testing.T) {
	repo := &contactRepositoryStub{}
	service := application.NewTeacherContactService(repo, profileSourceStub{err: errors.New("profile unavailable")})
	if _, err := service.RegisterPhone(context.Background(), "user-1", "+380000000"); err == nil || !strings.Contains(err.Error(), "resolve Classroom profile") {
		t.Fatalf("error=%v", err)
	}
	if repo.recipient.ID != 0 || repo.channel.ID != 0 {
		t.Fatalf("contact was partially written: %#v %#v", repo.recipient, repo.channel)
	}
}

func TestTeacherContactServicePropagatesAtomicWriteFailure(t *testing.T) {
	service := application.NewTeacherContactService(&contactRepositoryStub{err: errors.New("write failed")}, profileSourceStub{profile: teacher.Profile{Name: "Teacher", Email: "teacher@example.com"}})
	if _, err := service.RegisterPhone(context.Background(), "user-1", "+380000000"); err == nil || !strings.Contains(err.Error(), "register teacher contact") {
		t.Fatalf("error=%v", err)
	}
}

var _ ports.TeacherRepository = (*teacherRepo)(nil)
