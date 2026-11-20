package ports

import (
	"context"

	"github.com/alex/assistent/internal/domain/submission"
)

// MaterialAnalyzer classifies a material that no registered parser handles.
// Validation and aggregation eligibility remain application rules.
type MaterialAnalyzer interface {
	Analyze(context.Context, submission.Material, submission.ParseContext) (submission.ExtractionProposal, error)
}
