package renderer

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"

	"github.com/alex/assistent/internal/domain/scenario"
	"github.com/alex/assistent/internal/platform/config"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type fakeSheetsClient struct {
	id        string
	title     string
	sheets    []string
	values    map[string][][]any
	createErr error
	addErr    error
	writeErr  error
	ranges    []sheetValueRange
	clear     []string
	rangeErr  error
	clearErr  error
}

func (f *fakeSheetsClient) CreateSpreadsheet(_ context.Context, title string) (string, error) {
	f.title = title
	if f.createErr != nil {
		return "", f.createErr
	}
	return f.id, nil
}
func (f *fakeSheetsClient) AddSheets(_ context.Context, _ string, names []string) error {
	if f.addErr != nil {
		return f.addErr
	}
	f.sheets = append(f.sheets, names...)
	return nil
}
func (f *fakeSheetsClient) WriteValues(_ context.Context, _ string, sheet string, rows [][]any) error {
	if f.writeErr != nil {
		return f.writeErr
	}
	if f.values == nil {
		f.values = map[string][][]any{}
	}
	f.values[sheet] = rows
	return nil
}

func (f *fakeSheetsClient) WriteRanges(_ context.Context, _ string, ranges []sheetValueRange) error {
	if f.rangeErr != nil {
		return f.rangeErr
	}
	f.ranges = ranges
	return nil
}

func (f *fakeSheetsClient) ClearRanges(_ context.Context, _ string, ranges []string) error {
	if f.clearErr != nil {
		return f.clearErr
	}
	f.clear = ranges
	return nil
}

type fakeTemplateUploader struct {
	id    string
	err   error
	title string
	path  string
}

type sheetsOnlyClient struct{}

func (sheetsOnlyClient) CreateSpreadsheet(context.Context, string) (string, error) {
	return "sheet", nil
}
func (sheetsOnlyClient) AddSheets(context.Context, string, []string) error          { return nil }
func (sheetsOnlyClient) WriteValues(context.Context, string, string, [][]any) error { return nil }

type sheetsWriterWithoutClearer struct{ sheetsOnlyClient }

func (sheetsWriterWithoutClearer) WriteRanges(context.Context, string, []sheetValueRange) error {
	return nil
}

func (f *fakeTemplateUploader) UploadSpreadsheet(_ context.Context, title, path string) (string, error) {
	f.title, f.path = title, path
	if f.err != nil {
		return "", f.err
	}
	return f.id, nil
}

func loadTemplateLayoutFromPath(path string) (map[int]int, []int, [][]any) {
	return loadTemplateLayout(path)
}

