package task

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Task phase determination", func() {
	It("uses due date, time, and timezone", func() {
		item := Task{DueDate: "2026-11-10", DueTime: "12:00:00", TimeZone: "Europe/Kyiv"}
		tests := []struct {
			name string
			now  time.Time
			want Phase
		}{
			{"discovery", time.Date(2026, 11, 7, 9, 59, 0, 0, time.UTC), PhaseDiscovery},
			{"three days before", time.Date(2026, 11, 7, 10, 0, 0, 0, time.UTC), PhaseThreeDaysBefore},
			{"one day before", time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC), PhaseOneDayBefore},
			{"deadline", time.Date(2026, 11, 10, 10, 0, 0, 0, time.UTC), PhaseDeadlineReached},
			{"completed", time.Date(2026, 11, 10, 10, 1, 0, 0, time.UTC), PhaseCompleted},
		}
		for _, test := range tests {
			Expect(DeterminePhase(item, test.now)).To(Equal(test.want))
		}
	})

	It("falls back to DueAt and discovery", func() {
		now := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
		Expect(DeterminePhase(Task{DueAt: "2026-11-10T12:00:00Z"}, now)).To(Equal(PhaseDeadlineReached))
		Expect(DeterminePhase(Task{DueAt: "invalid"}, now)).To(Equal(PhaseDiscovery))
	})

	It("supports custom minute windows", func() {
		deadline := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
		item := Task{DueAt: deadline.Format(time.RFC3339)}
		windows := DeadlineWindows{PreliminaryBefore: 5 * time.Minute, RefreshBefore: 2 * time.Minute}
		checks := []struct {
			now  time.Time
			want Phase
		}{
			{deadline.Add(-6 * time.Minute), PhaseDiscovery},
			{deadline.Add(-5 * time.Minute), PhaseThreeDaysBefore},
			{deadline.Add(-2 * time.Minute), PhaseOneDayBefore},
			{deadline, PhaseDeadlineReached},
		}
		for _, check := range checks {
			Expect(DeterminePhaseWithWindows(item, check.now, windows)).To(Equal(check.want))
		}
		Expect(DeterminePhaseWithWindows(item, deadline.Add(-72*time.Hour), DeadlineWindows{})).To(Equal(PhaseThreeDaysBefore))
	})
})
