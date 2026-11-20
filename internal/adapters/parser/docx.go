package parser

import (
	"archive/zip"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"strings"

	"github.com/alex/assistent/internal/domain/submission"
)

const maxWordXML = 16 << 20

// DOCX handles Word files and Drive exports of Google Docs. It reads paragraphs
// and table cells from the main document, not embedded objects or remote links.
type DOCX struct{}

func (DOCX) Type() string { return "docx" }

func (DOCX) CanHandle(item submission.Material, _ submission.ParseContext) bool {
	return matchesFormat(item, "application/vnd.google-apps.document", "application/vnd.openxmlformats-officedocument.wordprocessingml.document", ".docx")
}

func (DOCX) Parse(ctx context.Context, item submission.Material, _ submission.ParseContext) (submission.ParsedDocument, error) {
	archive, err := zip.OpenReader(item.LocalPath)
	if err != nil {
		return submission.ParsedDocument{}, err
	}
	defer archive.Close()
	for _, part := range archive.File {
		if part.Name == "word/document.xml" {
			return parseWordDocument(ctx, part)
		}
	}
	return submission.ParsedDocument{}, fmt.Errorf("DOCX main document is missing")
}

func parseWordDocument(ctx context.Context, part *zip.File) (submission.ParsedDocument, error) {
	if part.UncompressedSize64 > maxWordXML {
		return submission.ParsedDocument{}, fmt.Errorf("DOCX main document exceeds size limit")
	}
	reader, err := part.Open()
	if err != nil {
		return submission.ParsedDocument{}, err
	}
	defer reader.Close()
	limited := &io.LimitedReader{R: contextReader{ctx, reader}, N: maxWordXML + 1}
	decoder := xml.NewDecoder(limited)
	var word struct {
		XMLName xml.Name  `xml:"document"`
		Body    *wordBody `xml:"body"`
	}
	if err := decoder.Decode(&word); err != nil {
		return submission.ParsedDocument{}, err
	}
	// Read through EOF to enforce the limit and verify the archive checksum.
	if _, err := io.Copy(io.Discard, limited); err != nil {
		return submission.ParsedDocument{}, err
	}
	if limited.N == 0 || !wordNamespace(word.XMLName.Space) || word.Body == nil {
		return submission.ParsedDocument{}, fmt.Errorf("invalid DOCX main document")
	}
	return word.Body.document(), nil
}

func wordNamespace(namespace string) bool {
	return namespace == "http://schemas.openxmlformats.org/wordprocessingml/2006/main" ||
		namespace == "http://purl.oclc.org/ooxml/wordprocessingml/main"
}

type wordBody struct {
	Paragraphs []wordParagraph `xml:"p"`
	Tables     []wordTable     `xml:"tbl"`
}

type wordTable struct {
	Rows []struct {
		Cells []struct {
			Paragraphs []wordParagraph `xml:"p"`
		} `xml:"tc"`
	} `xml:"tr"`
}

func (b wordBody) document() submission.ParsedDocument {
	var document submission.ParsedDocument
	for i, paragraph := range b.Paragraphs {
		if text := string(paragraph); text != "" {
			document.Fields = append(document.Fields, submission.Field{Value: text, Location: submission.Location{Paragraph: i + 1}})
		}
	}
	for i, source := range b.Tables {
		table := submission.Table{Index: i + 1}
		for row, sourceRow := range source.Rows {
			var cells []submission.Field
			for col, cell := range sourceRow.Cells {
				lines := make([]string, len(cell.Paragraphs))
				for j, paragraph := range cell.Paragraphs {
					lines[j] = string(paragraph)
				}
				cells = append(cells, submission.Field{
					Value: strings.Join(lines, "\n"), Location: submission.Location{Table: table.Index, Row: row + 1, Column: col + 1},
				})
			}
			table.Rows = append(table.Rows, cells)
		}
		document.Tables = append(document.Tables, table)
	}
	return document
}

type wordParagraph string

func (p *wordParagraph) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	var text strings.Builder
	for depth := 1; depth > 0; {
		token, err := d.Token()
		if err != nil {
			return err
		}
		switch t := token.(type) {
		case xml.StartElement:
			if wordNamespace(t.Name.Space) {
				switch t.Name.Local {
				case "t":
					var value string
					if err := d.DecodeElement(&value, &t); err != nil {
						return err
					}
					text.WriteString(value)
					continue
				case "tab":
					text.WriteByte('\t')
				case "br", "cr":
					text.WriteByte('\n')
				}
			}
			depth++
		case xml.EndElement:
			depth--
		}
	}
	*p = wordParagraph(text.String())
	return nil
}
