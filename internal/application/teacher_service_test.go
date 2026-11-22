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
	value      teacher.Recipient
	channels   []teacher.Channel
	err        error
	channelErr error
	listErr    error
	createErr  error
	updateErr  error
}

func (r *teacherRepo) Create(_ context.Context, v *teacher.Recipient) error {
	if r.err != nil {
		return r.err
	}
	if r.createErr != nil {
		return r.createErr
	}
	v.ID = 1
	r.value = *v
	return nil
}
func (r *teacherRepo) Update(_ context.Context, v *teacher.Recipient) error {
	if r.err != nil {
		return r.err
	}
	if r.updateErr != nil {
		return r.updateErr
	}
	r.value = *v
	return nil
}
func (r *teacherRepo) Get(context.Context, int64) (teacher.Recipient, error) {
	if r.err != nil {
		return teacher.Recipient{}, r.err
	}
	return r.value, nil
}
func (r *teacherRepo) List(context.Context) ([]teacher.Recipient, error) {
	if r.listErr != nil {
		return nil, r.listErr
	}
	return []teacher.Recipient{r.value}, nil
}
func (r *teacherRepo) CreateChannel(_ context.Context, v *teacher.Channel) error {
	if r.err != nil {
		return r.err
	}
	if r.channelErr != nil {
		return r.channelErr
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
	if err := service.SetStatus(context.Background(), 1, teacher.Disabled); err == nil {
		t.Fatal("status lookup error missing")
	}
	if _, err := application.NewTeacherService(&teacherRepo{}).AddChannel(context.Background(), 1, "unsupported", "x"); err == nil {
		t.Fatal("unsupported channel accepted")
	}
}

func TestTeacherServiceSetsPhoneForExistingTeacher(t *testing.T) {
	repo := &teacherRepo{value: teacher.Recipient{ID: 4}}
	channel, err := application.NewTeacherService(repo).SetPhone(context.Background(), 4, "+380501234567")
	if err != nil || channel.RecipientID != 4 || channel.Kind != "phone" || channel.Destination != "+380501234567" {
		t.Fatalf("channel=%#v err=%v", channel, err)
	}
}

func TestTeacherServiceRejectsInvalidPhoneInputAndRepositoryFailure(t *testing.T) {
	service := application.NewTeacherService(&teacherRepo{})
	if _, err := service.SetPhone(context.Background(), 0, "+380501234567"); err == nil {
		t.Fatal("invalid teacher ID was accepted")
	}
	if _, err := service.SetPhone(context.Background(), 1, ""); err == nil {
		t.Fatal("empty phone was accepted")
	}
	if _, err := application.NewTeacherService(&teacherRepo{err: errors.New("lookup failed")}).SetPhone(context.Background(), 1, "+380501234567"); err == nil || !strings.Contains(err.Error(), "find teacher") {
		t.Fatalf("lookup error = %v", err)
	}
	if _, err := application.NewTeacherService(&teacherRepo{value: teacher.Recipient{ID: 1}, channelErr: errors.New("write failed")}).SetPhone(context.Background(), 1, "+380501234567"); err == nil || !strings.Contains(err.Error(), "set teacher phone") {
		t.Fatalf("repository error = %v", err)
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
