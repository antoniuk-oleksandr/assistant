package notification

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alex/assistent/internal/domain/notification"
	"github.com/alex/assistent/internal/domain/report"
	"github.com/alex/assistent/internal/platform/config"
)

func TestNewReportReadyNotifierUsesNoopWithoutEmailConfiguration(t *testing.T) {
	notifier := NewReportReadyNotifier(config.EmailConfig{})
	if notifier.Enabled() {
		t.Fatal("notifier should be disabled without Resend configuration")
	}
	if err := notifier.Notify(context.Background(), report.Result{}, "link"); err != nil {
		t.Fatal(err)
	}
}

func TestReminderEmailProviderUsesConfiguredResendClient(t *testing.T) {
	provider := NewReminderEmailProvider(config.EmailConfig{})
	if provider.Name() != "email" {
		t.Fatalf("disabled provider name = %q", provider.Name())
	}
	if err := provider.SendReminder(context.Background(), notification.Reminder{}); err == nil {
		t.Fatal("disabled provider unexpectedly sent a reminder")
	}
	if configured := NewReminderEmailProvider(config.EmailConfig{APIKey: "re_test", From: "from@example.com"}); configured.Name() != "email" {
		t.Fatalf("configured provider name = %q", configured.Name())
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/emails" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	configured := NewReminderEmailProvider(config.EmailConfig{APIKey: "re_test", From: "from@example.com", BaseURL: server.URL}).(*reminderEmailProvider)
	if err := configured.SendReminder(context.Background(), notification.Reminder{Destination: "to@example.com", Subject: "Reminder", Message: "Please submit"}); err != nil {
		t.Fatal(err)
	}
}

func TestNewReportReadyNotifierEnablesConfiguredResend(t *testing.T) {
	notifier := NewReportReadyNotifier(config.EmailConfig{APIKey: "re_test", TargetEmail: "target@example.com", TemplatePath: "templates/report-ready-email.txt"})
	if !notifier.Enabled() {
		t.Fatal("configured notifier should be enabled")
	}
	configured, ok := notifier.(*ReportReadyNotifier)
	if !ok || !strings.HasSuffix(configured.template, filepath.Join("templates", "report-ready-email.txt")) {
		t.Fatalf("configured notifier = %#v", notifier)
	}
}

func TestResolveTemplatePathFallsBackForMissingPath(t *testing.T) {
	path := "missing/report-ready-email.txt"
	if got := resolveTemplatePath(path); got != path {
		t.Fatalf("resolveTemplatePath() = %q, want %q", got, path)
	}
}

func TestReportReadyNotifierRendersGoogleSheetLink(t *testing.T) {
	dir := t.TempDir()
	templatePath := filepath.Join(dir, "report.txt")
	if err := os.WriteFile(templatePath, []byte("Open <google_sheet_link>"), 0o600); err != nil {
		t.Fatal(err)
	}
	notifier := &ReportReadyNotifier{template: templatePath}
	// Resend is deliberately not exercised here; the template expansion is
	// covered through the unexported message builder below.
	message, err := notifier.renderMessage("https://sheet.example")
	if err != nil {
		t.Fatal(err)
	}
	if message != "Open https://sheet.example" {
		t.Fatalf("message = %q", message)
	}
}

type fakeResendClient struct {
	from, to, subject, body string
	err                     error
}

func (f *fakeResendClient) Send(_ context.Context, from, to, subject, body string) error {
	f.from, f.to, f.subject, f.body = from, to, subject, body
	return f.err
}

func TestReportReadyNotifierSendsRenderedMessage(t *testing.T) {
	dir := t.TempDir()
	templatePath := filepath.Join(dir, "report.txt")
	if err := os.WriteFile(templatePath, []byte("Sheet: <google_sheet_link>"), 0o600); err != nil {
		t.Fatal(err)
	}
	client := &fakeResendClient{}
	notifier := &ReportReadyNotifier{client: client, target: "target@example.com", from: "reports@example.com", template: templatePath}
	if !notifier.Enabled() {
		t.Fatal("notifier should be enabled")
	}
	if err := notifier.Notify(context.Background(), report.Result{}, "https://sheet.example"); err != nil {
		t.Fatal(err)
	}
	if client.from != "reports@example.com" || client.to != "target@example.com" || client.subject != "Scientific activity report is ready" || client.body != "Sheet: https://sheet.example" {
		t.Fatalf("Resend message = %#v", client)
	}
}

func TestReportReadyNotifierReturnsResendError(t *testing.T) {
	dir := t.TempDir()
	templatePath := filepath.Join(dir, "report.txt")
	if err := os.WriteFile(templatePath, []byte("<google_sheet_link>"), 0o600); err != nil {
		t.Fatal(err)
	}
	notifier := &ReportReadyNotifier{client: &fakeResendClient{err: io.ErrUnexpectedEOF}, template: templatePath}
	if err := notifier.Notify(context.Background(), report.Result{}, "link"); err == nil || !strings.Contains(err.Error(), "send report email with Resend") {
		t.Fatalf("Notify() error = %v", err)
	}
}

func TestResendHTTPClientSendsRequestAndReportsHTTPFailure(t *testing.T) {
	var got map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/emails" || r.Header.Get("Authorization") != "Bearer re_test" {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	client := &resendHTTPClient{baseURL: server.URL, apiKey: "re_test", client: server.Client()}
	if err := client.Send(context.Background(), "from@example.com", "to@example.com", "Subject", "Body"); err != nil {
		t.Fatal(err)
	}
	if got["from"] != "from@example.com" || got["subject"] != "Subject" {
		t.Fatalf("payload = %#v", got)
	}

	failure := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "failed", http.StatusBadGateway) }))
	defer failure.Close()
	client.baseURL = failure.URL
	if err := client.Send(context.Background(), "from", "to", "subject", "body"); err == nil || !strings.Contains(err.Error(), "Resend API returned 502") {
		t.Fatalf("Send() error = %v", err)
	}
}
