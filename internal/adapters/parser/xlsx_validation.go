package parser

import (
	"archive/zip"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"strings"
)

// Excelize's streaming iterator can turn XML decoder errors into EOF. Check
// package XML independently before extracting any cells from it.
func validateWorkbook(ctx context.Context, file *os.File) error {
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if info.Size() > maxWorkbookBytes {
		return fmt.Errorf("workbook exceeds file size limit")
	}
	archive, err := zip.NewReader(file, info.Size())
	if err != nil {
		return err
	}
	var remaining uint64 = maxWorkbookXML
	var manifest *zip.File
	for _, part := range archive.File {
		if err := ctx.Err(); err != nil {
			return err
		}
		if part.UncompressedSize64 > remaining {
			return fmt.Errorf("workbook exceeds uncompressed size limit")
		}
		remaining -= part.UncompressedSize64
		if part.Name == "[Content_Types].xml" {
			manifest = part
		}
	}
	declaredXML := make(map[string]bool)
	if manifest != nil {
		if err := validateWorkbookPart(ctx, manifest, func(element xml.StartElement) {
			if element.Name.Local != "Override" {
				return
			}
			var name, kind string
			for _, attr := range element.Attr {
				switch attr.Name.Local {
				case "PartName":
					name = strings.TrimPrefix(attr.Value, "/")
				case "ContentType":
					kind = attr.Value
				}
			}
			if strings.HasSuffix(kind, "+xml") || kind == "application/xml" || kind == "text/xml" {
				declaredXML[name] = true
			}
		}); err != nil {
			return fmt.Errorf("invalid workbook content types: %w", err)
		}
	}
	for _, part := range archive.File {
		name := strings.ToLower(part.Name)
		if part == manifest || (!declaredXML[part.Name] && !strings.HasSuffix(name, ".xml") && !strings.HasSuffix(name, ".rels")) {
			continue
		}
		if err := validateWorkbookPart(ctx, part, nil); err != nil {
			return fmt.Errorf("invalid workbook XML: %w", err)
		}
	}
	return nil
}

func validateWorkbookPart(ctx context.Context, part *zip.File, onStart func(xml.StartElement)) error {
	reader, err := part.Open()
	if err != nil {
		return err
	}
	defer reader.Close()
	decoder := xml.NewDecoder(contextReader{ctx, reader})
	depth, root := 0, false
	for {
		token, err := decoder.Token()
		if err == io.EOF && root && depth == 0 {
			return nil
		}
		if err != nil {
			return err
		}
		switch element := token.(type) {
		case xml.StartElement:
			if depth == 0 && root {
				return fmt.Errorf("multiple XML root elements")
			}
			root = true
			depth++
			if onStart != nil {
				onStart(element)
			}
		case xml.EndElement:
			depth--
		case xml.CharData:
			if depth == 0 && strings.TrimSpace(string(element)) != "" {
				return fmt.Errorf("unexpected text outside XML root")
			}
		}
	}
}
