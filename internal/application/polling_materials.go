package application

import (
	"context"

	"github.com/alex/assistent/internal/domain/submission"
	"github.com/alex/assistent/internal/domain/task"
)

func (s *PollingService) processMaterials(ctx context.Context, binding task.Binding, current submission.Submission) ([]submission.Material, error) {
	origin := submission.ParseContext{
		ScenarioID:   binding.ScenarioID,
		ScenarioName: binding.ScenarioName, ScenarioType: binding.ScenarioType,
		CourseID: binding.Task.CourseID, TaskExternalID: binding.Task.ExternalID,
		SubmissionExternalID: current.ExternalID, SubmittedAt: current.SubmittedAt,
		SubmitterID: current.SubmitterID, SubmitterName: current.SubmitterName, SubmitterEmail: current.SubmitterEmail,
	}
	// Do not mutate the source's slice (which it may reuse on the next poll).
	materials := append([]submission.Material(nil), current.Materials...)
	for i, item := range materials {
		item = s.materials.Retrieve(ctx, item)
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if s.parsing != nil && item.Status == submission.Pending && item.LocalPath != "" {
			result, err := s.parsing.Parse(ctx, item, origin)
			if err != nil {
				return nil, err
			}
			item.Parsing = &result
			if origin.ScenarioID == 0 {
				switch {
				case result.Document != nil:
					item.Status = submission.Parsed
				case result.Issue == submission.ParseFailed:
					item.Status = submission.Failed
				}
			}
			// An unsupported format can still be usable evidence. Keep its
			// retrieval status and record the interpretation outcome separately.
		}
		if origin.ScenarioID != 0 && item.Parsing != nil {
			item.ParseOutcomes = []submission.MaterialParse{{Source: item.DocumentSource(origin), Result: *item.Parsing}}
			item.Parsing = nil
		}
		materials[i] = item
	}
	return materials, nil
}
