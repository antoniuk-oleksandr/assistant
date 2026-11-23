package renderer

import (
	"context"
	"os"
	"path/filepath"

	"github.com/alex/assistent/internal/domain/scenario"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/xuri/excelize/v2"
)

var _ = Describe("Google sheet renderer", func() {
	It("writes aggregate and evidence sheets", func() {
		directory := GinkgoT().TempDir()
		renderer := NewLocalXLSXRenderer()
		renderer.directory = directory
		data := scenario.ReportData{MissingSubmissions: []string{"missing-1"}, SupportingEvidence: []scenario.ReportSource{{SubmissionID: "s1", MaterialID: "e1", Name: "certificate.pdf"}}, Conflicts: []scenario.ReportConflict{{Appendix: 2, Source: scenario.ReportSource{SubmissionID: "s1", MaterialID: "a2"}, Values: []string{"one", "two"}}}}
		data.Appendices[0].Number = 1
		data.Appendices[0].NumericTotal = 4
		data.Appendices[0].TextValues = []string{"award"}
		output, err := renderer.RenderAndSave(context.Background(), data)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { _ = os.Remove(output.Path) })
		Expect(output.Format).To(Equal("google-sheets"))
		Expect(filepath.Ext(output.Path)).To(Equal(".xlsx"))
		book, err := excelize.OpenFile(output.Path)
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = book.Close() }()
		got, err := book.GetCellValue("Appendices", "B2")
		Expect(err).NotTo(HaveOccurred())
		Expect(got).To(Equal("4"))
		got, err = book.GetCellValue("Supporting Evidence", "D2")
		Expect(err).NotTo(HaveOccurred())
		Expect(got).To(Equal("certificate.pdf"))
		got, err = book.GetCellValue("Missing Submissions", "A2")
		Expect(err).NotTo(HaveOccurred())
		Expect(got).To(Equal("missing-1"))
		got, err = book.GetCellValue("Conflicts", "D2")
		Expect(err).NotTo(HaveOccurred())
		Expect(got).To(Equal("one; two"))
	})

	It("honors context and directory errors", func() {
		renderer := NewLocalXLSXRenderer()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := renderer.RenderAndSave(ctx, scenario.ReportData{})
		Expect(err).To(HaveOccurred())
		parent := GinkgoT().TempDir()
		file := filepath.Join(parent, "not-a-directory")
		Expect(os.WriteFile(file, []byte("x"), 0600)).To(Succeed())
		renderer.directory = file
		_, err = renderer.RenderAndSave(context.Background(), scenario.ReportData{})
		Expect(err).To(HaveOccurred())
		renderer.directory = "/proc"
		_, err = renderer.RenderAndSave(context.Background(), scenario.ReportData{})
		Expect(err).To(HaveOccurred())
	})
})
