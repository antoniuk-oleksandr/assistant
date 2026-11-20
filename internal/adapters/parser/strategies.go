package parser

import (
	"context"
	"mime"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/alex/assistent/internal/domain/ports"
	"github.com/alex/assistent/internal/domain/submission"
)

// Strategies gives scenario-specific interpretation precedence over source and
// format defaults. Workspace strategies consume the existing Drive exports.
func Strategies() []ports.DocumentParser {
	word, excel := DOCX{}, XLSX{}
	docs := workspaceExport{DocumentParser: word, kind: "application/vnd.google-apps.document", name: "google-docs"}
	sheets := workspaceExport{DocumentParser: excel, kind: "application/vnd.google-apps.spreadsheet", name: "google-sheets"}
	formats := []ports.DocumentParser{docs, sheets, word, excel}
	strategies := make([]ports.DocumentParser, 0, 2*len(formats))
	for _, format := range formats {
		strategies = append(strategies, scienceAppendix{DocumentParser: format})
	}
	return append(strategies, formats...)
}

type workspaceExport struct {
	ports.DocumentParser
	kind, name string
}

func (p workspaceExport) Type() string { return p.name }

func (p workspaceExport) CanHandle(item submission.Material, _ submission.ParseContext) bool {
	kind, _, err := mime.ParseMediaType(strings.TrimSpace(item.MIMEType))
	return err == nil && kind == p.kind
}

type scienceAppendix struct{ ports.DocumentParser }

func (p scienceAppendix) Type() string { return "science-report-appendix/" + p.DocumentParser.Type() }

func (p scienceAppendix) CanHandle(item submission.Material, origin submission.ParseContext) bool {
	return origin.ScenarioType == "ScienceReportScenario" && appendixNumber(item) != 0 && p.DocumentParser.CanHandle(item, origin)
}

func (p scienceAppendix) Parse(ctx context.Context, item submission.Material, origin submission.ParseContext) (submission.ParsedDocument, error) {
	doc, err := p.DocumentParser.Parse(ctx, item, origin)
	if err == nil {
		number := appendixNumber(item)
		doc.Source.AppendixNumber = &number
	}
	return doc, err
}

// Infer only the documented, unambiguous appendix-NN names. Compound names
// and free-form titles are left intact and go through the general extractor.
func appendixNumber(item submission.Material) int {
	if item.AppendixNumber != nil {
		return validAppendix(*item.AppendixNumber)
	}
	name := strings.ToLower(strings.TrimSpace(item.Name))
	if ext := filepath.Ext(name); ext == ".docx" || ext == ".xlsx" {
		name = strings.TrimSuffix(name, ext)
	}
	const prefix = "appendix-"
	if !strings.HasPrefix(name, prefix) || len(name) != len(prefix)+2 {
		return 0
	}
	suffix := name[len(prefix):]
	if suffix[0] < '0' || suffix[0] > '9' || suffix[1] < '0' || suffix[1] > '9' {
		return 0
	}
	number, _ := strconv.Atoi(suffix)
	return validAppendix(number)
}

func validAppendix(number int) int {
	if number >= 1 && number <= 28 {
		return number
	}
	return 0
}
