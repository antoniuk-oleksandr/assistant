package infrastructure_test

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/alex/assistent/internal/adapters/parser"
	"github.com/alex/assistent/internal/application"
	"github.com/alex/assistent/internal/domain/submission"
	"github.com/alex/assistent/tests/support/fixtures"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/xuri/excelize/v2"
)

func TestMaterialFormats(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Material format adapters")
}

var _ = Describe("Office material parsers", func() {
	var service *application.MaterialParsing
	BeforeEach(func() { service = application.NewMaterialParsing(parser.DOCX{}, parser.XLSX{}) })

	DescribeTable("selects the format from provider metadata or a generic file extension",
		func(mime, name, localPath, selected string) {
			item := submission.Material{MIMEType: mime, Name: name, LocalPath: localPath}
			origin := fixtures.MaterialOrigin()
			Expect(parser.DOCX{}.CanHandle(item, origin)).To(Equal(selected == "docx"))
			Expect(parser.XLSX{}.CanHandle(item, origin)).To(Equal(selected == "xlsx"))
		},
		Entry("Google Docs with a title and no extension", fixtures.GoogleDocMIME, "Report", "snapshot", "docx"),
		Entry("Google Sheets metadata over a misleading filename", fixtures.GoogleSheetMIME, "Report.docx", "snapshot", "xlsx"),
		Entry("Word MIME with parameters", fixtures.DOCXMIME+"; charset=utf-8", "Report", "snapshot", "docx"),
		Entry("Excel MIME", fixtures.XLSXMIME, "Report", "snapshot", "xlsx"),
		Entry("uppercase filename extension with a generic MIME", "application/octet-stream", "Report.DOCX", "snapshot", "docx"),
		Entry("local export extension when the title has none", "", "Report", "snapshot.XLSX", "xlsx"),
		Entry("explicit PDF MIME cannot be reclassified by a filename", "application/pdf", "Report.docx", "snapshot.docx", ""),
	)

	It("extracts DOCX paragraphs and table cells with original coordinates", func() {
		item := fixtures.SubmittedMaterial()
		item.LocalPath = fixtures.DOCX(GinkgoT(), `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>
<w:p/><w:p><w:r><w:t>Field &amp; lab</w:t><w:tab/><w:t>summary</w:t></w:r></w:p>
<w:tbl><w:tr><w:tc><w:p><w:r><w:t>Count</w:t></w:r></w:p></w:tc><w:tc><w:p><w:r><w:t>12</w:t></w:r></w:p></w:tc></w:tr></w:tbl>
</w:body></w:document>`)
		result, err := service.Parse(context.Background(), item, fixtures.MaterialOrigin())
		Expect(err).NotTo(HaveOccurred())
		Expect(result.ParserType).To(Equal("docx"))
		Expect(result.Issue).To(BeEmpty())
		Expect(result.Document).NotTo(BeNil())
		Expect(result.Document.Fields).To(Equal([]submission.Field{{Value: "Field & lab\tsummary", Location: submission.Location{Paragraph: 2}}}))
		Expect(result.Document.Tables).To(Equal([]submission.Table{{Index: 1, Rows: [][]submission.Field{{
			{Value: "Count", Location: submission.Location{Table: 1, Row: 1, Column: 1}},
			{Value: "12", Location: submission.Location{Table: 1, Row: 1, Column: 2}},
		}}}}))
	})

	It("extracts Google Sheets exports without renumbering sparse cells or sheets", func() {
		item := fixtures.SubmittedMaterial()
		item.MIMEType = fixtures.GoogleSheetMIME
		item.LocalPath = fixtures.XLSX(GinkgoT())
		result, err := service.Parse(context.Background(), item, fixtures.MaterialOrigin())
		Expect(err).NotTo(HaveOccurred())
		Expect(result.ParserType).To(Equal("xlsx"))
		Expect(result.Issue).To(BeEmpty())
		Expect(result.Document).NotTo(BeNil())
		Expect(result.Document.Tables).To(Equal([]submission.Table{
			{Name: "Results", Index: 1, Rows: [][]submission.Field{
				{{Value: "Metric", Location: submission.Location{Table: 1, Row: 1, Column: 1}}},
				{{Value: "12", Location: submission.Location{Table: 1, Row: 4, Column: 4}}},
			}},
			{Name: "Evidence", Index: 2, Rows: [][]submission.Field{
				{{Value: "Sample A", Location: submission.Location{Table: 2, Row: 2, Column: 2}}},
			}},
		}))
	})

	It("extracts the raw XLSX decimal instead of its rounded display value", func() {
		item := fixtures.SubmittedMaterial()
		item.MIMEType = fixtures.XLSXMIME
		item.LocalPath = filepath.Join(GinkgoT().TempDir(), "rounded.xlsx")
		book := excelize.NewFile()
		DeferCleanup(book.Close)
		style, err := book.NewStyle(&excelize.Style{NumFmt: 1}) // Built-in integer display format (`0`).
		Expect(err).NotTo(HaveOccurred())
		Expect(book.SetCellValue("Sheet1", "B3", 12.49)).To(Succeed())
		Expect(book.SetCellStyle("Sheet1", "B3", "B3", style)).To(Succeed())
		Expect(book.GetCellValue("Sheet1", "B3")).To(Equal("12"))
		Expect(book.SaveAs(item.LocalPath)).To(Succeed())

		result, err := service.Parse(context.Background(), item, fixtures.MaterialOrigin())
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Issue).To(BeEmpty())
		Expect(result.Document).NotTo(BeNil())
		Expect(result.Document.Tables).To(Equal([]submission.Table{{
			Name: "Sheet1", Index: 1, Rows: [][]submission.Field{{
				{Value: "12.49", Location: submission.Location{Table: 1, Row: 3, Column: 2}},
			}},
		}}))
	})

	It("reports malformed DOCX XML as a parse failure, not as unsupported", func() {
		item := fixtures.SubmittedMaterial()
		item.LocalPath = fixtures.DOCX(GinkgoT(), `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>`)
		result, err := service.Parse(context.Background(), item, fixtures.MaterialOrigin())
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(submission.ParseResult{ParserType: "docx", Issue: submission.ParseFailed}))
	})

	appendix4, appendix9 := 4, 9
	DescribeTable("interprets appendix metadata only in the appropriate scenario",
		func(scenarioType, title, mime, selected, role string, explicit, appendix *int) {
			origin := fixtures.MaterialOrigin()
			origin.ScenarioType = scenarioType
			item := fixtures.SubmittedMaterial()
			item.Name, item.MIMEType, item.Role, item.AppendixNumber = title, mime, "supporting_evidence", explicit
			if explicit != nil {
				item.Role = "appendix"
			}
			item.LocalPath = fixtures.DOCX(GinkgoT(), `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>Summary</w:t></w:r></w:p></w:body></w:document>`)
			result, err := application.NewMaterialParsing(parser.Strategies()...).Parse(context.Background(), item, origin)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.ParserType).To(Equal(selected))
			Expect(result.Issue).To(BeEmpty())
			Expect(result.Document).NotTo(BeNil())
			Expect(result.Document.Fields).To(Equal([]submission.Field{{Value: "Summary", Location: submission.Location{Paragraph: 1}}}))
			source := item.DocumentSource(origin)
			source.Role, source.AppendixNumber = role, appendix
			Expect(result.Document.Source).To(Equal(source))
		},
		Entry("the science binding infers a canonical title on a Google Docs export", "ScienceReportScenario", "Appendix-04", fixtures.GoogleDocMIME, "science-report-appendix/google-docs", "appendix", nil, &appendix4),
		Entry("another binding uses the Workspace fallback without inference", "OtherScenario", "Appendix-04", fixtures.GoogleDocMIME, "google-docs", "supporting_evidence", nil, nil),
		Entry("an ambiguous title uses the DOCX fallback", "ScienceReportScenario", "Appendix-04 notes.docx", fixtures.DOCXMIME, "docx", "supporting_evidence", nil, nil),
		Entry("explicit metadata takes precedence over the title", "ScienceReportScenario", "Appendix-04.docx", fixtures.DOCXMIME, "science-report-appendix/docx", "appendix", &appendix9, &appendix9),
	)

	It("rejects a workbook with a truncated worksheet instead of returning a partial document", func() {
		original, err := zip.OpenReader(fixtures.XLSX(GinkgoT()))
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(original.Close)
		item := fixtures.SubmittedMaterial()
		item.MIMEType = fixtures.XLSXMIME
		item.LocalPath = filepath.Join(GinkgoT().TempDir(), "truncated.xlsx")
		file, err := os.Create(item.LocalPath)
		Expect(err).NotTo(HaveOccurred())
		defer file.Close()
		archive := zip.NewWriter(file)
		replaced := false
		for _, part := range original.File {
			if part.Name != "xl/worksheets/sheet2.xml" {
				Expect(archive.Copy(part)).To(Succeed())
				continue
			}
			worksheet, err := archive.Create(part.Name)
			Expect(err).NotTo(HaveOccurred())
			_, err = worksheet.Write([]byte(`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData><row r="1"><c r="A1"><v>12</v></c></row>`))
			Expect(err).NotTo(HaveOccurred())
			replaced = true
		}
		Expect(replaced).To(BeTrue())
		Expect(archive.Close()).To(Succeed())
		Expect(file.Close()).To(Succeed())

		result, err := service.Parse(context.Background(), item, fixtures.MaterialOrigin())
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(submission.ParseResult{ParserType: "xlsx", Issue: submission.ParseFailed}))
	})
})
