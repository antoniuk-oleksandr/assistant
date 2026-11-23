package parser

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/alex/assistent/internal/domain/submission"
	. "github.com/onsi/ginkgo/v2"
	"github.com/xuri/excelize/v2"
)

type ginkgoTest struct{}

func (*ginkgoTest) TempDir() string                   { return GinkgoT().TempDir() }
func (*ginkgoTest) Cleanup(fn func())                 { DeferCleanup(fn) }
func (*ginkgoTest) Fatal(args ...any)                 { Fail(fmt.Sprint(args...)) }
func (*ginkgoTest) Fatalf(format string, args ...any) { Fail(fmt.Sprintf(format, args...)) }
func (*ginkgoTest) Errorf(format string, args ...any) { Fail(fmt.Sprintf(format, args...)) }
func (*ginkgoTest) Run(name string, fn func(*ginkgoTest)) bool {
	By(name)
	fn(&ginkgoTest{})
	return true
}

func testDocumentParsersReadSupportedFiles(t *ginkgoTest) {
	dir := t.TempDir()
	xlsxPath := filepath.Join(dir, "report.xlsx")
	book := excelize.NewFile()
	if err := book.SetCellValue("Sheet1", "A1", "value"); err != nil {
		t.Fatal(err)
	}
	if err := book.SaveAs(xlsxPath); err != nil {
		t.Fatal(err)
	}
	if err := book.Close(); err != nil {
		t.Fatal(err)
	}

	xlsx := XLSX{}
	if !xlsx.CanHandle(submission.Material{Name: "report.xlsx"}, submission.ParseContext{}) {
		t.Fatal("XLSX should handle xlsx files")
	}
	document, err := xlsx.Parse(context.Background(), submission.Material{LocalPath: xlsxPath}, submission.ParseContext{})
	if err != nil || len(document.Tables) != 1 || len(document.Tables[0].Rows) != 1 {
		t.Fatalf("XLSX.Parse() = %#v, %v", document, err)
	}

	docxPath := filepath.Join(dir, "report.docx")
	file, err := os.Create(docxPath)
	if err != nil {
		t.Fatal(err)
	}
	archive := zip.NewWriter(file)
	part, err := archive.Create("word/document.xml")
	if err != nil {
		t.Fatal(err)
	}
	_, err = part.Write([]byte(`<document xmlns="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><body><p><r><t>Hello</t></r></p></body></document>`))
	if err != nil {
		t.Fatal(err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	docx := DOCX{}
	if !docx.CanHandle(submission.Material{Name: "report.docx"}, submission.ParseContext{}) {
		t.Fatal("DOCX should handle docx files")
	}
	document, err = docx.Parse(context.Background(), submission.Material{LocalPath: docxPath}, submission.ParseContext{})
	if err != nil || len(document.Fields) != 1 || document.Fields[0].Value != "Hello" {
		t.Fatalf("DOCX.Parse() = %#v, %v", document, err)
	}
}

func testDocumentParsersRejectInvalidInputs(t *ginkgoTest) {
	if _, err := (XLSX{}).Parse(context.Background(), submission.Material{LocalPath: filepath.Join(t.TempDir(), "missing.xlsx")}, submission.ParseContext{}); err == nil {
		t.Fatal("missing XLSX unexpectedly parsed")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.docx")
	if err := os.WriteFile(path, []byte("not a zip"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := (DOCX{}).Parse(context.Background(), submission.Material{LocalPath: path}, submission.ParseContext{}); err == nil {
		t.Fatal("invalid DOCX unexpectedly parsed")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (XLSX{}).Parse(ctx, submission.Material{LocalPath: path}, submission.ParseContext{}); err == nil {
		t.Fatal("canceled XLSX unexpectedly parsed")
	}
}

func testDOCXRejectsInvalidMainDocumentShapes(t *ginkgoTest) {
	for name, xmlDocument := range map[string]string{
		"wrong namespace": `<document xmlns="urn:wrong"><body/></document>`,
		"missing body":    `<document xmlns="http://schemas.openxmlformats.org/wordprocessingml/2006/main"/>`,
		"malformed":       `<document>`,
	} {
		t.Run(name, func(t *ginkgoTest) {
			path := filepath.Join(t.TempDir(), "invalid.docx")
			file, err := os.Create(path)
			if err != nil {
				t.Fatal(err)
			}
			archive := zip.NewWriter(file)
			part, err := archive.Create("word/document.xml")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := part.Write([]byte(xmlDocument)); err != nil {
				t.Fatal(err)
			}
			if err := archive.Close(); err != nil {
				t.Fatal(err)
			}
			if err := file.Close(); err != nil {
				t.Fatal(err)
			}
			if _, err := (DOCX{}).Parse(context.Background(), submission.Material{LocalPath: path}, submission.ParseContext{}); err == nil {
				t.Fatal("invalid document unexpectedly parsed")
			}
		})
	}
}

func testDOCXRejectsArchiveWithoutMainDocument(t *ginkgoTest) {
	path := filepath.Join(t.TempDir(), "missing-main.docx")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	archive := zip.NewWriter(file)
	if _, err := archive.Create("word/styles.xml"); err != nil {
		t.Fatal(err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := (DOCX{}).Parse(context.Background(), submission.Material{LocalPath: path}, submission.ParseContext{}); err == nil || !strings.Contains(err.Error(), "main document is missing") {
		t.Fatalf("missing document error = %v", err)
	}
}

func testDOCXRejectsOversizedMainDocument(t *ginkgoTest) {
	path := filepath.Join(t.TempDir(), "large.docx")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	archive := zip.NewWriter(file)
	part, err := archive.Create("word/document.xml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(append([]byte(`<document>`), bytes.Repeat([]byte("x"), maxWordXML)...)); err != nil {
		t.Fatal(err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := (DOCX{}).Parse(context.Background(), submission.Material{LocalPath: path}, submission.ParseContext{}); err == nil || !strings.Contains(err.Error(), "exceeds size limit") {
		t.Fatalf("oversized document error = %v", err)
	}
}

func testDOCXExtractsTablesAndWordBreaks(t *ginkgoTest) {
	path := filepath.Join(t.TempDir(), "table.docx")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	archive := zip.NewWriter(file)
	part, err := archive.Create("word/document.xml")
	if err != nil {
		t.Fatal(err)
	}
	_, err = part.Write([]byte(`<document xmlns="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><body><p><r><t>A</t><tab/><t>B</t><br/><t>C</t></r></p><tbl><tr><tc><p><r><t>cell</t></r></p></tc></tr></tbl></body></document>`))
	if err != nil {
		t.Fatal(err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	document, err := (DOCX{}).Parse(context.Background(), submission.Material{LocalPath: path}, submission.ParseContext{})
	if err != nil || len(document.Fields) != 1 || document.Fields[0].Value != "A\tB\nC" || len(document.Tables) != 1 || document.Tables[0].Rows[0][0].Value != "cell" {
		t.Fatalf("DOCX.Parse() = %#v, %v", document, err)
	}
}

func testXLSXRejectsEmptyWorkbookAndHonorsCancellation(t *ginkgoTest) {
	path := filepath.Join(t.TempDir(), "empty.xlsx")
	book := excelize.NewFile()
	if _, err := book.NewSheet("Second"); err != nil {
		t.Fatal(err)
	}
	if err := book.SaveAs(path); err != nil {
		t.Fatal(err)
	}
	if err := book.Close(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (XLSX{}).Parse(ctx, submission.Material{LocalPath: path}, submission.ParseContext{}); err == nil {
		t.Fatal("canceled workbook unexpectedly parsed")
	}
}

func testXLSXRejectsMalformedWorkbookXML(t *ginkgoTest) {
	path := filepath.Join(t.TempDir(), "malformed.xlsx")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	archive := zip.NewWriter(file)
	part, err := archive.Create("[Content_Types].xml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte(`<Types><Override>`)); err != nil {
		t.Fatal(err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := (XLSX{}).Parse(context.Background(), submission.Material{LocalPath: path}, submission.ParseContext{}); err == nil {
		t.Fatal("malformed workbook unexpectedly parsed")
	}
}

func testXLSXRejectsOversizedFileBeforeOpeningWorkbook(t *ginkgoTest) {
	path := filepath.Join(t.TempDir(), "oversized.xlsx")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(maxWorkbookBytes + 1); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := (XLSX{}).Parse(context.Background(), submission.Material{LocalPath: path}, submission.ParseContext{}); err == nil || !strings.Contains(err.Error(), "file size limit") {
		t.Fatalf("oversized workbook error = %v", err)
	}
}

func testStrategiesRejectMalformedMIMEAndAppendixNames(t *ginkgoTest) {
	strategies := Strategies()
	for _, strategy := range strategies {
		if strategy.CanHandle(submission.Material{Name: "appendix-01.docx", MIMEType: "bad;mime"}, submission.ParseContext{ScenarioType: "ScienceReportScenario"}) {
			t.Fatalf("strategy %q accepted malformed MIME", strategy.Type())
		}
	}
	for _, name := range []string{"appendix-00.docx", "appendix-99.docx", "appendix-1.docx", "appendix-010.docx"} {
		if appendixNumber(submission.Material{Name: name}) != 0 {
			t.Fatalf("invalid appendix name accepted: %s", name)
		}
	}
}

func testValidateWorkbookPartRejectsMultipleRootsAndText(t *ginkgoTest) {
	for _, xmlDocument := range []string{"<a/><b/>", "text<a/>"} {
		path := filepath.Join(t.TempDir(), "part.zip")
		file, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		archive := zip.NewWriter(file)
		part, err := archive.Create("part.xml")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write([]byte(xmlDocument)); err != nil {
			t.Fatal(err)
		}
		if err := archive.Close(); err != nil {
			t.Fatal(err)
		}
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
		reader, err := zip.OpenReader(path)
		if err != nil {
			t.Fatal(err)
		}
		err = validateWorkbookPart(context.Background(), reader.File[0], nil)
		_ = reader.Close()
		if err == nil {
			t.Fatalf("invalid XML %q was accepted", xmlDocument)
		}
	}
}

func testParserStrategiesUseReportAppendixPrecedence(t *ginkgoTest) {
	strategies := Strategies()
	item := submission.Material{Name: "appendix-04.docx", MIMEType: "application/vnd.openxmlformats-officedocument.wordprocessingml.document"}
	origin := submission.ParseContext{ScenarioType: "ScienceReportScenario"}
	var selected portsParser
	for _, strategy := range strategies {
		if strategy.CanHandle(item, origin) {
			selected = portsParser{kind: strategy.Type()}
			break
		}
	}
	if selected.kind != "science-report-appendix/docx" {
		t.Fatalf("selected parser = %q", selected.kind)
	}
	if appendixNumber(item) != 4 || validAppendix(0) != 0 || validAppendix(29) != 0 {
		t.Fatal("appendix validation failed")
	}
}

func testParserStrategiesHandleWorkspaceExportsAndRejectAmbiguousAppendices(t *ginkgoTest) {
	strategies := Strategies()
	origin := submission.ParseContext{ScenarioType: "ScienceReportScenario"}
	for _, item := range []submission.Material{
		{Name: "document", MIMEType: "application/vnd.google-apps.document"},
		{Name: "sheet", MIMEType: "application/vnd.google-apps.spreadsheet"},
	} {
		found := false
		for _, strategy := range strategies {
			if strategy.CanHandle(item, origin) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("no strategy handles %#v", item)
		}
	}
	for _, name := range []string{"appendix-1.docx", "appendix-29.docx", "appendix-aa.docx"} {
		if appendixNumber(submission.Material{Name: name}) != 0 {
			t.Fatalf("ambiguous appendix %q was inferred", name)
		}
	}
	if appendixNumber(submission.Material{Name: "Appendix-04.xlsx"}) != 4 {
		t.Fatal("case-insensitive appendix name was not inferred")
	}
	appendix := 4
	if appendixNumber(submission.Material{AppendixNumber: &appendix}) != appendix {
		t.Fatal("explicit appendix number was not retained")
	}
	invalid := 29
	if appendixNumber(submission.Material{AppendixNumber: &invalid}) != 0 {
		t.Fatal("invalid explicit appendix number was accepted")
	}
}

func testMatchesFormatUsesMIMEAndExportExtensions(t *ginkgoTest) {
	parser := XLSX{}
	for _, item := range []submission.Material{
		{Name: "report.xlsx", MIMEType: ""},
		{Name: "report.bin", LocalPath: "/tmp/report.xlsx", MIMEType: "application/octet-stream"},
		{Name: "report.xlsx", MIMEType: "application/vnd.google-apps.spreadsheet"},
	} {
		if !parser.CanHandle(item, submission.ParseContext{}) {
			t.Fatalf("XLSX did not handle %#v", item)
		}
	}
	for _, item := range []submission.Material{{Name: "report.pdf", MIMEType: "application/pdf"}, {Name: "report.txt", MIMEType: "bad;mime"}} {
		if parser.CanHandle(item, submission.ParseContext{}) {
			t.Fatalf("XLSX handled %#v", item)
		}
	}
}

type portsParser struct{ kind string }

var _ = Describe("document parsers", func() {
	It("reads supported files", func() { testDocumentParsersReadSupportedFiles(&ginkgoTest{}) })
	It("rejects invalid inputs", func() { testDocumentParsersRejectInvalidInputs(&ginkgoTest{}) })
	It("rejects invalid DOCX main document shapes", func() { testDOCXRejectsInvalidMainDocumentShapes(&ginkgoTest{}) })
	It("rejects archives without a main document", func() { testDOCXRejectsArchiveWithoutMainDocument(&ginkgoTest{}) })
	It("rejects oversized DOCX documents", func() { testDOCXRejectsOversizedMainDocument(&ginkgoTest{}) })
	It("extracts DOCX tables and word breaks", func() { testDOCXExtractsTablesAndWordBreaks(&ginkgoTest{}) })
	It("rejects empty XLSX workbooks and honors cancellation", func() { testXLSXRejectsEmptyWorkbookAndHonorsCancellation(&ginkgoTest{}) })
	It("rejects malformed workbook XML", func() { testXLSXRejectsMalformedWorkbookXML(&ginkgoTest{}) })
	It("rejects oversized XLSX files", func() { testXLSXRejectsOversizedFileBeforeOpeningWorkbook(&ginkgoTest{}) })
	It("rejects malformed MIME and appendix names", func() { testStrategiesRejectMalformedMIMEAndAppendixNames(&ginkgoTest{}) })
	It("rejects invalid workbook parts", func() { testValidateWorkbookPartRejectsMultipleRootsAndText(&ginkgoTest{}) })
	It("uses report appendix precedence", func() { testParserStrategiesUseReportAppendixPrecedence(&ginkgoTest{}) })
	It("handles workspace exports and ambiguous appendices", func() { testParserStrategiesHandleWorkspaceExportsAndRejectAmbiguousAppendices(&ginkgoTest{}) })
	It("matches MIME types and export extensions", func() { testMatchesFormatUsesMIMEAndExportExtensions(&ginkgoTest{}) })
})
