package application_test

import (
	"context"
	"errors"
	"github.com/alex/assistent/internal/application"
	"github.com/alex/assistent/internal/domain/ports"
	"github.com/alex/assistent/internal/domain/task"
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
	if r.channelErr != nil {
		return nil, r.channelErr
	}
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

func TestTeacherServiceListsPersistedRecipients(t *testing.T) {
	repo := &teacherRepo{value: teacher.Recipient{ID: 1, Name: "Teacher"}}
	items, err := application.NewTeacherService(repo).List(context.Background())
	if err != nil || len(items) != 1 || items[0].Name != "Teacher" {
		t.Fatalf("items=%#v err=%v", items, err)
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

type participantSourceStub struct {
	participants map[string][]task.Participant
	courses      []string
	err          error
}

func (s *participantSourceStub) ListParticipants(_ context.Context, courseID string) ([]task.Participant, error) {
	s.courses = append(s.courses, courseID)
	if s.err != nil {
		return nil, s.err
	}
	return s.participants[courseID], nil
}

func TestTeacherServiceListsOnlyRequestedCourse(t *testing.T) {
	repo := &teacherRepo{
		value: teacher.Recipient{ID: 4, Name: "Teacher", Email: "teacher@example.com", ClassroomID: "user-1"},
	}
	people := &participantSourceStub{participants: map[string][]task.Participant{
		"123": {{ExternalID: "user-1", Name: "Teacher", Email: "teacher@example.com"}},
		"456": {{ExternalID: "user-1", Name: "Teacher", Email: "teacher@example.com"}},
	}}
	service := application.NewTeacherServiceWithDirectory(repo, people)

	first, err := service.ListForCourse(context.Background(), "123")
	if err != nil || len(first) != 1 {
		t.Fatalf("first course entries=%#v err=%v", first, err)
	}
	second, err := service.ListForCourse(context.Background(), "456")
	if err != nil || len(second) != 1 || second[0].Recipient.ID != 4 {
		t.Fatalf("second course entries=%#v err=%v", second, err)
	}
	if len(people.courses) != 2 || people.courses[0] != "123" || people.courses[1] != "456" {
		t.Fatalf("requested courses=%v", people.courses)
	}
}

func TestTeacherServiceListForCoursePropagatesDirectoryErrors(t *testing.T) {
	ctx := context.Background()
	if _, err := application.NewTeacherService(&teacherRepo{}).ListForCourse(ctx, "123"); err == nil {
		t.Fatal("missing participant source was accepted")
	}
	if _, err := application.NewTeacherServiceWithDirectory(&teacherRepo{}, &participantSourceStub{err: errors.New("Classroom unavailable")}).ListForCourse(ctx, "123"); err == nil || !strings.Contains(err.Error(), "list teachers for course") {
		t.Fatalf("participant error = %v", err)
	}
	if _, err := application.NewTeacherServiceWithDirectory(&teacherRepo{}, &participantSourceStub{}).ListForCourse(ctx, "not-a-course"); err == nil {
		t.Fatal("invalid course reference was accepted")
	}
	if _, err := application.NewTeacherServiceWithDirectory(&teacherRepo{listErr: errors.New("database unavailable")}, &participantSourceStub{participants: map[string][]task.Participant{"123": {{ExternalID: "user-1"}}}}).ListForCourse(ctx, "123"); err == nil || !strings.Contains(err.Error(), "list persisted teachers") {
		t.Fatalf("repository list error = %v", err)
	}
}

func TestTeacherServiceListForCoursePropagatesSynchronizationErrors(t *testing.T) {
	ctx := context.Background()
	participants := &participantSourceStub{participants: map[string][]task.Participant{"123": {{ExternalID: "user-1", Name: "New", Email: "new@example.com"}}}}
	if _, err := application.NewTeacherServiceWithDirectory(&teacherRepo{createErr: errors.New("create failed")}, participants).ListForCourse(ctx, "123"); err == nil || !strings.Contains(err.Error(), "store Classroom teacher") {
		t.Fatalf("create error = %v", err)
	}
	repo := &teacherRepo{value: teacher.Recipient{ID: 1, ClassroomID: "user-1", Name: "Old", Email: "old@example.com"}, updateErr: errors.New("update failed")}
	if _, err := application.NewTeacherServiceWithDirectory(repo, participants).ListForCourse(ctx, "123"); err == nil || !strings.Contains(err.Error(), "update Classroom teacher") {
		t.Fatalf("update error = %v", err)
	}
}

func TestTeacherServiceListForCourseUpdatesChangedRecipient(t *testing.T) {
	repo := &teacherRepo{value: teacher.Recipient{ID: 1, ClassroomID: "user-1", Name: "Old", Email: "old@example.com"}}
	people := &participantSourceStub{participants: map[string][]task.Participant{"123": {{ExternalID: "user-1", Name: "New", Email: "new@example.com"}}}}
	entries, err := application.NewTeacherServiceWithDirectory(repo, people).ListForCourse(context.Background(), "123")
	if err != nil || len(entries) != 1 || entries[0].Recipient.Name != "New" || repo.value.Email != "new@example.com" {
		t.Fatalf("entries=%#v repository=%#v err=%v", entries, repo.value, err)
	}
}

var _ ports.TeacherRepository = (*teacherRepo)(nil)
