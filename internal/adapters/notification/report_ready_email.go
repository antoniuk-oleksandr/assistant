package notification

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/alex/assistent/internal/domain/ports"
	"github.com/alex/assistent/internal/domain/report"
	"github.com/alex/assistent/internal/platform/config"
)

const googleSheetLinkTag = "<google_sheet_link>"

type resendClient interface {
	Send(context.Context, string, string, string, string) error
}

type ReportReadyNotifier struct {
	client   resendClient
	target   string
	from     string
	template string
}

func NewReportReadyNotifier(cfg config.EmailConfig) ports.ReportReadyNotifier {
	if cfg.APIKey == "" || cfg.TargetEmail == "" {
		return noopReportReadyNotifier{}
	}
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = "https://api.resend.com"
	}
	from := cfg.From
	if from == "" {
		from = "onboarding@resend.dev"
	}
	return &ReportReadyNotifier{client: &resendHTTPClient{baseURL: strings.TrimRight(baseURL, "/"), apiKey: cfg.APIKey, client: http.DefaultClient}, target: cfg.TargetEmail, from: from, template: resolveTemplatePath(cfg.TemplatePath)}
}

func resolveTemplatePath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	directory, err := os.Getwd()
	if err != nil {
		return path
	}
	for {
		candidate := filepath.Join(directory, path)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			return path
		}
		directory = parent
	}
}

func (n *ReportReadyNotifier) Enabled() bool { return true }

func (n *ReportReadyNotifier) Notify(ctx context.Context, _ report.Result, link string) error {
	body, err := n.renderMessage(link)
	if err != nil {
		return err
	}
	if err := n.client.Send(ctx, n.from, n.target, "Scientific activity report is ready", body); err != nil {
		return fmt.Errorf("send report email with Resend: %w", err)
	}
	return nil
}

func (n *ReportReadyNotifier) renderMessage(link string) (string, error) {
	body, err := os.ReadFile(n.template)
	if err != nil {
		return "", fmt.Errorf("read report email template: %w", err)
	}
	message := string(body)
	if !strings.Contains(message, googleSheetLinkTag) {
		return "", errors.New("report email template must contain <google_sheet_link>")
	}
	return strings.ReplaceAll(message, googleSheetLinkTag, link), nil
}

type resendHTTPClient struct {
	baseURL string
	apiKey  string
	client  *http.Client
}

func (c *resendHTTPClient) Send(ctx context.Context, from, to, subject, body string) error {
	payload, err := json.Marshal(map[string]any{"from": from, "to": []string{to}, "subject": subject, "text": body})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/emails", strings.NewReader(string(payload)))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	response, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("Resend API returned %s", response.Status)
	}
	return nil
}

type noopReportReadyNotifier struct{}

func (noopReportReadyNotifier) Enabled() bool                                       { return false }
func (noopReportReadyNotifier) Notify(context.Context, report.Result, string) error { return nil }
