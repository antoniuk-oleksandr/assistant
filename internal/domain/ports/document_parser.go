package ports

import (
	"context"

	"github.com/alex/assistent/internal/domain/submission"
)

// DocumentParser reads a downloaded snapshot. CanHandle only inspects metadata
// and submission context; it must not perform I/O.
type DocumentParser interface {
	Type() string
	CanHandle(submission.Material, submission.ParseContext) bool
	Parse(context.Context, submission.Material, submission.ParseContext) (submission.ParsedDocument, error)
}
