package task

import (
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestTaskDomain(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Task domain")
}

var _ = Describe("Task deadlines", func() {
	It("supports date-time and RFC3339 forms", func() {
		cases := []Task{
			{DueDate: "2026-11-21", DueTime: "12:30:00"},
			{DueDate: "2026-11-21"},
			{DueAt: "2026-11-21T12:30:00Z"},
			{DueAt: "2026-11-21T12:30:00"},
		}
		for _, item := range cases {
			value, ok := Deadline(item)
			Expect(ok).To(BeTrue())
			Expect(value).NotTo(BeZero())
		}
		_, ok := Deadline(Task{DueDate: "bad"})
		Expect(ok).To(BeFalse())
		_, ok = Deadline(Task{DueAt: "bad"})
		Expect(ok).To(BeFalse())
	})

	It("uses configured timezone and phase boundaries", func() {
		item := Task{DueDate: "2026-11-21", DueTime: "14:00:00", TimeZone: "Europe/Kyiv"}
		deadline, ok := Deadline(item)
		Expect(ok).To(BeTrue())
		Expect(deadline.Location().String()).To(Equal("Europe/Kyiv"))
		windows := DeadlineWindows{PreliminaryBefore: 2 * time.Minute, RefreshBefore: time.Minute}
		for _, test := range []struct {
			name string
			now  time.Time
			want Phase
		}{
			{"before", deadline.Add(-3 * time.Minute), PhaseDiscovery},
			{"preliminary", deadline.Add(-90 * time.Second), PhaseThreeDaysBefore},
			{"refresh", deadline.Add(-30 * time.Second), PhaseOneDayBefore},
			{"at deadline", deadline, PhaseDeadlineReached},
			{"after", deadline.Add(time.Second), PhaseCompleted},
		} {
			Expect(DeterminePhaseWithWindows(item, test.now, windows)).To(Equal(test.want))
		}
		Expect(DeterminePhaseWithWindows(Task{}, deadline, windows)).To(Equal(PhaseDiscovery))
	})
})
