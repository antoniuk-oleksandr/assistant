package renderer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/alex/assistent/internal/platform/config"
	"golang.org/x/oauth2"
)

const sheetsScope = "https://www.googleapis.com/auth/spreadsheets"

type sheetsClient interface {
	CreateSpreadsheet(context.Context, string) (string, error)
	AddSheets(context.Context, string, []string) error
	WriteValues(context.Context, string, string, [][]any) error
}

type googleSheetsClient struct {
	baseURL string
	client  *http.Client
}

func newGoogleSheetsClient(cfg config.SheetsConfig) (sheetsClient, error) {
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = "https://sheets.googleapis.com/v4"
	}
	u, err := url.Parse(baseURL)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "https" && u.Scheme != "http") {
		return nil, fmt.Errorf("invalid GOOGLE_SHEETS_BASE_URL")
	}
	if cfg.ClientID == "" && cfg.ClientSecret == "" && cfg.RefreshToken == "" {
		return nil, fmt.Errorf("Google Sheets OAuth credentials are required")
	}
	if cfg.ClientID == "" || cfg.ClientSecret == "" || cfg.RefreshToken == "" {
		return nil, fmt.Errorf("Google Sheets OAuth requires GOOGLE_SHEETS_OAUTH_CLIENT_ID, GOOGLE_SHEETS_OAUTH_CLIENT_SECRET, and GOOGLE_SHEETS_OAUTH_REFRESH_TOKEN")
	}
	oauth := oauth2.Config{ClientID: cfg.ClientID, ClientSecret: cfg.ClientSecret, Scopes: []string{sheetsScope}, Endpoint: oauth2.Endpoint{TokenURL: "https://oauth2.googleapis.com/token", AuthStyle: oauth2.AuthStyleInParams}}
	tokens := oauth.TokenSource(context.Background(), &oauth2.Token{RefreshToken: cfg.RefreshToken})
	return &googleSheetsClient{baseURL: strings.TrimRight(u.String(), "/"), client: oauth2.NewClient(context.Background(), tokens)}, nil
}

func (c *googleSheetsClient) request(ctx context.Context, method, path string, payload any, result any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("google sheets API returned %s: %s", resp.Status, strings.TrimSpace(string(detail)))
	}
	if result == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
		return fmt.Errorf("decode Google Sheets response: %w", err)
	}
	return nil
}

func (c *googleSheetsClient) CreateSpreadsheet(ctx context.Context, title string) (string, error) {
	var response struct {
		SpreadsheetID string `json:"spreadsheetId"`
	}
	if err := c.request(ctx, http.MethodPost, "/spreadsheets", map[string]any{"properties": map[string]string{"title": title}}, &response); err != nil {
		return "", err
	}
	if response.SpreadsheetID == "" {
		return "", fmt.Errorf("Google Sheets response did not contain spreadsheetId")
	}
	return response.SpreadsheetID, nil
}

func (c *googleSheetsClient) AddSheets(ctx context.Context, id string, names []string) error {
	requests := make([]map[string]any, 0, len(names))
	for _, name := range names {
		requests = append(requests, map[string]any{"addSheet": map[string]any{"properties": map[string]string{"title": name}}})
	}
	return c.request(ctx, http.MethodPost, "/spreadsheets/"+url.PathEscape(id)+":batchUpdate", map[string]any{"requests": requests}, nil)
}

func (c *googleSheetsClient) WriteValues(ctx context.Context, id, sheet string, values [][]any) error {
	payload := map[string]any{"valueInputOption": "USER_ENTERED", "data": []map[string]any{{"range": sheet + "!A1", "majorDimension": "ROWS", "values": values}}}
	return c.request(ctx, http.MethodPost, "/spreadsheets/"+url.PathEscape(id)+"/values:batchUpdate", payload, nil)
}
