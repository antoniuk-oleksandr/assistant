package renderer

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/alex/assistent/internal/domain/scenario"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("local report renderer", func() {
	It("writes a structured report package", func() {
		renderer := NewLocalReportRenderer()
		data := scenario.ReportData{ScenarioID: 11, TaskID: 22}
		data.Appendices[0].Number = 1
		data.Appendices[0].TextValues = []string{"value"}

		output, err := renderer.RenderAndSave(context.Background(), data)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { _ = os.Remove(output.Path) })
		Expect(output.Format).To(Equal("local-package"))
		Expect(output.Version).To(Equal(1))
		Expect(output.Path).NotTo(BeEmpty())
		Expect(filepath.Base(output.Path)).NotTo(Equal(output.Path))
		contents, err := os.ReadFile(output.Path)
		Expect(err).NotTo(HaveOccurred())
		var got scenario.ReportData
		Expect(json.Unmarshal(contents, &got)).To(Succeed())
		Expect(got.ScenarioID).To(Equal(data.ScenarioID))
		Expect(got.TaskID).To(Equal(data.TaskID))
		Expect(got.Appendices[0].TextValues[0]).To(Equal("value"))
	})

	It("reports output directory failures", func() {
		parent := GinkgoT().TempDir()
		invalidDirectory := filepath.Join(parent, "not-a-directory")
		Expect(os.WriteFile(invalidDirectory, []byte("file"), 0600)).To(Succeed())
		renderer := NewLocalReportRenderer()
		renderer.directory = invalidDirectory
		_, err := renderer.RenderAndSave(context.Background(), scenario.ReportData{})
		Expect(err).To(HaveOccurred())
	})

	It("honors XLSX cancellation", func() {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := NewLocalXLSXRenderer().RenderAndSave(ctx, scenario.ReportData{})
		Expect(err).To(HaveOccurred())
	})

	It("writes all XLSX report sections", func() {
		renderer := NewLocalXLSXRenderer()
		data := scenario.ReportData{
			Appendices:         [28]scenario.ReportAppendix{{Number: 1, NumericTotal: 2, TextValues: []string{"A", "B"}}},
			SupportingEvidence: []scenario.ReportSource{{SubmissionID: "s1", Submitter: "person", MaterialID: "m1", Name: "file", URI: "uri", LocalPath: "path", Reason: "reason"}},
			InvalidMaterials:   []scenario.ReportSource{{Name: "bad", Reason: "invalid"}},
			MissingSubmissions: []string{"missing"},
			Conflicts:          []scenario.ReportConflict{{Appendix: 1, Values: []string{"one", "two"}}},
		}
		output, err := renderer.RenderAndSave(context.Background(), data)
		Expect(err).NotTo(HaveOccurred())
		defer os.Remove(output.Path)
		Expect(output.Format).To(Equal("google-sheets"))
		Expect(output.Path).NotTo(BeEmpty())
	})

	It("reports injected filesystem failures", func() {
		originalMarshal, originalMkdir, originalCreate := localMarshalIndent, localMkdirAll, localCreateTemp
		originalWrite, originalClose := localWriteFile, localCloseFile
		DeferCleanup(func() {
			localMarshalIndent, localMkdirAll, localCreateTemp = originalMarshal, originalMkdir, originalCreate
			localWriteFile, localCloseFile = originalWrite, originalClose
		})
		localMarshalIndent = func(any, string, string) ([]byte, error) { return nil, errors.New("marshal failed") }
		_, err := NewLocalReportRenderer().RenderAndSave(context.Background(), scenario.ReportData{})
		Expect(err).To(MatchError(ContainSubstring("marshal report package")))
		localMarshalIndent = originalMarshal
		localMkdirAll = func(string, os.FileMode) error { return errors.New("mkdir failed") }
		_, err = NewLocalReportRenderer().RenderAndSave(context.Background(), scenario.ReportData{})
		Expect(err).To(MatchError(ContainSubstring("create report output directory")))
		localMkdirAll = originalMkdir
		localCreateTemp = func(string, string) (*os.File, error) { return nil, errors.New("temp failed") }
		_, err = NewLocalReportRenderer().RenderAndSave(context.Background(), scenario.ReportData{})
		Expect(err).To(MatchError(ContainSubstring("create report package")))
		localCreateTemp = originalCreate
		localWriteFile = func(*os.File, []byte) (int, error) { return 0, errors.New("write failed") }
		_, err = NewLocalReportRenderer().RenderAndSave(context.Background(), scenario.ReportData{})
		Expect(err).To(MatchError(ContainSubstring("write report package")))
		localWriteFile = originalWrite
		localCloseFile = func(*os.File) error { return errors.New("close failed") }
		_, err = NewLocalReportRenderer().RenderAndSave(context.Background(), scenario.ReportData{})
		Expect(err).To(MatchError(ContainSubstring("close report package")))
	})
})
