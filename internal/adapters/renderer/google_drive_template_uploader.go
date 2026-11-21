package renderer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/alex/assistent/internal/platform/config"
	"golang.org/x/oauth2"
)

const (
	driveFileScope  = "https://www.googleapis.com/auth/drive.file"
	xlsxMIME        = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	googleSheetMIME = "application/vnd.google-apps.spreadsheet"
)

type templateUploader interface {
	UploadSpreadsheet(context.Context, string, string) (string, error)
}

type googleDriveTemplateUploader struct {
	endpoint string
	client   *http.Client
}

func newGoogleDriveTemplateUploader(cfg config.DriveConfig) (templateUploader, error) {
	if cfg.ClientID == "" && cfg.ClientSecret == "" && cfg.RefreshToken == "" {
		return nil, nil
	}
	if cfg.ClientID == "" || cfg.ClientSecret == "" || cfg.RefreshToken == "" {
		return nil, fmt.Errorf("Google Drive OAuth requires GOOGLE_DRIVE_OAUTH_CLIENT_ID, GOOGLE_DRIVE_OAUTH_CLIENT_SECRET, and GOOGLE_DRIVE_OAUTH_REFRESH_TOKEN")
	}
	base := cfg.BaseURL
	if base == "" {
		base = "https://www.googleapis.com/drive/v3"
	}
	u, err := url.Parse(base)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "https" && u.Scheme != "http") {
		return nil, fmt.Errorf("invalid GOOGLE_DRIVE_BASE_URL")
	}
	oauth := oauth2.Config{
		ClientID: cfg.ClientID, ClientSecret: cfg.ClientSecret,
		Scopes:   []string{driveFileScope},
		Endpoint: oauth2.Endpoint{TokenURL: "https://oauth2.googleapis.com/token", AuthStyle: oauth2.AuthStyleInParams},
	}
	tokens := oauth.TokenSource(context.Background(), &oauth2.Token{RefreshToken: cfg.RefreshToken})
	return &googleDriveTemplateUploader{
		endpoint: u.Scheme + "://" + u.Host + "/upload/drive/v3/files",
		client:   oauth2.NewClient(context.Background(), tokens),
	}, nil
}

func (u *googleDriveTemplateUploader) UploadSpreadsheet(ctx context.Context, title, path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open report template: %w", err)
	}
	defer file.Close()

	var body bytes.Buffer
	multipartWriter := multipart.NewWriter(&body)
	metadata, err := multipartWriter.CreatePart(map[string][]string{
		"Content-Type": {"application/json; charset=UTF-8"},
	})
	if err != nil {
		return "", err
	}
	if err := json.NewEncoder(metadata).Encode(map[string]string{"name": title, "mimeType": googleSheetMIME}); err != nil {
		return "", err
	}
	content, err := multipartWriter.CreatePart(map[string][]string{"Content-Type": {xlsxMIME}})
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(content, file); err != nil {
		return "", fmt.Errorf("read report template: %w", err)
	}
	if err := multipartWriter.Close(); err != nil {
		return "", err
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, u.endpoint+"?uploadType=multipart&supportsAllDrives=true&fields=id", &body)
	if err != nil {
		return "", err
	}
	request.Header.Set("Content-Type", multipartWriter.FormDataContentType())
	response, err := u.client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		detail, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return "", fmt.Errorf("Google Drive template upload returned %s: %s", response.Status, strings.TrimSpace(string(detail)))
	}
	var result struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode Google Drive template upload: %w", err)
	}
	if result.ID == "" {
		return "", fmt.Errorf("Google Drive template upload did not return a file id")
	}
	return result.ID, nil
}
