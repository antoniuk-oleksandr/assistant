package parser

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/alex/assistent/internal/domain/submission"
	"github.com/xuri/excelize/v2"
)

func TestDocumentParsersReadSupportedFiles(t *testing.T) {
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

func TestDocumentParsersRejectInvalidInputs(t *testing.T) {
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

func TestParserStrategiesUseReportAppendixPrecedence(t *testing.T) {
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

func TestParserStrategiesHandleWorkspaceExportsAndRejectAmbiguousAppendices(t *testing.T) {
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

func TestMatchesFormatUsesMIMEAndExportExtensions(t *testing.T) {
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
