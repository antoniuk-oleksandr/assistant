package parser

import (
	"mime"
	"path/filepath"
	"strings"

	"github.com/alex/assistent/internal/domain/submission"
)

// Provider MIME metadata takes precedence over filename. The local extension
// can describe a Drive export, while the MIME metadata describes its source.
func matchesFormat(item submission.Material, nativeMIME, officeMIME, ext string) bool {
	raw := strings.TrimSpace(item.MIMEType)
	kind, _, err := mime.ParseMediaType(raw)
	if err != nil && raw != "" {
		return false
	}
	switch kind {
	case nativeMIME, officeMIME:
		return true
	case "", "application/octet-stream", "application/zip":
		nameExt := filepath.Ext(item.Name)
		return strings.EqualFold(nameExt, ext) ||
			((nameExt == "" || strings.EqualFold(nameExt, ".bin")) && strings.EqualFold(filepath.Ext(item.LocalPath), ext))
	default:
		return false
	}
}
