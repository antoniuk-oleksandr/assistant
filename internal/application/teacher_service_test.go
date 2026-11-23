package application_test

import (
	"context"
	"errors"
	"github.com/alex/assistent/internal/application"
	"github.com/alex/assistent/internal/domain/ports"
	"github.com/alex/assistent/internal/domain/task"
	"github.com/alex/assistent/internal/domain/teacher"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
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

var _ = Describe("teacher service", func() {
	It("manages recipients and channels", func() {
		repo := &teacherRepo{}
		service := application.NewTeacherService(repo)
		rec, err := service.Add(context.Background(), "Teacher", "teacher@example.com", "classroom-1")
		Expect(err).NotTo(HaveOccurred())
		Expect(rec.Status).To(Equal(teacher.Active))
		Expect(service.SetStatus(context.Background(), rec.ID, teacher.Disabled)).To(Succeed())
		channel, err := service.AddChannel(context.Background(), rec.ID, "email", "teacher@example.com")
		Expect(err).NotTo(HaveOccurred())
		Expect(channel.Status).To(Equal(teacher.Active))
		Expect(service.SetChannelStatus(context.Background(), channel, teacher.Disabled)).To(Succeed())
		channels, err := service.ActiveChannels(context.Background(), rec.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(channels).To(BeEmpty())
	})

	It("lists persisted recipients", func() {
		repo := &teacherRepo{value: teacher.Recipient{ID: 1, Name: "Teacher"}}
		items, err := application.NewTeacherService(repo).List(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(items).To(HaveLen(1))
		Expect(items[0].Name).To(Equal("Teacher"))
	})

	It("propagates repository errors", func() {
		service := application.NewTeacherService(&teacherRepo{err: errors.New("write failed")})
		_, err := service.Add(context.Background(), "x", "x@example.com", "")
		Expect(err).To(HaveOccurred())
		_, err = service.AddChannel(context.Background(), 1, "email", "x")
		Expect(err).To(HaveOccurred())
		Expect(service.SetStatus(context.Background(), 1, teacher.Disabled)).To(HaveOccurred())
		_, err = application.NewTeacherService(&teacherRepo{}).AddChannel(context.Background(), 1, "unsupported", "x")
		Expect(err).To(HaveOccurred())
	})

	It("lists only the requested course", func() {
		repo := &teacherRepo{
			value: teacher.Recipient{ID: 4, Name: "Teacher", Email: "teacher@example.com", ClassroomID: "user-1"},
		}
		people := &participantSourceStub{participants: map[string][]task.Participant{
			"123": {{ExternalID: "user-1", Name: "Teacher", Email: "teacher@example.com"}},
			"456": {{ExternalID: "user-1", Name: "Teacher", Email: "teacher@example.com"}},
		}}
		service := application.NewTeacherServiceWithDirectory(repo, people)

		first, err := service.ListForCourse(context.Background(), "123")
		Expect(err).NotTo(HaveOccurred())
		Expect(first).To(HaveLen(1))
		second, err := service.ListForCourse(context.Background(), "456")
		Expect(err).NotTo(HaveOccurred())
		Expect(second).To(HaveLen(1))
		Expect(second[0].Recipient.ID).To(Equal(int64(4)))
		Expect(people.courses).To(Equal([]string{"123", "456"}))
	})

	It("deduplicates participants within a course", func() {
		repo := &teacherRepo{value: teacher.Recipient{ID: 4, Name: "Teacher", ClassroomID: "user-1"}}
		people := &participantSourceStub{participants: map[string][]task.Participant{
			"123": {{ExternalID: "user-1", Name: "Teacher"}, {ExternalID: "user-1", Name: "Teacher"}},
		}}
		items, err := application.NewTeacherServiceWithDirectory(repo, people).ListForCourse(context.Background(), "123")
		Expect(err).NotTo(HaveOccurred())
		Expect(items).To(HaveLen(1))
	})

	It("propagates directory errors", func() {
		ctx := context.Background()
		_, err := application.NewTeacherService(&teacherRepo{}).ListForCourse(ctx, "123")
		Expect(err).To(HaveOccurred())
		_, err = application.NewTeacherServiceWithDirectory(&teacherRepo{}, &participantSourceStub{err: errors.New("Classroom unavailable")}).ListForCourse(ctx, "123")
		Expect(err).To(MatchError(ContainSubstring("list teachers for course")))
		_, err = application.NewTeacherServiceWithDirectory(&teacherRepo{}, &participantSourceStub{}).ListForCourse(ctx, "not-a-course")
		Expect(err).To(HaveOccurred())
		_, err = application.NewTeacherServiceWithDirectory(&teacherRepo{listErr: errors.New("database unavailable")}, &participantSourceStub{participants: map[string][]task.Participant{"123": {{ExternalID: "user-1"}}}}).ListForCourse(ctx, "123")
		Expect(err).To(MatchError(ContainSubstring("list persisted teachers")))
	})

	It("propagates synchronization errors", func() {
		ctx := context.Background()
		participants := &participantSourceStub{participants: map[string][]task.Participant{"123": {{ExternalID: "user-1", Name: "New", Email: "new@example.com"}}}}
		_, err := application.NewTeacherServiceWithDirectory(&teacherRepo{createErr: errors.New("create failed")}, participants).ListForCourse(ctx, "123")
		Expect(err).To(MatchError(ContainSubstring("store Classroom teacher")))
		repo := &teacherRepo{value: teacher.Recipient{ID: 1, ClassroomID: "user-1", Name: "Old", Email: "old@example.com"}, updateErr: errors.New("update failed")}
		_, err = application.NewTeacherServiceWithDirectory(repo, participants).ListForCourse(ctx, "123")
		Expect(err).To(MatchError(ContainSubstring("update Classroom teacher")))
	})

	It("updates a changed recipient", func() {
		repo := &teacherRepo{value: teacher.Recipient{ID: 1, ClassroomID: "user-1", Name: "Old", Email: "old@example.com"}}
		people := &participantSourceStub{participants: map[string][]task.Participant{"123": {{ExternalID: "user-1", Name: "New", Email: "new@example.com"}}}}
		entries, err := application.NewTeacherServiceWithDirectory(repo, people).ListForCourse(context.Background(), "123")
		Expect(err).NotTo(HaveOccurred())
		Expect(entries).To(HaveLen(1))
		Expect(entries[0].Recipient.Name).To(Equal("New"))
		Expect(repo.value.Email).To(Equal("new@example.com"))
	})
})

var _ ports.TeacherRepository = (*teacherRepo)(nil)
