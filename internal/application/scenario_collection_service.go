package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/alex/assistent/internal/domain/ports"
	"github.com/alex/assistent/internal/domain/report"
	"github.com/alex/assistent/internal/domain/scenario"
)

var ErrNilCollectionProcessor = errors.New("nil collection processor")

// ScenarioCollectionService persists the processing lifecycle and delegates
// collection rules to the selected scenario implementation.
type ScenarioCollectionService struct {
	submissions ports.SubmissionRepository
	runs        ports.ReportRunRepository
	renderer    ports.ReportRenderer
	outputs     ports.ReportOutputRepository
	results     ports.ReportResultRepository
	clock       func() time.Time
}

func NewScenarioCollectionService(submissions ports.SubmissionRepository, runs ports.ReportRunRepository) *ScenarioCollectionService {
	return NewScenarioCollectionServiceWithRenderer(submissions, runs, nil, nil)
}

func NewScenarioCollectionServiceWithRenderer(submissions ports.SubmissionRepository, runs ports.ReportRunRepository, renderer ports.ReportRenderer, outputs ports.ReportOutputRepository) *ScenarioCollectionService {
	return NewScenarioCollectionServiceWithRendererAndResult(submissions, runs, renderer, outputs, nil)
}

func NewScenarioCollectionServiceWithRendererAndResult(submissions ports.SubmissionRepository, runs ports.ReportRunRepository, renderer ports.ReportRenderer, outputs ports.ReportOutputRepository, results ports.ReportResultRepository) *ScenarioCollectionService {
	return &ScenarioCollectionService{submissions: submissions, runs: runs, renderer: renderer, outputs: outputs, results: results, clock: func() time.Time { return time.Now().UTC() }}
}

func (s *ScenarioCollectionService) Process(ctx context.Context, scenarioID, taskID int64, processor scenario.CollectionProcessor) (report.Run, error) {
	if existing, found, err := s.existingRun(ctx, scenarioID, taskID); err != nil {
		return report.Run{}, fmt.Errorf("find report run: %w", err)
	} else if found {
		return existing, nil
	}
	run := report.NewRun(scenarioID, taskID, s.clock())
	if err := s.runs.Create(ctx, &run); err != nil {
		return report.Run{}, fmt.Errorf("persist report run: %w", err)
	}
	if processor == nil {
		return run, s.persistFailure(ctx, &run, ErrNilCollectionProcessor)
	}
	if err := run.Transition(report.Collecting, s.clock()); err != nil {
		return run, s.persistFailure(ctx, &run, err)
	}
	if err := s.runs.Update(ctx, &run); err != nil {
		return run, fmt.Errorf("persist collecting report run: %w", err)
	}
	items, err := s.submissions.ListByTask(ctx, taskID)
	if err != nil {
		return run, s.persistFailure(ctx, &run, fmt.Errorf("collect submissions for task %d: %w", taskID, err))
	}
	if err := run.Transition(report.Processing, s.clock()); err != nil {
		return run, s.persistFailure(ctx, &run, err)
	}
	if err := s.runs.Update(ctx, &run); err != nil {
		return run, fmt.Errorf("persist processing report run: %w", err)
	}
	result, processErr := processor.Process(scenario.Collection{ScenarioID: scenarioID, TaskID: taskID, Items: items})
	if processErr != nil {
		result.Outcome = scenario.Failed
		result.ErrorMessage = processErr.Error()
	}
	if result.ReportData != nil && s.renderer != nil {
		var persistedResult *report.Result
		if s.results != nil {
			value := report.NewResult(run.ID, scenarioID, taskID, *result.ReportData, s.clock())
			if err := s.results.Create(ctx, &value); err != nil {
				return run, s.persistFailure(ctx, &run, fmt.Errorf("persist processing result: %w", err))
			}
			persistedResult = &value
		}
		output, err := s.renderer.RenderAndSave(ctx, *result.ReportData)
		if err != nil {
			return run, s.persistFailure(ctx, &run, fmt.Errorf("render report package: %w", err))
		}
		result.ResultPath = output.Path
		if s.outputs != nil {
			if err := s.outputs.Create(ctx, ports.ReportOutput{ReportRunID: run.ID, Format: output.Format, Path: output.Path, Version: output.Version, CreatedAt: output.CreatedAt}); err != nil {
				return run, s.persistFailure(ctx, &run, fmt.Errorf("persist report output: %w", err))
			}
		}
		if persistedResult != nil {
			status := report.ResultCompleted
			if result.Outcome == scenario.Partial {
				status = report.ResultPartial
			}
			if result.Outcome == scenario.Failed {
				status = report.ResultFailed
			}
			persistedResult.Complete(status, s.clock(), result.ErrorMessage)
			if err := s.results.Update(ctx, persistedResult); err != nil {
				return run, s.persistFailure(ctx, &run, fmt.Errorf("persist terminal result: %w", err))
			}
		}
	}
	if err := run.ApplyResult(result, s.clock()); err != nil {
		return run, s.persistFailure(ctx, &run, err)
	}
	if err := s.runs.Update(ctx, &run); err != nil {
		return run, fmt.Errorf("persist terminal report run: %w", err)
	}
	if processErr != nil {
		return run, fmt.Errorf("process scenario collection: %w", processErr)
	}
	return run, nil
}

func (s *ScenarioCollectionService) existingRun(ctx context.Context, scenarioID, taskID int64) (report.Run, bool, error) {
	repository, ok := s.runs.(ports.ScenarioTaskReportRunRepository)
	if !ok {
		return report.Run{}, false, nil
	}
	run, err := repository.GetByScenarioTask(ctx, scenarioID, taskID)
	if errors.Is(err, ports.ErrReportRunNotFound) {
		return report.Run{}, false, nil
	}
	if err != nil {
		return report.Run{}, false, err
	}
	return run, true, nil
}

func (s *ScenarioCollectionService) persistFailure(ctx context.Context, run *report.Run, cause error) error {
	run.ErrorMessage = cause.Error()
	_ = run.Transition(report.Failed, s.clock())
	if err := s.runs.Update(ctx, run); err != nil {
		return fmt.Errorf("%v; persist failed report run: %w", cause, err)
	}
	return cause
}