var _ = Describe("Google Sheets report renderer", func() {
	It("creates an aggregate", func() {
		client := &fakeSheetsClient{id: "sheet-123"}
		renderer := &GoogleSheetsReportRenderer{client: client, title: "Report"}
		data := scenario.ReportData{MissingSubmissions: []string{"missing"}}
		data.Appendices[0].Number = 1
		data.Appendices[0].NumericTotal = 3
		output, err := renderer.RenderAndSave(context.Background(), data)
		Expect(err).NotTo(HaveOccurred())
		Expect(output.Format).To(Equal("google-sheets"))
		Expect(output.Path).To(ContainSubstring("/sheet-123/edit"))
		Expect(client.title).To(Equal("Report"))
		Expect(client.sheets).To(HaveLen(5))
		Expect(client.values["Appendices"][1][1]).To(Equal(float64(3)))
	})

	It("stops on provider errors", func() {
		providerErr := errors.New("provider unavailable")
		for name, client := range map[string]*fakeSheetsClient{
			"create": {createErr: providerErr},
			"add":    {id: "sheet", addErr: providerErr},
			"write":  {id: "sheet", writeErr: providerErr},
		} {
			By(name)
			_, err := (&GoogleSheetsReportRenderer{client: client, title: "Report"}).RenderAndSave(context.Background(), scenario.ReportData{})
			Expect(errors.Is(err, providerErr)).To(BeTrue())
		}
	})

	It("imports a template and updates column C", func() {
		_, filename, _, ok := runtime.Caller(0)
		if !ok {
			Fail("runtime.Caller failed")
		}
		templatePath := filepath.Join(filepath.Dir(filename), "..", "..", "..", "templates", "scientific-report", "output_example.xlsx")
		if _, err := os.Stat(templatePath); err != nil {
			Fail(err.Error())
		}
		appendices, slashRows, columnValues := loadTemplateLayoutFromPath(templatePath)
		client := &fakeSheetsClient{id: "created-but-replaced"}
		uploader := &fakeTemplateUploader{id: "imported-sheet"}
		renderer := &GoogleSheetsReportRenderer{
			client: client, uploader: uploader, title: "Report", templatePath: templatePath,
			templateAppendices: appendices, templateSlashRows: slashRows, templateColumnValues: columnValues,
		}
		data := scenario.ReportData{CourseData: map[string]string{
			"department_name":            "Department",
			"department_email":           "dept@example.com",
			"lab_assistant_name":         "Anna",
			"lab_assistant_mobile_phone": "+3801",
			"department_phone":           "+3802",
		}}
		data.Appendices[0] = scenario.ReportAppendix{Number: 1, NumericTotal: 4}
		data.Appendices[1] = scenario.ReportAppendix{Number: 2}
		output, err := renderer.RenderAndSave(context.Background(), data)
		Expect(err).NotTo(HaveOccurred())
		Expect(output.Path).To(Equal("https://docs.google.com/spreadsheets/d/imported-sheet/edit"))
		Expect(uploader.title).To(Equal("Report"))
		Expect(uploader.path).To(Equal(templatePath))
		Expect(client.ranges).To(HaveLen(6))
		Expect(client.ranges[0].rangeName).To(Equal("Аркуш1!C13"))
		Expect(client.ranges[0].values).To(HaveLen(120))
		if client.ranges[1].rangeName != "Аркуш1!B4" || client.ranges[1].values[0][0] != "кафедри Department" || client.ranges[2].rangeName != "Аркуш1!C7" || client.ranges[2].values[0][0] != "dept@example.com" || client.ranges[3].rangeName != "Аркуш1!C8" || client.ranges[3].values[0][0] != "Anna" || client.ranges[4].values[0][0] != "'+3801" || client.ranges[5].values[0][0] != "'+3802" {
			Fail("course data ranges did not match")
		}
		if client.ranges[0].values[appendices[1]-13][0] != float64(4) {
			Fail("appendix value did not match")
		}
		if client.ranges[0].values[appendices[2]-13][0] != "/" {
			Fail("empty appendix value did not match")
		}
		Expect(client.clear).To(HaveLen(len(templateAlwaysBlankRows)))
	})

	It("reports template errors", func() {
		providerErr := errors.New("provider unavailable")
		base := &GoogleSheetsReportRenderer{
			client:   &fakeSheetsClient{id: "sheet", rangeErr: providerErr},
			uploader: &fakeTemplateUploader{id: "sheet"}, title: "Report", templatePath: "template.xlsx",
			templateAppendices: map[int]int{1: 13}, templateColumnValues: make([][]any, 200),
		}
		_, err := base.RenderAndSave(context.Background(), scenario.ReportData{})
		Expect(errors.Is(err, providerErr)).To(BeTrue())
		base.client = &fakeSheetsClient{id: "sheet", clearErr: providerErr}
		_, err = base.RenderAndSave(context.Background(), scenario.ReportData{})
		Expect(errors.Is(err, providerErr)).To(BeTrue())
		base.client = &fakeSheetsClient{id: "sheet"}
		base.uploader = nil
		_, err = base.RenderAndSave(context.Background(), scenario.ReportData{})
		Expect(err).To(MatchError(ContainSubstring("Drive write access")))
		base.client = sheetsOnlyClient{}
		base.uploader = &fakeTemplateUploader{id: "sheet"}
		_, err = base.RenderAndSave(context.Background(), scenario.ReportData{})
		Expect(err).To(MatchError(ContainSubstring("cell updates")))
		base.client = sheetsWriterWithoutClearer{}
		_, err = base.RenderAndSave(context.Background(), scenario.ReportData{})
		Expect(err).To(MatchError(ContainSubstring("cell clearing")))
	})

	It("selects an explicit local artifact", func() {
		renderer, err := NewConfiguredReportRenderer(config.ReportConfig{}, config.SheetsConfig{}, config.DriveConfig{})
		Expect(err).NotTo(HaveOccurred())
		Expect(renderer).NotTo(BeNil())
		renderer, err = NewConfiguredReportRenderer(config.ReportConfig{Renderer: "local-xlsx"}, config.SheetsConfig{}, config.DriveConfig{})
		Expect(err).NotTo(HaveOccurred())
		Expect(renderer).To(BeAssignableToTypeOf(&LocalXLSXRenderer{}))
	})

	It("selects Google Sheets", func() {
		renderer, err := NewConfiguredReportRenderer(config.ReportConfig{Renderer: "google-sheets"}, config.SheetsConfig{ClientID: "client", ClientSecret: "secret", RefreshToken: "refresh"}, config.DriveConfig{})
		Expect(err).NotTo(HaveOccurred())
		Expect(renderer).To(BeAssignableToTypeOf(&GoogleSheetsReportRenderer{}))
	})

	It("reports Drive configuration errors", func() {
		_, err := NewConfiguredReportRenderer(config.ReportConfig{Renderer: "google-sheets"}, config.SheetsConfig{ClientID: "client", ClientSecret: "secret", RefreshToken: "refresh"}, config.DriveConfig{ClientID: "client"})
		Expect(err).To(MatchError(ContainSubstring("Google Drive OAuth requires")))
	})

	It("rejects unknown selections", func() {
		_, err := NewConfiguredReportRenderer(config.ReportConfig{Renderer: "unknown"}, config.SheetsConfig{}, config.DriveConfig{})
		Expect(err).To(HaveOccurred())
	})
})
