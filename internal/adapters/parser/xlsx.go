package parser

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/alex/assistent/internal/domain/submission"
	"github.com/xuri/excelize/v2"
)

const (
	maxWorkbookBytes = 100 << 20
	maxWorkbookXML   = 128 << 20
	maxWorkbookRows  = 1_048_576
	maxCells         = 200_000
	maxCellTextBytes = 16 << 20
)

// XLSX handles downloaded Excel files and Drive exports of Google Sheets.
type XLSX struct{}

func (XLSX) Type() string { return "xlsx" }

func (XLSX) CanHandle(item submission.Material, _ submission.ParseContext) bool {
	return matchesFormat(item, "application/vnd.google-apps.spreadsheet", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", ".xlsx")
}

func (XLSX) Parse(ctx context.Context, item submission.Material, _ submission.ParseContext) (submission.ParsedDocument, error) {
	file, err := os.Open(item.LocalPath)
	if err != nil {
		return submission.ParsedDocument{}, err
	}
	defer file.Close()
	if err := validateWorkbook(ctx, file); err != nil {
		return submission.ParsedDocument{}, err
	}
	book, err := excelize.OpenReader(io.LimitReader(contextReader{ctx, file}, maxWorkbookBytes+1), excelize.Options{
		UnzipSizeLimit: maxWorkbookXML, UnzipXMLSizeLimit: 8 << 20,
	})
	if book != nil {
		defer book.Close()
	}
	if err != nil {
		return submission.ParsedDocument{}, err
	}
	names := book.GetSheetList()
	if len(names) == 0 {
		return submission.ParsedDocument{}, fmt.Errorf("workbook has no sheets")
	}
	var document submission.ParsedDocument
	budget := workbookBudget{rows: maxWorkbookRows, cells: maxCells, text: maxCellTextBytes}
	for i, name := range names {
		if err := ctx.Err(); err != nil {
			return submission.ParsedDocument{}, err
		}
		table, err := parseSheet(ctx, book, name, i+1, &budget)
		if err != nil {
			return submission.ParsedDocument{}, err
		}
		document.Tables = append(document.Tables, table)
	}
	return document, nil
}

type workbookBudget struct{ rows, cells, text int }

func parseSheet(ctx context.Context, book *excelize.File, name string, index int, budget *workbookBudget) (submission.Table, error) {
	table := submission.Table{Name: name, Index: index}
	rows, err := book.Rows(name)
	if err != nil {
		return table, err
	}
	defer rows.Close()
	for row := 1; rows.Next(); row++ {
		budget.rows--
		if budget.rows < 0 {
			return table, fmt.Errorf("workbook exceeds row limit")
		}
		if err := ctx.Err(); err != nil {
			return table, err
		}
		values, err := rows.Columns(excelize.Options{RawCellValue: true})
		if err != nil {
			return table, err
		}
		budget.cells -= len(values)
		if budget.cells < 0 {
			return table, fmt.Errorf("workbook exceeds cell limit")
		}
		var cells []submission.Field
		for col, value := range values {
			if value == "" {
				continue
			}
			budget.text -= len(value)
			if budget.text < 0 {
				return table, fmt.Errorf("workbook exceeds cell text limit")
			}
			cells = append(cells, submission.Field{Value: value, Location: submission.Location{Table: index, Row: row, Column: col + 1}})
		}
		if len(cells) != 0 {
			table.Rows = append(table.Rows, cells)
		}
	}
	return table, rows.Error()
}
