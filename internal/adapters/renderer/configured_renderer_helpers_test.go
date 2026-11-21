package renderer

import (
	"path/filepath"
	"testing"

	"github.com/alex/assistent/internal/domain/scenario"
)

func TestConfiguredRendererBuildsTemplateValuesAndSections(t *testing.T) {
	ranges := courseDataRanges(map[string]string{
		"department_name":            "Computer Science",
		"department_email":           "dept@example.com",
		"lab_assistant_name":         "Anna",
		"lab_assistant_mobile_phone": "+380501234567",
		"department_phone":           "+380441234567",
	})
	if len(ranges) != 5 || ranges[0].values[0][0] != "кафедри Computer Science" || ranges[3].values[0][0] != "'+380501234567" {
		t.Fatalf("course data ranges = %#v", ranges)
	}
	if phoneCellValue("0441234567") != "0441234567" || phoneCellValue("") != "" {
		t.Fatal("phone values were changed unexpectedly")
	}

	data := scenario.ReportData{
		Appendices:         [28]scenario.ReportAppendix{{Number: 1, NumericTotal: 2, TextValues: []string{"text"}, Sources: []scenario.ReportSource{{Name: "source"}}}},
		SupportingEvidence: []scenario.ReportSource{{SubmissionID: "submission"}},
		InvalidMaterials:   []scenario.ReportSource{{Reason: "invalid"}},
		MissingSubmissions: []string{"missing"},
		Conflicts:          []scenario.ReportConflict{{Appendix: 1, Values: []string{"a", "b"}}},
	}
	sections := reportSections(data)
	if len(sections) != 5 || len(sections[0].rows) != 29 || len(sections[1].rows) != 2 || len(sections[4].rows) != 2 {
		t.Fatalf("sections = %#v", sections)
	}
	if join([]string{"a", "b"}) != "a; b" || join(nil) != "" {
		t.Fatal("join formatting is incorrect")
	}
}

func TestConfiguredRendererLoadsTemplateLayout(t *testing.T) {
	path, err := filepath.Abs("../../../templates/scientific-report/output_example.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("REPORT_TEMPLATE_PATH", path)
	appendices, slashRows, values := loadTemplateLayout()
	if len(appendices) == 0 || len(slashRows) == 0 || len(values) != 200 {
		t.Fatalf("template layout sizes = %d, %d, %d", len(appendices), len(slashRows), len(values))
	}
	if reportTemplatePath() == "" {
		t.Fatal("template path is empty")
	}
}
