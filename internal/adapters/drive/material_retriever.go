package drive

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/alex/assistent/internal/adapters/googlefiles"
	"github.com/alex/assistent/internal/domain/ports"
	"github.com/alex/assistent/internal/domain/submission"
	"github.com/alex/assistent/internal/platform/config"
	"golang.org/x/oauth2"
)

const (
	defaultBaseURL = "https://www.googleapis.com/drive/v3"
	metadataFields = "id,name,mimeType,size,webViewLink,capabilities"
	maxMetadata    = 1 << 20
	maxFileBytes   = 100 << 20
)

type fileMetadata struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	MIMEType     string `json:"mimeType"`
	Size         *int64 `json:"size,string"`
	WebViewLink  string `json:"webViewLink"`
	Capabilities struct {
		CanDownload *bool `json:"canDownload"`
	} `json:"capabilities"`
}

type materialRetriever struct {
	baseURL     string
	tokens      oauth2.TokenSource
	materialDir string
	client      *http.Client
}

func NewMaterialRetriever(cfg config.DriveConfig) (ports.MaterialRetriever, error) {
	tokens, err := newTokenSource(cfg, "https://oauth2.googleapis.com/token", newHTTPClient())
	if err != nil {
		return nil, err
	}
	return NewMaterialRetrieverWithTokenSource(cfg, tokens)
}

// NewMaterialRetrieverWithTokenSource replaces only credential acquisition;
// the same HTTP, status translation and persistence paths are used in tests.
func NewMaterialRetrieverWithTokenSource(cfg config.DriveConfig, tokens oauth2.TokenSource) (ports.MaterialRetriever, error) {
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	u, err := url.Parse(baseURL)
	if err != nil || !validBaseURL(u) {
		return nil, fmt.Errorf("invalid GOOGLE_DRIVE_BASE_URL: expected HTTPS or loopback HTTP without credentials, query, or fragment")
	}
	return &materialRetriever{
		baseURL:     strings.TrimRight(u.String(), "/"),
		tokens:      tokens,
		materialDir: cfg.MaterialDir,
		client:      newHTTPClient(),
	}, nil
}

func newHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 45 * time.Second,
		// Redirect targets must not receive OAuth credentials or Drive resource keys.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

func validBaseURL(u *url.URL) bool {
	if u.Hostname() == "" || u.User != nil || u.ForceQuery || u.RawQuery != "" || u.Fragment != "" {
		return false
	}
	if u.Scheme == "https" {
		return true
	}
	host := u.Hostname()
	return u.Scheme == "http" && (host == "localhost" || net.ParseIP(host).IsLoopback())
}

func (r *materialRetriever) Retrieve(ctx context.Context, item submission.Material) submission.Material {
	item.LocalPath = ""
	item.Parsing = nil
	item.ParseOutcomes = nil
	item.Status = submission.Failed
	if !googlefiles.ValidID(item.ExternalID) {
		return item
	}
	ref := googlefiles.ParseURL(item.SourceURI)
	var resourceKeys string
	if ref.ID == item.ExternalID && ref.ResourceKey != "" {
		resourceKeys = ref.ID + "/" + ref.ResourceKey
	}

	query := url.Values{"fields": {metadataFields}, "supportsAllDrives": {"true"}}
	metadata, status := r.metadata(ctx, item.ExternalID, query, resourceKeys)
	if status != submission.Pending {
		item.Status = status
		return item
	}
	item = metadata.enrich(item)
	if metadata.Capabilities.CanDownload != nil && !*metadata.Capabilities.CanDownload {
		item.Status = submission.AccessDenied
		return item
	}

	path, query, extension := contentRequest(item)
	if query == nil {
		item.Status = submission.Pending
		item.Parsing = &submission.ParseResult{Issue: submission.UnsupportedMaterial}
		return item
	}
	if path == "" && item.SizeBytes != nil && *item.SizeBytes > maxFileBytes {
		return item
	}
	response, status := r.get(ctx, item.ExternalID+path, query, resourceKeys)
	if status != submission.Pending {
		item.Status = status
		return item
	}
	defer response.Body.Close()
	if response.ContentLength > maxFileBytes {
		return item
	}
	localPath, err := saveFile(ctx, r.materialDir, extension, response.Body)
	if err == nil {
		item.LocalPath = localPath
		item.Status = submission.Pending // downloaded, awaiting parsing
	}
	return item
}

func (r *materialRetriever) metadata(ctx context.Context, id string, query url.Values, resourceKeys string) (fileMetadata, submission.MaterialStatus) {
	response, status := r.get(ctx, id, query, resourceKeys)
	var result fileMetadata
	if status != submission.Pending {
		return result, status
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, maxMetadata+1))
	if err != nil || len(body) > maxMetadata || json.Unmarshal(body, &result) != nil || result.ID != id || result.MIMEType == "" {
		return fileMetadata{}, submission.Failed
	}
	if result.Size != nil && *result.Size < 0 {
		return fileMetadata{}, submission.Failed
	}
	return result, submission.Pending
}

func (m fileMetadata) enrich(item submission.Material) submission.Material {
	if m.Name != "" {
		item.Name = m.Name
	}
	if m.WebViewLink != "" && item.SourceURI == "" {
		item.SourceURI = m.WebViewLink
	}
	item.MIMEType = m.MIMEType
	item.SizeBytes = m.Size
	return item
}

func (r *materialRetriever) get(ctx context.Context, path string, query url.Values, resourceKeys string) (*http.Response, submission.MaterialStatus) {
	if ctx.Err() != nil {
		return nil, submission.Failed
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.baseURL+"/files/"+path+"?"+query.Encode(), nil)
	if err != nil {
		return nil, submission.Failed
	}
	if r.tokens == nil {
		return nil, submission.AccessDenied
	}
	token, err := r.tokens.Token()
	if err != nil || !token.Valid() {
		// OAuth errors may contain credentials or provider response bodies.
		return nil, submission.Failed
	}
	token.SetAuthHeader(req)
	if resourceKeys != "" {
		req.Header.Set("X-Goog-Drive-Resource-Keys", resourceKeys)
	}
	response, err := r.client.Do(req)
	if err != nil {
		return nil, submission.Failed
	}
	if response.StatusCode == http.StatusOK {
		return response, submission.Pending
	}
	defer response.Body.Close()
	switch response.StatusCode {
	case http.StatusForbidden:
		if isQuotaError(response.Body) {
			return nil, submission.Failed
		}
		return nil, submission.AccessDenied
	case http.StatusUnauthorized:
		return nil, submission.AccessDenied
	case http.StatusNotFound, http.StatusGone:
		return nil, submission.NotFound
	default:
		return nil, submission.Failed
	}
}
