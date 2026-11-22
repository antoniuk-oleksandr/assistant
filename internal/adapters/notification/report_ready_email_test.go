package notification

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alex/assistent/internal/domain/notification"
	"github.com/alex/assistent/internal/domain/ports"
	"github.com/alex/assistent/internal/domain/report"
	"github.com/alex/assistent/internal/domain/teacher"
	"github.com/alex/assistent/internal/platform/config"
)

type fakeSMTPSender struct {
	from, to, subject, body string
	err                     error
}

type fakeReportRecipients struct {
	people   []teacher.Recipient
	channels map[int64][]teacher.Channel
}

func (f fakeReportRecipients) List(context.Context) ([]teacher.Recipient, error) {
	return f.people, nil
}
func (f fakeReportRecipients) ListActiveChannels(_ context.Context, id int64) ([]teacher.Channel, error) {
	return f.channels[id], nil
}
func (fakeReportRecipients) Create(context.Context, *teacher.Recipient) error { return nil }
func (fakeReportRecipients) Update(context.Context, *teacher.Recipient) error { return nil }
func (fakeReportRecipients) Get(context.Context, int64) (teacher.Recipient, error) {
	return teacher.Recipient{}, nil
}
func (fakeReportRecipients) CreateChannel(context.Context, *teacher.Channel) error { return nil }
func (fakeReportRecipients) UpdateChannel(context.Context, *teacher.Channel) error { return nil }

var _ ports.TeacherRepository = fakeReportRecipients{}

func (f *fakeSMTPSender) Send(_ context.Context, from, to, subject, body string) error {
	f.from, f.to, f.subject, f.body = from, to, subject, body
	return f.err
}

func TestEmailProvidersAreDisabledWithoutSMTPCredentials(t *testing.T) {
	if NewReminderEmailProvider(config.EmailConfig{}).Name() != "email" {
		t.Fatal("unexpected reminder provider name")
	}
	if err := NewReminderEmailProvider(config.EmailConfig{}).SendReminder(context.Background(), notification.Reminder{}); err == nil {
		t.Fatal("disabled reminder provider unexpectedly sent")
	}
	if err := NewReportDeliveryEmailProvider(config.EmailConfig{}).Deliver(context.Background(), report.Result{}, "owner@example.com"); err == nil {
		t.Fatal("disabled delivery provider unexpectedly sent")
	}
	notifier := NewReportReadyNotifier(config.EmailConfig{}, nil)
	if notifier.Enabled() {
		t.Fatal("notifier should be disabled without SMTP configuration")
	}
}

func TestEmailProvidersUseGoogleSMTPConfiguration(t *testing.T) {
	cfg := config.EmailConfig{SMTPHost: "smtp.gmail.com", SMTPPort: 587, SMTPUsername: "user@gmail.com", SMTPPassword: "app-password", From: "reports@gmail.com"}
	reminder := NewReminderEmailProvider(cfg).(*reminderEmailProvider)
	if reminder.from != cfg.From || reminder.client.(*googleSMTPClient).host != cfg.SMTPHost {
		t.Fatalf("reminder SMTP configuration = %#v", reminder)
	}
	delivery := NewReportDeliveryEmailProvider(cfg).(*reportDeliveryEmailProvider)
	if delivery.from != cfg.From || delivery.client.(*googleSMTPClient).port != cfg.SMTPPort {
		t.Fatalf("delivery SMTP configuration = %#v", delivery)
	}
}

func TestNewReportReadyNotifierEnablesConfiguredSMTP(t *testing.T) {
	notifier := NewReportReadyNotifier(config.EmailConfig{SMTPUsername: "user@gmail.com", SMTPPassword: "app-password", TemplatePath: "templates/report-ready-email.txt"}, nil)
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

func TestReportReadyNotifierRendersAndSendsThroughSMTP(t *testing.T) {
	dir := t.TempDir()
	templatePath := filepath.Join(dir, "report.txt")
	if err := os.WriteFile(templatePath, []byte("Sheet: <google_sheet_link>"), 0o600); err != nil {
		t.Fatal(err)
	}
	client := &fakeSMTPSender{}
	notifier := &ReportReadyNotifier{client: client, from: "reports@example.com", template: templatePath, recipients: fakeReportRecipients{
		people: []teacher.Recipient{{ID: 1, Email: "target@example.com", Status: teacher.Active}},
	}}
	if err := notifier.Notify(context.Background(), report.Result{}, "https://sheet.example"); err != nil {
		t.Fatal(err)
	}
	if client.from != "reports@example.com" || client.to != "target@example.com" || client.subject != "Scientific activity report is ready" || client.body != "Sheet: https://sheet.example" {
		t.Fatalf("SMTP message = %#v", client)
	}
}

func TestReportReadyNotifierReturnsSMTPError(t *testing.T) {
	dir := t.TempDir()
	templatePath := filepath.Join(dir, "report.txt")
	if err := os.WriteFile(templatePath, []byte("<google_sheet_link>"), 0o600); err != nil {
		t.Fatal(err)
	}
	notifier := &ReportReadyNotifier{client: &fakeSMTPSender{err: errors.New("connection failed")}, from: "reports@example.com", template: templatePath, recipients: fakeReportRecipients{
		people: []teacher.Recipient{{ID: 1, Email: "target@example.com", Status: teacher.Active}},
	}}
	if err := notifier.Notify(context.Background(), report.Result{}, "link"); err == nil || !strings.Contains(err.Error(), "with Google SMTP") {
		t.Fatalf("Notify() error = %v", err)
	}
}

func TestTelegramProviderUsesConfiguredEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/bottoken/sendMessage" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	provider := NewTelegramProvider(config.TelegramConfig{Token: "token", BaseURL: server.URL})
	if err := provider.SendReminder(context.Background(), notification.Reminder{Destination: "chat", Subject: "Reminder", Message: "Submit"}); err != nil {
		t.Fatal(err)
	}
	if err := provider.Deliver(context.Background(), report.Result{}, "chat"); err != nil {
		t.Fatal(err)
	}
}
