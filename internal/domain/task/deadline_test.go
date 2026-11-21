package task

import (
	"testing"
	"time"
)

func TestDeadlineSupportsDateTimeAndRFC3339Forms(t *testing.T) {
	cases := []Task{
		{DueDate: "2026-11-21", DueTime: "12:30:00"},
		{DueDate: "2026-11-21"},
		{DueAt: "2026-11-21T12:30:00Z"},
		{DueAt: "2026-11-21T12:30:00"},
	}
	for _, item := range cases {
		if value, ok := Deadline(item); !ok || value.IsZero() {
			t.Fatalf("Deadline(%#v) = %s, %t", item, value, ok)
		}
	}
	if _, ok := Deadline(Task{DueDate: "bad"}); ok {
		t.Fatal("invalid date unexpectedly parsed")
	}
	if _, ok := Deadline(Task{DueAt: "bad"}); ok {
		t.Fatal("invalid deadline unexpectedly parsed")
	}
}

func TestDeadlineUsesConfiguredTimezoneAndPhaseBoundaries(t *testing.T) {
	item := Task{DueDate: "2026-11-21", DueTime: "14:00:00", TimeZone: "Europe/Kyiv"}
	deadline, ok := Deadline(item)
	if !ok || deadline.Location().String() != "Europe/Kyiv" {
		t.Fatalf("deadline = %s, %t", deadline, ok)
	}
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
		if got := DeterminePhaseWithWindows(item, test.now, windows); got != test.want {
			t.Errorf("%s: phase = %q, want %q", test.name, got, test.want)
		}
	}
	if got := DeterminePhaseWithWindows(Task{}, deadline, windows); got != PhaseDiscovery {
		t.Fatalf("missing deadline phase = %q", got)
	}
}
