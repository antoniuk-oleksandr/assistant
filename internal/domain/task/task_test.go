package task

import (
	"testing"
	"time"
)

func TestDeterminePhaseUsesDueDateTimeAndTimezone(t *testing.T) {
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
		t.Run(test.name, func(t *testing.T) {
			if got := DeterminePhase(item, test.now); got != test.want {
				t.Fatalf("DeterminePhase() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestDeterminePhaseFallsBackToDueAtAndDiscovery(t *testing.T) {
	now := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	if got := DeterminePhase(Task{DueAt: "2026-11-10T12:00:00Z"}, now); got != PhaseDeadlineReached {
		t.Fatalf("RFC3339 phase = %q", got)
	}
	if got := DeterminePhase(Task{DueAt: "invalid"}, now); got != PhaseDiscovery {
		t.Fatalf("invalid deadline phase = %q", got)
	}
}
