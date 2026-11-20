package parser

import (
	"context"
	"io"
	"mime"
	"os"
	"path/filepath"
	"strings"

	"github.com/alex/assistent/internal/domain/submission"
)

// MaterialAnalyzer provides a provider-neutral fallback for materials that no
// parser can handle. It makes only deterministic classifications from material
// metadata and, for explicitly text-like local files, bounded content signals.
// The proposal is intentionally auditable and remains below the aggregation
// threshold when the evidence is not strong enough to support a fact.
type MaterialAnalyzer struct{}

func NewMaterialAnalyzer() MaterialAnalyzer { return MaterialAnalyzer{} }

func (MaterialAnalyzer) Analyze(ctx context.Context, item submission.Material, origin submission.ParseContext) (submission.ExtractionProposal, error) {
	if err := ctx.Err(); err != nil {
		return submission.ExtractionProposal{}, err
	}

	content, contentAvailable, err := localTextSignals(ctx, item)
	if err != nil {
		return submission.ExtractionProposal{}, err
	}
	if item.AppendixNumber == nil {
		if number := appendixNumber(item); number != 0 {
			item.AppendixNumber = &number
		}
	}
	documentType, meaning, evidence := classify(item, origin, content)

	fields := map[string]any{
		"name":                 item.Name,
		"mime_type":            item.MIMEType,
		"source_uri":           item.SourceURI,
		"role":                 item.Role,
		"scenario_id":          origin.ScenarioID,
		"scenario_name":        origin.ScenarioName,
		"scenario_type":        origin.ScenarioType,
		"task_external_id":     origin.TaskExternalID,
		"submission_id":        origin.SubmissionExternalID,
		"document_meaning":     meaning,
		"classification_basis": evidence,
		"content_available":    contentAvailable,
	}
	if item.LocalPath != "" {
		fields["local_path"] = item.LocalPath
	}
	if item.AppendixNumber != nil {
		fields["appendix"] = *item.AppendixNumber
	}

	return submission.ExtractionProposal{
		SchemaVersion:        submission.AnalysisSchemaVersion,
		Appendix:             item.AppendixNumber,
		DocumentType:         documentType,
		Fields:               fields,
		IsSupportingEvidence: item.Role != "appendix",
		Confidence:           0.25,
	}, nil
}

func classify(item submission.Material, origin submission.ParseContext, content string) (string, string, []string) {
	name := strings.ToLower(item.Name + " " + filepath.Base(item.LocalPath))
	text := strings.ToLower(name + " " + content)
	evidence := make([]string, 0, 4)
	scenario := strings.ToLower(strings.TrimSpace(origin.ScenarioType + " " + origin.ScenarioName))
	isScience := strings.Contains(scenario, "science") || strings.Contains(scenario, "scientific")

	if item.AppendixNumber != nil && *item.AppendixNumber >= 1 && *item.AppendixNumber <= 28 {
		evidence = append(evidence, "material appendix number")
		if isScience {
			return "scientific_report_appendix", "candidate input for the scientific report appendix set", evidence
		}
		return "scenario_appendix", "candidate appendix input for the bound scenario", evidence
	}
	if strings.Contains(name, "appendix-") {
		evidence = append(evidence, "appendix filename")
	}
	for _, candidate := range []struct {
		kind, meaning, signal string
	}{
		{"certificate", "supporting certification or award evidence", "certificate"},
		{"publication", "supporting publication or scholarly output evidence", "publication"},
		{"patent", "supporting patent or invention evidence", "patent"},
	} {
		if strings.Contains(text, candidate.signal) {
			evidence = append(evidence, candidate.signal+" name/content signal")
			return candidate.kind, candidate.meaning, evidence
		}
	}
	if strings.Contains(strings.ToLower(item.Role), "support") {
		evidence = append(evidence, "supporting evidence role")
		return "supporting_evidence", "supporting evidence for the bound scenario", evidence
	}
	if isScience {
		evidence = append(evidence, "scientific report scenario context")
		return "scientific_report_material", "unclassified material associated with the scientific report scenario", evidence
	}
	if origin.ScenarioType != "" || origin.ScenarioName != "" {
		evidence = append(evidence, "bound scenario context")
		return "scenario_material", "unclassified material associated with the bound scenario", evidence
	}
	evidence = append(evidence, "material metadata only")
	return formatKind(item), "unclassified material with no reliable scenario-specific meaning", evidence
}

func formatKind(item submission.Material) string {
	kind, _, err := mime.ParseMediaType(strings.TrimSpace(item.MIMEType))
	if err == nil {
		switch {
		case strings.HasPrefix(kind, "image/"):
			return "image"
		case strings.HasPrefix(kind, "text/"):
			return "text_document"
		case kind == "application/pdf":
			return "pdf_document"
		case strings.Contains(kind, "spreadsheet"):
			return "spreadsheet"
		case strings.Contains(kind, "word") || strings.Contains(kind, "document"):
			return "word_document"
		}
	}
	if ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(item.Name)), "."); ext != "" {
		return ext + "_document"
	}
	return "unknown_material"
}

func localTextSignals(ctx context.Context, item submission.Material) (string, bool, error) {
	if item.LocalPath == "" || !textLike(item) {
		return "", false, nil
	}
	file, err := os.Open(item.LocalPath)
	if err != nil {
		if os.IsNotExist(err) || os.IsPermission(err) {
			return "", false, nil
		}
		return "", false, err
	}
	defer file.Close()
	const maxSignalBytes = 64 * 1024
	data, err := io.ReadAll(io.LimitReader(file, maxSignalBytes))
	if err != nil {
		return "", false, err
	}
	if err := ctx.Err(); err != nil {
		return "", false, err
	}
	return string(data), len(data) > 0, nil
}

func textLike(item submission.Material) bool {
	kind, _, err := mime.ParseMediaType(strings.TrimSpace(item.MIMEType))
	if err == nil && strings.HasPrefix(kind, "text/") {
		return true
	}
	text := strings.ToLower(filepath.Ext(item.Name))
	return text == ".txt" || text == ".md" || text == ".csv" || text == ".json"
}
