package renderer

import (
	"path/filepath"
	"testing"

	"github.com/alex/assistent/internal/domain/scenario"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestRendererHelpers(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Renderer helpers")
}

var _ = Describe("Configured renderer helpers", func() {
	It("builds template values and sections", func() {
		ranges := courseDataRanges(map[string]string{
			"department_name":            "Computer Science",
			"department_email":           "dept@example.com",
			"lab_assistant_name":         "Anna",
			"lab_assistant_mobile_phone": "+380501234567",
			"department_phone":           "+380441234567",
		})
		Expect(ranges).To(HaveLen(5))
		Expect(ranges[0].values[0][0]).To(Equal("кафедри Computer Science"))
		Expect(ranges[3].values[0][0]).To(Equal("'+380501234567"))
		Expect(phoneCellValue("0441234567")).To(Equal("0441234567"))
		Expect(phoneCellValue("")).To(BeEmpty())

		data := scenario.ReportData{
			Appendices:         [28]scenario.ReportAppendix{{Number: 1, NumericTotal: 2, TextValues: []string{"text"}, Sources: []scenario.ReportSource{{Name: "source"}}}},
			SupportingEvidence: []scenario.ReportSource{{SubmissionID: "submission"}},
			InvalidMaterials:   []scenario.ReportSource{{Reason: "invalid"}},
			MissingSubmissions: []string{"missing"},
			Conflicts:          []scenario.ReportConflict{{Appendix: 1, Values: []string{"a", "b"}}},
		}
		sections := reportSections(data)
		Expect(sections).To(HaveLen(5))
		Expect(sections[0].rows).To(HaveLen(29))
		Expect(sections[1].rows).To(HaveLen(2))
		Expect(sections[4].rows).To(HaveLen(2))
		Expect(join([]string{"a", "b"})).To(Equal("a; b"))
		Expect(join(nil)).To(BeEmpty())
	})

	It("loads the template layout", func() {
		path, err := filepath.Abs("../../../templates/scientific-report/output_example.xlsx")
		Expect(err).NotTo(HaveOccurred())
		appendices, slashRows, values := loadTemplateLayout(path)
		Expect(appendices).NotTo(BeEmpty())
		Expect(slashRows).NotTo(BeEmpty())
		Expect(values).To(HaveLen(200))
		Expect(path).NotTo(BeEmpty())
	})
})
