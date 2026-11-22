package notification

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/alex/assistent/internal/domain/notification"
	"github.com/alex/assistent/internal/domain/ports"
	"github.com/alex/assistent/internal/domain/report"
	"github.com/alex/assistent/internal/domain/scenario"
	"github.com/alex/assistent/internal/domain/task"
	"github.com/alex/assistent/internal/domain/teacher"
	"github.com/alex/assistent/internal/platform/config"
)

type fakeSMTPSender struct {
	from, to, subject, body string
	err                     error
}

type fakeReportReadyTelegram struct {
	destination string
	message     string
	err         error
}

type fakeReportReadyProfile struct{ classroomID string }

func (f fakeReportReadyProfile) GetProfile(context.Context, string) (teacher.Profile, error) {
	return teacher.Profile{ClassroomID: f.classroomID}, nil
}

type failingReportReadyProfile struct{}

func (failingReportReadyProfile) GetProfile(context.Context, string) (teacher.Profile, error) {
	return teacher.Profile{}, errors.New("profile unavailable")
}

func (f *fakeReportReadyTelegram) SendReportReady(_ context.Context, destination, message string) error {
	f.destination, f.message = destination, message
	return f.err
}

type fakeReportRecipients struct {
	people     []teacher.Recipient
	channels   map[int64][]teacher.Channel
	err        error
	channelErr error
}

