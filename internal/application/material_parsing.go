package application

import (
	"context"

	"github.com/alex/assistent/internal/domain/ports"
	"github.com/alex/assistent/internal/domain/submission"
)

// MaterialParsing selects the first matching parser in registration order.
// Register context-specific strategies ahead of general format parsers.
type MaterialParsing struct {
	parsers []ports.DocumentParser
}

func NewMaterialParsing(parsers ...ports.DocumentParser) *MaterialParsing {
	return &MaterialParsing{parsers: append([]ports.DocumentParser(nil), parsers...)}
}

// Parse returns a per-material outcome, not a polling error, for unsupported
// formats or parser errors. Cancellation still aborts the enclosing operation.
func (s *MaterialParsing) Parse(ctx context.Context, item submission.Material, origin submission.ParseContext) (submission.ParseResult, error) {
	if err := ctx.Err(); err != nil {
		return submission.ParseResult{}, err
	}
	for _, parser := range s.parsers {
		if !parser.CanHandle(item, origin) {
			continue
		}
		result := submission.ParseResult{ParserType: parser.Type()}
		document, err := parser.Parse(ctx, item, origin)
		if ctx.Err() != nil {
			return submission.ParseResult{}, ctx.Err()
		}
		if err != nil {
			result.Issue = submission.ParseFailed
			return result, nil
		}
		source := item.DocumentSource(origin)
		if source.AppendixNumber == nil && document.Source.AppendixNumber != nil {
			source.AppendixNumber = document.Source.AppendixNumber
			source.Role = "appendix"
		}
		document.Source = source
		result.Document = &document
		return result, nil
	}
	return submission.ParseResult{Issue: submission.UnsupportedMaterial}, nil
}
