package notification

import (
	"context"
	"fmt"
	"net/http"

	"github.com/alex/assistent/internal/domain/ports"
	"github.com/alex/assistent/internal/domain/report"
	"github.com/alex/assistent/internal/platform/config"
)

type reportDeliveryEmailProvider struct {
	client resendClient
	from   string
}

func NewReportDeliveryEmailProvider(cfg config.EmailConfig) ports.ResultDeliveryProvider {
	if cfg.APIKey == "" || cfg.From == "" {
		return noopReportDeliveryProvider{}
	}
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = "https://api.resend.com"
	}
	return &reportDeliveryEmailProvider{client: &resendHTTPClient{baseURL: baseURL, apiKey: cfg.APIKey, client: http.DefaultClient}, from: cfg.From}
}

func (p *reportDeliveryEmailProvider) Name() string { return "email" }

func (p *reportDeliveryEmailProvider) Deliver(ctx context.Context, result report.Result, destination string) error {
	body := fmt.Sprintf("The final aggregate report is ready.\n\nTask: %d", result.Data.TaskID)
	if result.ErrorMessage != "" {
		body += "\n\n" + result.ErrorMessage
	}
	return p.client.Send(ctx, p.from, destination, "Final scientific activity report", body)
}

type noopReportDeliveryProvider struct{}

func (noopReportDeliveryProvider) Name() string { return "email" }
func (noopReportDeliveryProvider) Deliver(context.Context, report.Result, string) error {
	return fmt.Errorf("Resend report delivery is not configured")
}
