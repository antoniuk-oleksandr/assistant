package fixtures

import (
	"archive/zip"
	"os"
	"path/filepath"

	"github.com/alex/assistent/internal/domain/submission"
	"github.com/xuri/excelize/v2"
)

const (
	GoogleDocMIME   = "application/vnd.google-apps.document"
	GoogleSheetMIME = "application/vnd.google-apps.spreadsheet"
	DOCXMIME        = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	XLSXMIME        = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
)

func SubmittedMaterial() submission.Material {
	appendix := 4
	return submission.Material{
		ExternalID: "file-1", Name: "Appendix 4", MIMEType: GoogleDocMIME,
		SourceURI: "https://drive.google.com/file/d/file-1/view", LocalPath: "snapshots/file-1.docx",
		Role: "appendix", AppendixNumber: &appendix, Status: submission.Pending,
	}
}

func MaterialOrigin() submission.ParseContext {
	return submission.ParseContext{
		ScenarioName: "annual-report", ScenarioType: "scientific-report",
		CourseID: "course-1", TaskExternalID: "task-1", SubmissionExternalID: "submission-1",
		SubmittedAt: "2026-11-19T10:00:00Z", SubmitterID: "user-1",
		SubmitterName: "Test Submitter", SubmitterEmail: "submitter@example.test",
	}
}

type fixtureT interface {
	Helper()
	TempDir() string
	Fatal(...any)
}

func DOCX(t fixtureT, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "report.docx")
	file, err := os.Create(path)
	must(t, err)
	defer file.Close()
	z := zip.NewWriter(file)
	part, err := z.Create("word/document.xml")
	must(t, err)
	_, err = part.Write([]byte(body))
	must(t, err)
	must(t, z.Close())
	return path
}

func XLSX(t fixtureT) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "report.xlsx")
	book := excelize.NewFile()
	defer book.Close()
	must(t, book.SetSheetName("Sheet1", "Results"))
	must(t, book.SetCellValue("Results", "A1", "Metric"))
	must(t, book.SetCellValue("Results", "D4", 12))
	_, err := book.NewSheet("Evidence")
	must(t, err)
	must(t, book.SetCellValue("Evidence", "B2", "Sample A"))
	must(t, book.SaveAs(path))
	return path
}

func must(t fixtureT, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
