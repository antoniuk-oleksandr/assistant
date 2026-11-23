package application

import (
	"context"
	"fmt"
	"time"

	"github.com/alex/assistent/internal/domain/ports"
	"github.com/alex/assistent/internal/domain/task"
	"go.uber.org/zap"
)

type deadlineEvent struct {
	binding task.Binding
	phase   task.Phase
	at      time.Time
}

const schedulerBindingRefreshInterval = 5 * time.Second

// RunScheduled waits for persisted report deadlines and processes only the
// next due lifecycle event. It does not poll Classroom or generate a report at
// application startup.
func (s *PollingService) RunScheduled(ctx context.Context) error {
	for {
		bindings, err := s.bindings.Active(ctx)
		if err != nil {
			return fmt.Errorf("list active task bindings for scheduler: %w", err)
		}
		now := s.currentTime()
		if err := s.disableExpiredBindings(ctx, bindings, now); err != nil {
			return err
		}
		event, found := nextDeadlineEvent(bindings, now, s.deadlines)
		if !found {
			if err := waitForScheduler(ctx, schedulerBindingRefreshInterval); err != nil {
				return nil
			}
			continue
		}
		if !event.at.After(now) {
			if err := s.executeDeadlineEvent(ctx, event); err != nil {
				if ctx.Err() != nil {
					return nil
				}
				s.log().Error("scheduled report event failed", zap.String("task_id", event.binding.Task.ExternalID), zap.Error(err))
				if err := waitForScheduler(ctx, schedulerBindingRefreshInterval); err != nil {
					return nil
				}
			}
			continue
		}
		wait := event.at.Sub(now)
		if wait > schedulerBindingRefreshInterval {
			wait = schedulerBindingRefreshInterval
		}
		if err := waitForScheduler(ctx, wait); err != nil {
			return nil
		}
	}
}

func waitForScheduler(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (s *PollingService) disableExpiredBindings(ctx context.Context, bindings []task.Binding, now time.Time) error {
	for _, binding := range bindings {
		deadline, ok := task.Deadline(binding.Task)
		if !ok || !deadline.Before(now) {
			continue
		}
		if err := s.bindings.Unbind(ctx, binding.Task.ExternalID); err != nil {
			return fmt.Errorf("disable expired report task %q: %w", binding.Task.ExternalID, err)
		}
	}
	return nil
}

func (s *PollingService) currentTime() time.Time {
	if s.clock != nil {
		return s.clock().UTC()
	}
	return time.Now().UTC()
}

func nextDeadlineEvent(bindings []task.Binding, now time.Time, windows task.DeadlineWindows) (deadlineEvent, bool) {
	var next deadlineEvent
	found := false
	for _, binding := range bindings {
		deadline, ok := task.Deadline(binding.Task)
		if !ok {
			continue
		}
		if deadline.Before(now) {
			continue
		}
		phase, at, ok := nextEventForTask(binding.Task.Phase, deadline, windows)
		if !ok {
			continue
		}
		candidate := deadlineEvent{binding: binding, phase: phase, at: at}
		if !found || candidate.at.Before(next.at) {
			next, found = candidate, true
		}
	}
	return next, found
}

func nextEventForTask(current task.Phase, deadline time.Time, windows task.DeadlineWindows) (task.Phase, time.Time, bool) {
	if windows.PreliminaryBefore <= 0 {
		windows.PreliminaryBefore = task.DefaultDeadlineWindows.PreliminaryBefore
	}
	if windows.RefreshBefore <= 0 || windows.RefreshBefore > windows.PreliminaryBefore {
		windows.RefreshBefore = task.DefaultDeadlineWindows.RefreshBefore
	}
	switch current {
	case "", task.PhaseDiscovery:
		return task.PhaseThreeDaysBefore, deadline.Add(-windows.PreliminaryBefore), true
	case task.PhaseThreeDaysBefore:
		return task.PhaseOneDayBefore, deadline.Add(-windows.RefreshBefore), true
	case task.PhaseOneDayBefore:
		return task.PhaseDeadlineReached, deadline, true
	default:
		return "", time.Time{}, false
	}
}

func (s *PollingService) executeDeadlineEvent(ctx context.Context, event deadlineEvent) error {
	if event.phase == task.PhaseDeadlineReached {
		if err := s.bindings.Unbind(ctx, event.binding.Task.ExternalID); err != nil {
			return fmt.Errorf("disable report task at deadline %q: %w", event.binding.Task.ExternalID, err)
		}
		return nil
	}
	event.binding.Task.Phase = event.phase
	event.binding.Task.PhaseAt = s.currentTime().Format(time.RFC3339Nano)
	if registrar, ok := s.source.(ports.TaskCourseRegistrar); ok {
		registrar.RegisterTaskCourse(event.binding.Task.ExternalID, event.binding.Task.CourseID)
	}
	if err := s.refreshParticipantRoster(ctx, event.binding.Task); err != nil {
		return err
	}
	if err := s.refreshTask(ctx, event.binding); err != nil {
		return fmt.Errorf("process scheduled report event for task %q: %w", event.binding.Task.ExternalID, err)
	}
	if err := s.tasks.Register(ctx, event.binding.Task); err != nil {
		return fmt.Errorf("persist scheduled report phase for task %q: %w", event.binding.Task.ExternalID, err)
	}
	return nil
}

func (s *PollingService) refreshParticipantRoster(ctx context.Context, item task.Task) error {
	if s.participants == nil {
		return nil
	}
	source, ok := s.source.(ports.ParticipantSource)
	if !ok {
		return fmt.Errorf("load participants for task %q: Classroom participant source is unavailable", item.ExternalID)
	}
	roster, err := source.ListParticipants(ctx, item.CourseID)
	if err != nil {
		return fmt.Errorf("load participants for task %q: %w", item.ExternalID, err)
	}
	if err := s.participants.Replace(ctx, item.ID, roster); err != nil {
		return fmt.Errorf("persist participants for task %q: %w", item.ExternalID, err)
	}
	return nil
}
