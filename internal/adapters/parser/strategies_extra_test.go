package parser

import (
	"context"
	"strings"

	"github.com/alex/assistent/internal/domain/submission"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/xuri/excelize/v2"
)

var _ = Describe("parser strategies", func() {
	It("exposes types and handles appendices", func() {
		strategies := Strategies()
		Expect(strategies).To(HaveLen(8))
		for _, strategy := range strategies {
			Expect(strategy.Type()).NotTo(BeEmpty())
		}
		context := submission.ParseContext{ScenarioType: "ScienceReportScenario"}
		material := submission.Material{Name: "appendix-03.xlsx", MIMEType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"}
		Expect(strategies[3].CanHandle(material, context)).To(BeTrue())
		Expect(strategies[3].CanHandle(material, submission.ParseContext{})).To(BeFalse())
		Expect(appendixNumber(submission.Material{Name: "appendix-29.docx"})).To(Equal(0))
		Expect(appendixNumber(submission.Material{Name: "appendix-03.docx"})).To(Equal(3))
	})
})

type fixedParser struct{}

func (fixedParser) Type() string                                                { return "fixed" }
func (fixedParser) CanHandle(submission.Material, submission.ParseContext) bool { return true }
func (fixedParser) Parse(context.Context, submission.Material, submission.ParseContext) (submission.ParsedDocument, error) {
	return submission.ParsedDocument{}, nil
}

var _ = Describe("science appendix parser", func() {
	It("adds the appendix number", func() {
		parser := scienceAppendix{DocumentParser: fixedParser{}}
		document, err := parser.Parse(context.Background(), submission.Material{Name: "appendix-03.docx"}, submission.ParseContext{})
		Expect(err).NotTo(HaveOccurred())
		Expect(document.Source.AppendixNumber).NotTo(BeNil())
		Expect(*document.Source.AppendixNumber).To(Equal(3))
	})
})

var _ = Describe("context reader", func() {
	It("honors cancellation", func() {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := (contextReader{ctx: ctx, r: strings.NewReader("value")}).Read(make([]byte, 5))
		Expect(err).To(MatchError(context.Canceled))
	})
})

var _ = Describe("sheet parser", func() {
	It("honors workbook budgets", func() {
		book := excelize.NewFile()
		Expect(book.SetCellValue("Sheet1", "A1", "value")).To(Succeed())
		defer book.Close()
		for name, budget := range map[string]workbookBudget{
			"rows":  {rows: 0, cells: 10, text: 10},
			"cells": {rows: 10, cells: 0, text: 10},
			"text":  {rows: 10, cells: 10, text: 0},
		} {
			By(name)
			_, err := parseSheet(context.Background(), book, "Sheet1", 1, &budget)
			Expect(err).To(HaveOccurred())
		}
	})
})