func (f fakeReportRecipients) List(context.Context) ([]teacher.Recipient, error) {
	return f.people, f.err
}
func (f fakeReportRecipients) ListActiveChannels(_ context.Context, id int64) ([]teacher.Channel, error) {
	if f.channelErr != nil {
		return nil, f.channelErr
	}
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
	if NewReportDeliveryEmailProvider(config.EmailConfig{}).Name() != "email" {
		t.Fatal("unexpected disabled delivery provider name")
	}
	if (noopReportDeliveryProvider{}).Name() != "email" {
		t.Fatal("unexpected noop delivery provider name")
	}
	notifier := NewReportReadyNotifier(config.EmailConfig{}, nil)
	if notifier.Enabled() {
		t.Fatal("notifier should be disabled without SMTP configuration")
	}
	if err := notifier.Notify(context.Background(), report.Result{}, "link"); err != nil {
		t.Fatal(err)
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

func TestEmailProvidersSendUsingTheirSMTPClients(t *testing.T) {
	client := &fakeSMTPSender{}
	reminder := &reminderEmailProvider{client: client, from: "reports@example.com"}
	if reminder.Name() != "email" {
		t.Fatalf("reminder provider name = %q", reminder.Name())
	}
	if err := reminder.SendReminder(context.Background(), notification.Reminder{Destination: "teacher@example.com", Subject: "Reminder", Message: "Submit"}); err != nil {
		t.Fatal(err)
	}
	if client.to != "teacher@example.com" || client.subject != "Reminder" {
		t.Fatalf("reminder message = %#v", client)
	}

	delivery := &reportDeliveryEmailProvider{client: client, from: "reports@example.com"}
	if delivery.Name() != "email" {
		t.Fatalf("delivery provider name = %q", delivery.Name())
	}
	if err := delivery.Deliver(context.Background(), report.Result{Data: scenario.ReportData{TaskID: 42}, ErrorMessage: "partial"}, "head@example.com"); err != nil {
		t.Fatal(err)
	}
	if client.to != "head@example.com" || !strings.Contains(client.body, "Task: 42") || !strings.Contains(client.body, "partial") {
		t.Fatalf("delivery message = %#v", client)
	}
}

func TestGoogleSMTPClientUsesGoogleDefaults(t *testing.T) {
	client := newGoogleSMTPClient("", 0, "user@gmail.com", "app-password")
	if client.host != "smtp.gmail.com" || client.port != 587 {
		t.Fatalf("default SMTP configuration = %#v", client)
	}
}

func TestGoogleSMTPClientRejectsInvalidMessagesBeforeDialing(t *testing.T) {
	client := newGoogleSMTPClient("127.0.0.1", 1, "user@gmail.com", "app-password")
	if err := client.Send(context.Background(), "from\n@example.com", "to@example.com", "Subject", "body"); err == nil || !strings.Contains(err.Error(), "from contains a newline") {
		t.Fatalf("newline sender error = %v", err)
	}
	if err := client.Send(context.Background(), "from@example.com", "to@example.com", "Subject\r", "body"); err == nil || !strings.Contains(err.Error(), "subject contains a newline") {
		t.Fatalf("newline subject error = %v", err)
	}
	if err := client.Send(context.Background(), "from@example.com", "to\n@example.com", "Subject", "body"); err == nil || !strings.Contains(err.Error(), "to contains a newline") {
		t.Fatalf("newline recipient error = %v", err)
	}
}

func TestGoogleSMTPClientRejectsMissingCredentials(t *testing.T) {
	client := newGoogleSMTPClient("smtp.gmail.com", 587, "", "")
	if err := client.Send(context.Background(), "from@example.com", "to@example.com", "Subject", "body"); err == nil || !strings.Contains(err.Error(), "credentials are not configured") {
		t.Fatalf("missing credentials error = %v", err)
	}
}

func TestGoogleSMTPClientReportsConnectionFailure(t *testing.T) {
	client := newGoogleSMTPClient("127.0.0.1", 1, "user@gmail.com", "app-password")
	if err := client.Send(context.Background(), "from@example.com", "to@example.com", "Subject", "body"); err == nil {
		t.Fatal("connection failure was not returned")
	}
}

func TestGoogleSMTPClientReportsInvalidServerGreeting(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		connection, acceptErr := listener.Accept()
		if acceptErr == nil {
			_, _ = connection.Write([]byte("invalid SMTP greeting\r\n"))
			_ = connection.Close()
		}
	}()
	host, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	client := newGoogleSMTPClient(host, mustPort(t, port), "user@gmail.com", "app-password")
	if err := client.Send(context.Background(), "from@example.com", "to@example.com", "Subject", "body"); err == nil {
		t.Fatal("invalid greeting was accepted")
	}
}

func mustPort(t *testing.T, value string) int {
	t.Helper()
	port, err := strconv.Atoi(value)
	if err != nil {
		t.Fatal(err)
	}
	return port
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

func TestNewReportReadyNotifierDefaultsSenderToSMTPUsername(t *testing.T) {
	notifier := NewReportReadyNotifier(config.EmailConfig{SMTPUsername: "user@gmail.com", SMTPPassword: "app-password"}, nil)
	configured, ok := notifier.(*ReportReadyNotifier)
	if !ok || configured.from != "user@gmail.com" {
		t.Fatalf("configured sender = %#v", notifier)
	}
}

func TestResolveTemplatePathFallsBackForMissingPath(t *testing.T) {
	path := "missing/report-ready-email.txt"
	if got := resolveTemplatePath(path); got != path {
		t.Fatalf("resolveTemplatePath() = %q, want %q", got, path)
	}
}

func TestResolveTemplatePathKeepsAbsolutePath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "report.txt")
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

func TestReportReadyNotifierSendsSameBodyToPairedTelegramChat(t *testing.T) {
	dir := t.TempDir()
	templatePath := filepath.Join(dir, "report.txt")
	if err := os.WriteFile(templatePath, []byte("Sheet: <google_sheet_link>"), 0o600); err != nil {
		t.Fatal(err)
	}
	email := &fakeSMTPSender{}
	telegram := &fakeReportReadyTelegram{}
	notifier := &ReportReadyNotifier{
		client: email, from: "reports@example.com", template: templatePath, telegram: telegram,
		recipients: fakeReportRecipients{
			people:   []teacher.Recipient{{ID: 1, Email: "admin@example.com", Status: teacher.Active}},
			channels: map[int64][]teacher.Channel{1: {{Kind: "telegram", Destination: "chat-1", Status: teacher.Active}}},
		},
	}
	if err := notifier.Notify(context.Background(), report.Result{}, "https://sheet.example"); err != nil {
		t.Fatal(err)
	}
	if email.body != "Sheet: https://sheet.example" || telegram.message != email.body {
		t.Fatalf("email=%q telegram=%q", email.body, telegram.message)
	}
}

func TestReportReadyNotifierSendsOnlyToClassroomAdministrator(t *testing.T) {
	dir := t.TempDir()
	templatePath := filepath.Join(dir, "report.txt")
	if err := os.WriteFile(templatePath, []byte("<google_sheet_link>"), 0o600); err != nil {
		t.Fatal(err)
	}
	email := &fakeSMTPSender{}
	telegram := &fakeReportReadyTelegram{}
	notifier := &ReportReadyNotifier{
		client: email, from: "reports@example.com", template: templatePath, telegram: telegram,
		profile: fakeReportReadyProfile{classroomID: "admin"},
		recipients: fakeReportRecipients{
			people: []teacher.Recipient{
				{ID: 1, ClassroomID: "student", Email: "student@example.com", Status: teacher.Active},
				{ID: 2, ClassroomID: "admin", Email: "admin@example.com", Status: teacher.Active},
			},
			channels: map[int64][]teacher.Channel{2: {{Kind: "telegram", Destination: "admin-chat", Status: teacher.Active}}},
		},
	}
	if err := notifier.Notify(context.Background(), report.Result{}, "link"); err != nil {
		t.Fatal(err)
	}
	if email.to != "admin@example.com" || telegram.destination != "admin-chat" {
		t.Fatalf("email recipient=%q Telegram destination=%q", email.to, telegram.destination)
	}
}

func TestReportReadyNotifierReportsAdministratorLookupFailure(t *testing.T) {
	dir := t.TempDir()
	templatePath := filepath.Join(dir, "report.txt")
	if err := os.WriteFile(templatePath, []byte("<google_sheet_link>"), 0o600); err != nil {
		t.Fatal(err)
	}
	notifier := &ReportReadyNotifier{
		client: &fakeSMTPSender{}, from: "reports@example.com", template: templatePath,
		profile: failingReportReadyProfile{}, recipients: fakeReportRecipients{people: []teacher.Recipient{{ID: 1, Status: teacher.Active}}},
	}
	if err := notifier.Notify(context.Background(), report.Result{}, "link"); err == nil || !strings.Contains(err.Error(), "resolve report administrator") {
		t.Fatalf("Notify() error = %v", err)
	}
}

func TestNewReportReadyNotifierWithTelegramUsesConfiguredProfile(t *testing.T) {
	notifier := NewReportReadyNotifierWithTelegramAndProfile(
		config.EmailConfig{SMTPUsername: "user@gmail.com", SMTPPassword: "app-password"},
		fakeReportRecipients{}, &fakeReportReadyTelegram{}, fakeReportReadyProfile{classroomID: "admin"},
	)
	if _, ok := notifier.(*ReportReadyNotifier); !ok {
		t.Fatalf("notifier type = %T", notifier)
	}
}

func TestReportReadyNotifierSkipsTelegramWithoutPairing(t *testing.T) {
	dir := t.TempDir()
	templatePath := filepath.Join(dir, "report.txt")
	if err := os.WriteFile(templatePath, []byte("<google_sheet_link>"), 0o600); err != nil {
		t.Fatal(err)
	}
	telegram := &fakeReportReadyTelegram{}
	notifier := &ReportReadyNotifier{
		client: &fakeSMTPSender{}, from: "reports@example.com", template: templatePath, telegram: telegram,
		recipients: fakeReportRecipients{people: []teacher.Recipient{{ID: 1, Email: "admin@example.com", Status: teacher.Active}}},
	}
	if err := notifier.Notify(context.Background(), report.Result{}, "link"); err != nil {
		t.Fatal(err)
	}
	if telegram.message != "" {
		t.Fatalf("unexpected Telegram message %q", telegram.message)
	}
}

func TestReportReadyNotifierReturnsTelegramError(t *testing.T) {
	dir := t.TempDir()
	templatePath := filepath.Join(dir, "report.txt")
	if err := os.WriteFile(templatePath, []byte("<google_sheet_link>"), 0o600); err != nil {
		t.Fatal(err)
	}
	notifier := &ReportReadyNotifier{
		client: &fakeSMTPSender{}, from: "reports@example.com", template: templatePath,
		telegram: &fakeReportReadyTelegram{err: errors.New("telegram unavailable")},
		recipients: fakeReportRecipients{
			people:   []teacher.Recipient{{ID: 1, Email: "admin@example.com", Status: teacher.Active}},
			channels: map[int64][]teacher.Channel{1: {{Kind: "telegram", Destination: "chat-1", Status: teacher.Active}}},
		},
	}
	if err := notifier.Notify(context.Background(), report.Result{}, "link"); err == nil || !strings.Contains(err.Error(), "send report Telegram message") {
		t.Fatalf("Notify() error = %v", err)
	}
}

func TestReportReadyNotifierReturnsChannelLookupError(t *testing.T) {
	dir := t.TempDir()
	templatePath := filepath.Join(dir, "report.txt")
	if err := os.WriteFile(templatePath, []byte("<google_sheet_link>"), 0o600); err != nil {
		t.Fatal(err)
	}
	notifier := &ReportReadyNotifier{
		client: &fakeSMTPSender{}, from: "reports@example.com", template: templatePath,
		telegram:   &fakeReportReadyTelegram{},
		recipients: fakeReportRecipients{people: []teacher.Recipient{{ID: 1, Email: "admin@example.com", Status: teacher.Active}}, channelErr: errors.New("channel lookup failed")},
	}
	if err := notifier.Notify(context.Background(), report.Result{}, "link"); err == nil || !strings.Contains(err.Error(), "list report channels") {
		t.Fatalf("Notify() error = %v", err)
	}
}

func TestPreliminaryReportNotifierSendsMissingSummaryToEmailAndTelegram(t *testing.T) {
	email := &fakeSMTPSender{}
	telegram := &fakeReportReadyTelegram{}
	notifier := &preliminaryReportNotifier{
		client: email, from: "reports@example.com", telegram: telegram,
		recipients: fakeReportRecipients{
			people:   []teacher.Recipient{{ID: 1, Email: "admin@example.com", Status: teacher.Active}},
			channels: map[int64][]teacher.Channel{1: {{Kind: "telegram", Destination: "chat-1", Status: teacher.Active}}},
		},
	}
	item := task.Task{Title: "Task 18"}
	missing := []ports.MissingSubmitter{{Name: "Dmytro", Email: "dmytro@example.com"}}
	if err := notifier.Notify(context.Background(), item, missing); err != nil {
		t.Fatal(err)
	}
	if email.to != "admin@example.com" || !strings.Contains(email.body, "Dmytro <dmytro@example.com>") || telegram.message != email.body {
		t.Fatalf("email=%#v telegram=%q", email, telegram.message)
	}
}

func TestPreliminaryReportNotifierSaysEverybodySubmitted(t *testing.T) {
	email := &fakeSMTPSender{}
	notifier := &preliminaryReportNotifier{
		client: email, from: "reports@example.com",
		recipients: fakeReportRecipients{
			people:   []teacher.Recipient{{ID: 1, Email: "admin@example.com", Status: teacher.Active}},
			channels: map[int64][]teacher.Channel{1: {{Kind: "telegram", Destination: "chat-1", Status: teacher.Active}}},
		},
	}
	if err := notifier.Notify(context.Background(), task.Task{Title: "Task 18"}, nil); err != nil {
		t.Fatal(err)
	}
	if email.body != "Everybody has already sent the files for Task 18." {
		t.Fatalf("message=%q", email.body)
	}
}

func TestPreliminaryReportNotifierRequiresTelegramBoundAdmin(t *testing.T) {
	email := &fakeSMTPSender{}
	notifier := &preliminaryReportNotifier{client: email, recipients: fakeReportRecipients{people: []teacher.Recipient{{ID: 1, Email: "admin@example.com", Status: teacher.Active}}}}
	if err := notifier.Notify(context.Background(), task.Task{Title: "Task 18"}, nil); err != nil {
		t.Fatal(err)
	}
	if email.to != "" {
		t.Fatalf("unexpected email recipient %q", email.to)
	}
}

func TestPreliminaryReportNotifierPropagatesRecipientAndChannelErrors(t *testing.T) {
	notifier := &preliminaryReportNotifier{
		client:     &fakeSMTPSender{},
		recipients: fakeReportRecipients{err: errors.New("recipient lookup failed")},
	}
	if err := notifier.Notify(context.Background(), task.Task{Title: "Task"}, nil); err == nil || !strings.Contains(err.Error(), "list preliminary report recipients") {
		t.Fatalf("recipient error = %v", err)
	}
	notifier.recipients = fakeReportRecipients{
		people:     []teacher.Recipient{{ID: 1, Email: "admin@example.com", Status: teacher.Active}},
		channelErr: errors.New("channel lookup failed"),
	}
	if err := notifier.Notify(context.Background(), task.Task{Title: "Task"}, nil); err == nil || !strings.Contains(err.Error(), "list preliminary report channels") {
		t.Fatalf("channel error = %v", err)
	}
}

func TestPreliminaryReportNotifierPropagatesEmailError(t *testing.T) {
	notifier := &preliminaryReportNotifier{
		client: &fakeSMTPSender{err: errors.New("SMTP unavailable")},
		recipients: fakeReportRecipients{
			people:   []teacher.Recipient{{ID: 1, Email: "admin@example.com", Status: teacher.Active}},
			channels: map[int64][]teacher.Channel{1: {{Kind: "telegram", Destination: "chat-1", Status: teacher.Active}}},
		},
	}
	if err := notifier.Notify(context.Background(), task.Task{Title: "Task"}, nil); err == nil || !strings.Contains(err.Error(), "send preliminary summary email") {
		t.Fatalf("email error = %v", err)
	}
}

func TestNewPreliminaryReportNotifierDisablesWithoutSMTP(t *testing.T) {
	notifier := NewPreliminaryReportNotifier(config.EmailConfig{}, nil, nil)
	if err := notifier.Notify(context.Background(), task.Task{Title: "Task"}, nil); err != nil {
		t.Fatal(err)
	}
}

func TestReportReadyNotifierRejectsTemplateWithoutSheetLink(t *testing.T) {
	templatePath := filepath.Join(t.TempDir(), "report.txt")
	if err := os.WriteFile(templatePath, []byte("missing link"), 0o600); err != nil {
		t.Fatal(err)
	}
	notifier := &ReportReadyNotifier{template: templatePath}
	if _, err := notifier.renderMessage("link"); err == nil || !strings.Contains(err.Error(), "must contain") {
		t.Fatalf("renderMessage() error = %v", err)
	}
}

func TestReportReadyNotifierReportsMissingTemplate(t *testing.T) {
	notifier := &ReportReadyNotifier{template: filepath.Join(t.TempDir(), "missing.txt")}
	if _, err := notifier.renderMessage("link"); err == nil || !strings.Contains(err.Error(), "read report email template") {
		t.Fatalf("renderMessage() error = %v", err)
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

func TestReportReadyNotifierSkipsDisabledAndEmaillessRecipients(t *testing.T) {
	client := &fakeSMTPSender{}
	templatePath := filepath.Join(t.TempDir(), "report.txt")
	if err := os.WriteFile(templatePath, []byte("<google_sheet_link>"), 0o600); err != nil {
		t.Fatal(err)
	}
	notifier := &ReportReadyNotifier{client: client, from: "reports@example.com", template: templatePath, recipients: fakeReportRecipients{
		people: []teacher.Recipient{
			{ID: 1, Email: "", Status: teacher.Active},
			{ID: 2, Email: "disabled@example.com", Status: teacher.Disabled},
		},
	}}
	if err := notifier.Notify(context.Background(), report.Result{}, "link"); err != nil {
		t.Fatal(err)
	}
}

func TestReportReadyNotifierReturnsRecipientLookupError(t *testing.T) {
	templatePath := filepath.Join(t.TempDir(), "report.txt")
	if err := os.WriteFile(templatePath, []byte("<google_sheet_link>"), 0o600); err != nil {
		t.Fatal(err)
	}
	notifier := &ReportReadyNotifier{client: &fakeSMTPSender{}, from: "reports@example.com", template: templatePath, recipients: fakeReportRecipients{err: errors.New("lookup failed")}}
	if err := notifier.Notify(context.Background(), report.Result{}, "link"); err == nil || !strings.Contains(err.Error(), "list report recipients") {
		t.Fatalf("Notify() error = %v", err)
	}
}

func TestReportReadyNotifierAllowsMissingRecipientRepository(t *testing.T) {
	templatePath := filepath.Join(t.TempDir(), "report.txt")
	if err := os.WriteFile(templatePath, []byte("<google_sheet_link>"), 0o600); err != nil {
		t.Fatal(err)
	}
	notifier := &ReportReadyNotifier{client: &fakeSMTPSender{}, from: "reports@example.com", template: templatePath}
	if err := notifier.Notify(context.Background(), report.Result{}, "link"); err != nil {
		t.Fatal(err)
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
