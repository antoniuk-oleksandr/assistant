package drive

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/alex/assistent/internal/domain/submission"
)

const (
	googleMIME = "application/vnd.google-apps."
	docxMIME   = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	xlsxMIME   = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
)

var safeExtension = regexp.MustCompile(`^\.[a-z0-9]{1,10}$`)

func contentRequest(item submission.Material) (path string, query url.Values, extension string) {
	switch item.MIMEType {
	case googleMIME + "document":
		return "/export", url.Values{"mimeType": {docxMIME}}, ".docx"
	case googleMIME + "spreadsheet":
		return "/export", url.Values{"mimeType": {xlsxMIME}}, ".xlsx"
	}
	if strings.HasPrefix(item.MIMEType, googleMIME) {
		return "", nil, ""
	}
	return "", url.Values{"alt": {"media"}, "supportsAllDrives": {"true"}}, fileExtension(item)
}

func fileExtension(item submission.Material) string {
	switch item.MIMEType {
	case docxMIME:
		return ".docx"
	case xlsxMIME:
		return ".xlsx"
	case "application/pdf":
		return ".pdf"
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	}
	ext := strings.ToLower(filepath.Ext(item.Name))
	if safeExtension.MatchString(ext) {
		return ext
	}
	return ".bin"
}

func saveFile(ctx context.Context, dir, extension string, src io.Reader) (string, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	file, err := os.CreateTemp(dir, ".drive-*")
	if err != nil {
		return "", err
	}
	tempPath := file.Name()
	defer os.Remove(tempPath)
	hash := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(file, hash), io.LimitReader(src, maxFileBytes+1))
	closeErr := file.Close()
	if copyErr != nil {
		return "", copyErr
	}
	if closeErr != nil {
		return "", closeErr
	}
	if written > maxFileBytes {
		return "", fmt.Errorf("Drive file exceeds %d bytes", maxFileBytes)
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	path := filepath.Join(dir, fmt.Sprintf("%x%s", hash.Sum(nil), extension))
	// Both paths are on the same filesystem. Publish atomically without replacing
	// an artifact that a previously committed material row may still reference.
	if err := os.Link(tempPath, path); os.IsExist(err) {
		info, statErr := os.Lstat(path)
		if statErr != nil || !info.Mode().IsRegular() || info.Size() != written {
			return "", fmt.Errorf("invalid existing Drive artifact at digest path")
		}
	} else if err != nil {
		return "", err
	}
	return path, nil
}
