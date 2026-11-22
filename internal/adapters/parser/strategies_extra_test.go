package parser

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/alex/assistent/internal/domain/submission"
	"github.com/xuri/excelize/v2"
)

func TestParserStrategiesExposeTypesAndAppendixHandling(t *testing.T) {
	strategies := Strategies()
	if len(strategies) != 8 {
		t.Fatalf("strategy count = %d", len(strategies))
	}
	for _, strategy := range strategies {
		if strategy.Type() == "" {
			t.Fatalf("empty strategy type for %T", strategy)
		}
	}
	context := submission.ParseContext{ScenarioType: "ScienceReportScenario"}
	material := submission.Material{Name: "appendix-03.xlsx", MIMEType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"}
	if !strategies[3].CanHandle(material, context) {
		t.Fatal("appendix strategy did not accept a valid appendix")
	}
	if strategies[3].CanHandle(material, submission.ParseContext{}) {
		t.Fatal("appendix strategy accepted the wrong scenario")
	}
	if appendixNumber(submission.Material{Name: "appendix-29.docx"}) != 0 || appendixNumber(submission.Material{Name: "appendix-03.docx"}) != 3 {
		t.Fatal("appendix number inference mismatch")
	}
}

type fixedParser struct{}

func (fixedParser) Type() string                                                { return "fixed" }
func (fixedParser) CanHandle(submission.Material, submission.ParseContext) bool { return true }
func (fixedParser) Parse(context.Context, submission.Material, submission.ParseContext) (submission.ParsedDocument, error) {
	return submission.ParsedDocument{}, nil
}

func TestScienceAppendixParserAddsAppendixNumber(t *testing.T) {
	parser := scienceAppendix{DocumentParser: fixedParser{}}
	document, err := parser.Parse(context.Background(), submission.Material{Name: "appendix-03.docx"}, submission.ParseContext{})
	if err != nil || document.Source.AppendixNumber == nil || *document.Source.AppendixNumber != 3 {
		t.Fatalf("parsed appendix = %#v, error = %v", document, err)
	}
}

func TestContextReaderHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := (contextReader{ctx: ctx, r: strings.NewReader("value")}).Read(make([]byte, 5))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Read() error = %v", err)
	}
}

func TestParseSheetHonorsWorkbookBudgets(t *testing.T) {
	book := excelize.NewFile()
	if err := book.SetCellValue("Sheet1", "A1", "value"); err != nil {
		t.Fatal(err)
	}
	defer book.Close()
	for name, budget := range map[string]workbookBudget{
		"rows":  {rows: 0, cells: 10, text: 10},
		"cells": {rows: 10, cells: 0, text: 10},
		"text":  {rows: 10, cells: 10, text: 0},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parseSheet(context.Background(), book, "Sheet1", 1, &budget); err == nil {
				t.Fatal("budget overflow was not reported")
			}
		})
	}
}
