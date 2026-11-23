package notification

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/alex/assistent/internal/domain/notification"
	"github.com/alex/assistent/internal/domain/ports"
	"github.com/alex/assistent/internal/domain/report"
	"github.com/alex/assistent/internal/domain/scenario"
	"github.com/alex/assistent/internal/domain/task"
	"github.com/alex/assistent/internal/domain/teacher"
	"github.com/alex/assistent/internal/platform/config"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type reportGinkgoTest struct{}

func TestNotificationSpecs(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Notification")
}

func (*reportGinkgoTest) TempDir() string                   { return GinkgoT().TempDir() }
func (*reportGinkgoTest) Setenv(key, value string)          { GinkgoT().Setenv(key, value) }
func (*reportGinkgoTest) Cleanup(fn func())                 { DeferCleanup(fn) }
func (*reportGinkgoTest) Helper()                           {}
func (*reportGinkgoTest) Fatal(args ...any)                 { Fail(fmt.Sprint(args...)) }
func (*reportGinkgoTest) Fatalf(format string, args ...any) { Fail(fmt.Sprintf(format, args...)) }
func (*reportGinkgoTest) Errorf(format string, args ...any) { Fail(fmt.Sprintf(format, args...)) }
func (*reportGinkgoTest) Run(name string, fn func(*reportGinkgoTest)) bool {
	By(name)
	fn(&reportGinkgoTest{})
	return true
}

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

func testEmailProvidersAreDisabledWithoutSMTPCredentials(t *reportGinkgoTest) {
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

func testEmailProvidersUseGoogleSMTPConfiguration(t *reportGinkgoTest) {
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

func testEmailProvidersDefaultSenderToSMTPUsername(t *reportGinkgoTest) {
	cfg := config.EmailConfig{SMTPUsername: "user@gmail.com", SMTPPassword: "app-password"}
	if provider := NewReminderEmailProvider(cfg).(*reminderEmailProvider); provider.from != cfg.SMTPUsername {
		t.Fatalf("reminder sender = %q", provider.from)
	}
	if provider := NewReportDeliveryEmailProvider(cfg).(*reportDeliveryEmailProvider); provider.from != cfg.SMTPUsername {
		t.Fatalf("delivery sender = %q", provider.from)
	}
}

func testEmailProvidersSendUsingTheirSMTPClients(t *reportGinkgoTest) {
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

func testGoogleSMTPClientUsesGoogleDefaults(t *reportGinkgoTest) {
	client := newGoogleSMTPClient("", 0, "user@gmail.com", "app-password")
	if client.host != "smtp.gmail.com" || client.port != 587 {
		t.Fatalf("default SMTP configuration = %#v", client)
	}
}

func testGoogleSMTPClientRejectsInvalidMessagesBeforeDialing(t *reportGinkgoTest) {
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

func testGoogleSMTPClientRejectsMissingCredentials(t *reportGinkgoTest) {
	client := newGoogleSMTPClient("smtp.gmail.com", 587, "", "")
	if err := client.Send(context.Background(), "from@example.com", "to@example.com", "Subject", "body"); err == nil || !strings.Contains(err.Error(), "credentials are not configured") {
		t.Fatalf("missing credentials error = %v", err)
	}
}

func testGoogleSMTPClientReportsConnectionFailure(t *reportGinkgoTest) {
	client := newGoogleSMTPClient("127.0.0.1", 1, "user@gmail.com", "app-password")
	if err := client.Send(context.Background(), "from@example.com", "to@example.com", "Subject", "body"); err == nil {
		t.Fatal("connection failure was not returned")
	}
}

func testGoogleSMTPClientReportsInvalidServerGreeting(t *reportGinkgoTest) {
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

func testGoogleSMTPClientSendsMessageOverSTARTTLS(t *reportGinkgoTest) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	done := make(chan struct{})
	go func() {
		connection, acceptErr := listener.Accept()
		if acceptErr != nil {
			close(done)
			return
		}
		defer connection.Close()
		_, _ = connection.Write([]byte("220 smtp.test ESMTP\r\n"))
		buffer := make([]byte, 512)
		_, _ = connection.Read(buffer)
		_, _ = connection.Write([]byte("250-smtp.test\r\n250-STARTTLS\r\n250 AUTH PLAIN\r\n"))
		_, _ = connection.Read(buffer)
		_, _ = connection.Write([]byte("220 ready for TLS\r\n"))
		close(done)
	}()
	host, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	client := newGoogleSMTPClient(host, mustPort(t, port), "user@gmail.com", "app-password")
	if err := client.Send(context.Background(), "from@example.com", "to@example.com", "Subject", "body"); err == nil {
		t.Fatal("TLS handshake failure was not returned")
	}
	<-done
}

func testGoogleSMTPClientCompletesSMTPTransaction(t *reportGinkgoTest) {
	certificate := makeSMTPCertificate(t)
	clientConn, serverConn := net.Pipe()
	done := make(chan error, 1)
	go func() {
		defer serverConn.Close()
		write := func(connection net.Conn, value string) { _, _ = fmt.Fprint(connection, value+"\r\n") }
		write(serverConn, "220 smtp.test ESMTP")
		reader := bufio.NewReader(serverConn)
		line, err := reader.ReadString('\n')
		if err != nil || !strings.HasPrefix(line, "EHLO") {
			done <- fmt.Errorf("EHLO: %v", err)
			return
		}
		write(serverConn, "250-smtp.test")
		write(serverConn, "250-STARTTLS")
		write(serverConn, "250 AUTH PLAIN")
		line, err = reader.ReadString('\n')
		if err != nil || !strings.HasPrefix(line, "STARTTLS") {
			done <- fmt.Errorf("STARTTLS: %v", err)
			return
		}
		write(serverConn, "220 ready")
		tlsConn := tls.Server(serverConn, &tls.Config{Certificates: []tls.Certificate{certificate}})
		if err := tlsConn.Handshake(); err != nil {
			done <- err
			return
		}
		reader = bufio.NewReader(tlsConn)
		line, err = reader.ReadString('\n')
		if err != nil || !strings.HasPrefix(line, "EHLO") {
			done <- fmt.Errorf("TLS EHLO: %v", err)
			return
		}
		write(tlsConn, "250-smtp.test")
		write(tlsConn, "250 AUTH PLAIN")
		for {
			line, err = reader.ReadString('\n')
			if err != nil {
				done <- err
				return
			}
			switch {
			case strings.HasPrefix(line, "AUTH "):
				write(tlsConn, "235 authenticated")
			case strings.HasPrefix(line, "MAIL FROM:"):
				write(tlsConn, "250 sender")
			case strings.HasPrefix(line, "RCPT TO:"):
				write(tlsConn, "250 recipient")
			case strings.HasPrefix(line, "DATA"):
				write(tlsConn, "354 continue")
				for {
					line, err = reader.ReadString('\n')
					if err != nil {
						done <- err
						return
					}
					if line == ".\r\n" {
						break
					}
				}
				write(tlsConn, "250 queued")
				done <- nil
				return
			}
		}
	}()
	client := newGoogleSMTPClient("smtp.test", 587, "user@gmail.com", "app-password")
	client.dial = func(context.Context, string) (net.Conn, error) { return clientConn, nil }
	client.tlsConfig = func() *tls.Config { return &tls.Config{ServerName: "smtp.test", InsecureSkipVerify: true} } // test certificate is intentionally self-signed.
	if err := client.Send(context.Background(), "from@example.com", "to@example.com", "Subject", "body"); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func testGoogleSMTPClientReportsSMTPCommandFailures(t *reportGinkgoTest) {
	for _, failure := range []string{"AUTH", "MAIL", "RCPT", "DATA", "WRITE"} {
		t.Run(failure, func(t *reportGinkgoTest) {
			clientConn, serverConn := net.Pipe()
			certificate := makeSMTPCertificate(t)
			done := make(chan struct{})
			go func() {
				defer close(done)
				defer serverConn.Close()
				write := func(value string) { _, _ = fmt.Fprint(serverConn, value+"\r\n") }
				write("220 smtp.test")
				reader := bufio.NewReader(serverConn)
				_, _ = reader.ReadString('\n')
				write("250-smtp.test")
				write("250-STARTTLS")
				write("250 AUTH PLAIN")
				_, _ = reader.ReadString('\n')
				write("220 ready")
				tlsConn := tls.Server(serverConn, &tls.Config{Certificates: []tls.Certificate{certificate}})
				if tlsConn.Handshake() != nil {
					return
				}
				reader = bufio.NewReader(tlsConn)
				_, _ = reader.ReadString('\n')
				writeTLS := func(value string) { _, _ = fmt.Fprint(tlsConn, value+"\r\n") }
				writeTLS("250-smtp.test")
				writeTLS("250 AUTH PLAIN")
				for {
					line, err := reader.ReadString('\n')
					if err != nil {
						return
					}
					if failure == "WRITE" && strings.HasPrefix(line, "DATA") {
						writeTLS("354 continue")
						return
					}
					if (failure == "AUTH" && strings.HasPrefix(line, "AUTH ")) || (failure == "MAIL" && strings.HasPrefix(line, "MAIL FROM:")) || (failure == "RCPT" && strings.HasPrefix(line, "RCPT TO:")) || (failure == "DATA" && strings.HasPrefix(line, "DATA")) {
						writeTLS("550 rejected")
						return
					}
					switch {
					case strings.HasPrefix(line, "AUTH "):
						writeTLS("235 authenticated")
					case strings.HasPrefix(line, "MAIL FROM:"):
						writeTLS("250 sender")
					case strings.HasPrefix(line, "RCPT TO:"):
						writeTLS("250 recipient")
					}
				}
			}()
			client := newGoogleSMTPClient("smtp.test", 587, "user@gmail.com", "app-password")
			client.dial = func(context.Context, string) (net.Conn, error) { return clientConn, nil }
			client.tlsConfig = func() *tls.Config { return &tls.Config{ServerName: "smtp.test", InsecureSkipVerify: true} }
			if err := client.Send(context.Background(), "from@example.com", "to@example.com", "Subject", "body"); err == nil {
				t.Fatal("SMTP failure was ignored")
			}
			<-done
		})
	}
}

func testGoogleSMTPClientUsesDefaultNetworkAndTLSFactories(t *reportGinkgoTest) {
	client := &googleSMTPClient{host: "127.0.0.1", port: 1, username: "user", password: "password"}
	if err := client.Send(context.Background(), "from@example.com", "to@example.com", "Subject", "body"); err == nil {
		t.Fatal("default network dial unexpectedly succeeded")
	}

	clientConn, serverConn := net.Pipe()
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer serverConn.Close()
		_, _ = fmt.Fprint(serverConn, "220 smtp.test\r\n")
		reader := bufio.NewReader(serverConn)
		_, _ = reader.ReadString('\n')
		_, _ = fmt.Fprint(serverConn, "250-smtp.test\r\n250-STARTTLS\r\n250 AUTH PLAIN\r\n")
		_, _ = reader.ReadString('\n')
		_, _ = fmt.Fprint(serverConn, "220 ready\r\n")
	}()
	client = &googleSMTPClient{host: "smtp.test", port: 587, username: "user", password: "password", dial: func(context.Context, string) (net.Conn, error) { return clientConn, nil }}
	if err := client.Send(context.Background(), "from@example.com", "to@example.com", "Subject", "body"); err == nil {
		t.Fatal("untrusted default TLS certificate unexpectedly succeeded")
	}
	<-done
}

func makeSMTPCertificate(t *reportGinkgoTest) tls.Certificate {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "smtp.test"}, DNSNames: []string{"smtp.test"}, NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

func mustPort(t *reportGinkgoTest, value string) int {
	t.Helper()
	port, err := strconv.Atoi(value)
	if err != nil {
		t.Fatal(err)
	}
	return port
}

func testNewReportReadyNotifierEnablesConfiguredSMTP(t *reportGinkgoTest) {
	notifier := NewReportReadyNotifier(config.EmailConfig{SMTPUsername: "user@gmail.com", SMTPPassword: "app-password", TemplatePath: "templates/report-ready-email.txt"}, nil)
	if !notifier.Enabled() {
		t.Fatal("configured notifier should be enabled")
	}
	configured, ok := notifier.(*ReportReadyNotifier)
	if !ok || !strings.HasSuffix(configured.template, filepath.Join("templates", "report-ready-email.txt")) {
		t.Fatalf("configured notifier = %#v", notifier)
	}
}

func testNewReportReadyNotifierDefaultsSenderToSMTPUsername(t *reportGinkgoTest) {
	notifier := NewReportReadyNotifier(config.EmailConfig{SMTPUsername: "user@gmail.com", SMTPPassword: "app-password"}, nil)
	configured, ok := notifier.(*ReportReadyNotifier)
	if !ok || configured.from != "user@gmail.com" {
		t.Fatalf("configured sender = %#v", notifier)
	}
}

func testResolveTemplatePathFallsBackForMissingPath(t *reportGinkgoTest) {
	path := "missing/report-ready-email.txt"
	if got := resolveTemplatePath(path); got != path {
		t.Fatalf("resolveTemplatePath() = %q, want %q", got, path)
	}
}

func testResolveTemplatePathKeepsAbsolutePath(t *reportGinkgoTest) {
	path := filepath.Join(t.TempDir(), "report.txt")
	if got := resolveTemplatePath(path); got != path {
		t.Fatalf("resolveTemplatePath() = %q, want %q", got, path)
	}
}

func testReportReadyNotifierRendersAndSendsThroughSMTP(t *reportGinkgoTest) {
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

func testReportReadyNotifierSendsSameBodyToPairedTelegramChat(t *reportGinkgoTest) {
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

func testReportReadyNotifierSendsOnlyToClassroomAdministrator(t *reportGinkgoTest) {
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

func testReportReadyNotifierReportsAdministratorLookupFailure(t *reportGinkgoTest) {
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

func testNewReportReadyNotifierWithTelegramUsesConfiguredProfile(t *reportGinkgoTest) {
	notifier := NewReportReadyNotifierWithTelegramAndProfile(
		config.EmailConfig{SMTPUsername: "user@gmail.com", SMTPPassword: "app-password"},
		fakeReportRecipients{}, &fakeReportReadyTelegram{}, fakeReportReadyProfile{classroomID: "admin"},
	)
	if _, ok := notifier.(*ReportReadyNotifier); !ok {
		t.Fatalf("notifier type = %T", notifier)
	}
}

func testNewReportReadyNotifierWithTelegramUsesLegacyConstructor(t *reportGinkgoTest) {
	notifier := NewReportReadyNotifierWithTelegram(
		config.EmailConfig{SMTPUsername: "user@gmail.com", SMTPPassword: "app-password"},
		fakeReportRecipients{}, &fakeReportReadyTelegram{},
	)
	if _, ok := notifier.(*ReportReadyNotifier); !ok {
		t.Fatalf("notifier type = %T", notifier)
	}
}

func testReportReadyNotifierReturnsNoRecipientWhenAdministratorIsNotSynchronized(t *reportGinkgoTest) {
	dir := t.TempDir()
	templatePath := filepath.Join(dir, "report.txt")
	if err := os.WriteFile(templatePath, []byte("<google_sheet_link>"), 0o600); err != nil {
		t.Fatal(err)
	}
	notifier := &ReportReadyNotifier{
		client: &fakeSMTPSender{}, from: "reports@example.com", template: templatePath,
		profile:    fakeReportReadyProfile{classroomID: "missing-admin"},
		recipients: fakeReportRecipients{people: []teacher.Recipient{{ID: 1, ClassroomID: "student", Status: teacher.Active}}},
	}
	if err := notifier.Notify(context.Background(), report.Result{}, "link"); err != nil {
		t.Fatal(err)
	}
}

func testReportReadyNotifierSkipsTelegramWithoutPairing(t *reportGinkgoTest) {
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

func testReportReadyNotifierReturnsTelegramError(t *reportGinkgoTest) {
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

func testReportReadyNotifierReturnsChannelLookupError(t *reportGinkgoTest) {
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

func testPreliminaryReportNotifierSendsMissingSummaryToEmailAndTelegram(t *reportGinkgoTest) {
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

func testPreliminaryReportNotifierDeduplicatesIdenticalSummaries(t *reportGinkgoTest) {
	email := &fakeSMTPSender{}
	notifier := &preliminaryReportNotifier{
		client: email, from: "reports@example.com",
		recipients: fakeReportRecipients{people: []teacher.Recipient{{ID: 1, Email: "admin@example.com", Status: teacher.Active}}},
	}
	item := task.Task{Title: "Task"}
	if err := notifier.Notify(context.Background(), item, nil); err != nil {
		t.Fatal(err)
	}
	first := email.body
	if err := notifier.Notify(context.Background(), item, nil); err != nil {
		t.Fatal(err)
	}
	if email.body != first {
		t.Fatal("duplicate summary changed the message")
	}
	key := fmt.Sprintf("%d:%s", 1, preliminaryMessage(item, nil))
	preloaded := &preliminaryReportNotifier{
		client: email, recipients: notifier.recipients, sent: map[string]struct{}{key: {}},
	}
	if err := preloaded.Notify(context.Background(), item, nil); err != nil {
		t.Fatal(err)
	}
}

func testNewPreliminaryReportNotifierWithProfileUsesConfiguredSMTP(t *reportGinkgoTest) {
	notifier := NewPreliminaryReportNotifierWithProfile(
		config.EmailConfig{SMTPUsername: "user@gmail.com", SMTPPassword: "app-password"},
		fakeReportRecipients{}, &fakeReportReadyTelegram{}, fakeReportReadyProfile{classroomID: "admin"},
	)
	if _, ok := notifier.(*preliminaryReportNotifier); !ok {
		t.Fatalf("notifier type = %T", notifier)
	}
}

func testPreliminaryReportNotifierReportsAdministratorLookupFailure(t *reportGinkgoTest) {
	notifier := &preliminaryReportNotifier{
		client: &fakeSMTPSender{}, recipients: fakeReportRecipients{people: []teacher.Recipient{{ID: 1, Status: teacher.Active}}}, profile: failingReportReadyProfile{},
	}
	if err := notifier.Notify(context.Background(), task.Task{Title: "Task"}, nil); err == nil || !strings.Contains(err.Error(), "resolve preliminary report administrator") {
		t.Fatalf("Notify() error = %v", err)
	}
}

func testPreliminaryReportNotifierSaysEverybodySubmitted(t *reportGinkgoTest) {
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

func testPreliminaryReportNotifierRequiresTelegramBoundAdmin(t *reportGinkgoTest) {
	email := &fakeSMTPSender{}
	notifier := &preliminaryReportNotifier{client: email, recipients: fakeReportRecipients{people: []teacher.Recipient{{ID: 1, Email: "admin@example.com", Status: teacher.Active}}}}
	if err := notifier.Notify(context.Background(), task.Task{Title: "Task 18"}, nil); err != nil {
		t.Fatal(err)
	}
	if email.to != "" {
		t.Fatalf("unexpected email recipient %q", email.to)
	}
}

func testPreliminaryReportNotifierSkipsMissingAdministratorAndDuplicateMessages(t *reportGinkgoTest) {
	email := &fakeSMTPSender{}
	notifier := &preliminaryReportNotifier{client: email, recipients: fakeReportRecipients{people: []teacher.Recipient{{ID: 1, Email: "inactive@example.com", Status: teacher.Disabled}}}}
	if err := notifier.Notify(context.Background(), task.Task{Title: "Task 18"}, nil); err != nil {
		t.Fatal(err)
	}
	if email.to != "" {
		t.Fatalf("inactive administrator received email: %q", email.to)
	}

	notifier = &preliminaryReportNotifier{client: email, from: "reports@example.com", telegram: &fakeReportReadyTelegram{}, recipients: fakeReportRecipients{
		people:   []teacher.Recipient{{ID: 1, Email: "admin@example.com", Status: teacher.Active}},
		channels: map[int64][]teacher.Channel{1: {{Kind: "telegram", Destination: ""}, {Kind: "email", Destination: "admin@example.com"}}},
	}}
	if err := notifier.Notify(context.Background(), task.Task{Title: "Task 18"}, nil); err != nil {
		t.Fatal(err)
	}
	first := email.body
	if err := notifier.Notify(context.Background(), task.Task{Title: "Task 18"}, nil); err != nil {
		t.Fatal(err)
	}
	if email.body != first {
		t.Fatal("duplicate notification changed the message")
	}
}

func testPreliminaryReportNotifierPropagatesRecipientAndChannelErrors(t *reportGinkgoTest) {
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

func testPreliminaryReportNotifierPropagatesEmailError(t *reportGinkgoTest) {
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

func testPreliminaryReportNotifierUsesClassroomAdministratorAndReportsTelegramFailure(t *reportGinkgoTest) {
	notifier := &preliminaryReportNotifier{
		client: &fakeSMTPSender{}, from: "reports@example.com", telegram: &fakeReportReadyTelegram{err: errors.New("telegram failed")}, profile: fakeReportReadyProfile{classroomID: "admin"},
		recipients: fakeReportRecipients{people: []teacher.Recipient{{ID: 1, Email: "admin@example.com", ClassroomID: "admin", Status: teacher.Active}}},
	}
	if err := notifier.Notify(context.Background(), task.Task{Title: "Task"}, nil); err == nil || !strings.Contains(err.Error(), "Telegram") {
		t.Fatalf("Telegram error = %v", err)
	}
	noChat := &preliminaryReportNotifier{client: &fakeSMTPSender{}, from: "reports@example.com", telegram: &fakeReportReadyTelegram{}, profile: fakeReportReadyProfile{classroomID: "admin"}, recipients: fakeReportRecipients{people: []teacher.Recipient{{ID: 1, Email: "admin@example.com", ClassroomID: "admin", Status: teacher.Active}}}}
	if err := noChat.Notify(context.Background(), task.Task{Title: "Task"}, nil); err != nil {
		t.Fatal(err)
	}
}

func testNewPreliminaryReportNotifierDisablesWithoutSMTP(t *reportGinkgoTest) {
	notifier := NewPreliminaryReportNotifier(config.EmailConfig{}, nil, nil)
	if err := notifier.Notify(context.Background(), task.Task{Title: "Task"}, nil); err != nil {
		t.Fatal(err)
	}
}

func testPreliminaryReportNotifierTreatsMissingRecipientsAsDisabled(t *reportGinkgoTest) {
	notifier := &preliminaryReportNotifier{client: &fakeSMTPSender{}}
	if err := notifier.Notify(context.Background(), task.Task{Title: "Task"}, nil); err != nil {
		t.Fatalf("Notify() error = %v", err)
	}
}

func testReportReadyNotifierRejectsTemplateWithoutSheetLink(t *reportGinkgoTest) {
	templatePath := filepath.Join(t.TempDir(), "report.txt")
	if err := os.WriteFile(templatePath, []byte("missing link"), 0o600); err != nil {
		t.Fatal(err)
	}
	notifier := &ReportReadyNotifier{template: templatePath}
	if _, err := notifier.renderMessage("link"); err == nil || !strings.Contains(err.Error(), "must contain") {
		t.Fatalf("renderMessage() error = %v", err)
	}
}

func testReportReadyNotifierReportsMissingTemplate(t *reportGinkgoTest) {
	notifier := &ReportReadyNotifier{template: filepath.Join(t.TempDir(), "missing.txt")}
	if _, err := notifier.renderMessage("link"); err == nil || !strings.Contains(err.Error(), "read report email template") {
		t.Fatalf("renderMessage() error = %v", err)
	}
}

func testReportReadyNotifierReturnsSMTPError(t *reportGinkgoTest) {
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

func testReportReadyNotifierSkipsDisabledAndEmaillessRecipients(t *reportGinkgoTest) {
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

func testReportReadyNotifierReturnsRecipientLookupError(t *reportGinkgoTest) {
	templatePath := filepath.Join(t.TempDir(), "report.txt")
	if err := os.WriteFile(templatePath, []byte("<google_sheet_link>"), 0o600); err != nil {
		t.Fatal(err)
	}
	notifier := &ReportReadyNotifier{client: &fakeSMTPSender{}, from: "reports@example.com", template: templatePath, recipients: fakeReportRecipients{err: errors.New("lookup failed")}}
	if err := notifier.Notify(context.Background(), report.Result{}, "link"); err == nil || !strings.Contains(err.Error(), "list report recipients") {
		t.Fatalf("Notify() error = %v", err)
	}
}

func testReportReadyNotifierAllowsMissingRecipientRepository(t *reportGinkgoTest) {
	templatePath := filepath.Join(t.TempDir(), "report.txt")
	if err := os.WriteFile(templatePath, []byte("<google_sheet_link>"), 0o600); err != nil {
		t.Fatal(err)
	}
	notifier := &ReportReadyNotifier{client: &fakeSMTPSender{}, from: "reports@example.com", template: templatePath}
	if err := notifier.Notify(context.Background(), report.Result{}, "link"); err != nil {
		t.Fatal(err)
	}
}

func testTelegramProviderUsesConfiguredEndpoint(t *reportGinkgoTest) {
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

var _ = Describe("report notification tests", func() {
	It("runs all report notification cases", func() {
		t := &reportGinkgoTest{}
		testEmailProvidersAreDisabledWithoutSMTPCredentials(t)
		testEmailProvidersUseGoogleSMTPConfiguration(t)
		testEmailProvidersDefaultSenderToSMTPUsername(t)
		testEmailProvidersSendUsingTheirSMTPClients(t)
		testGoogleSMTPClientUsesGoogleDefaults(t)
		testGoogleSMTPClientRejectsInvalidMessagesBeforeDialing(t)
		testGoogleSMTPClientRejectsMissingCredentials(t)
		testGoogleSMTPClientReportsConnectionFailure(t)
		testGoogleSMTPClientReportsInvalidServerGreeting(t)
		testGoogleSMTPClientSendsMessageOverSTARTTLS(t)
		testGoogleSMTPClientCompletesSMTPTransaction(t)
		testGoogleSMTPClientReportsSMTPCommandFailures(t)
		testGoogleSMTPClientUsesDefaultNetworkAndTLSFactories(t)
		testNewReportReadyNotifierEnablesConfiguredSMTP(t)
		testNewReportReadyNotifierDefaultsSenderToSMTPUsername(t)
		testResolveTemplatePathFallsBackForMissingPath(t)
		testResolveTemplatePathKeepsAbsolutePath(t)
		testReportReadyNotifierRendersAndSendsThroughSMTP(t)
		testReportReadyNotifierSendsSameBodyToPairedTelegramChat(t)
		testReportReadyNotifierSendsOnlyToClassroomAdministrator(t)
		testReportReadyNotifierReportsAdministratorLookupFailure(t)
		testNewReportReadyNotifierWithTelegramUsesConfiguredProfile(t)
		testNewReportReadyNotifierWithTelegramUsesLegacyConstructor(t)
		testReportReadyNotifierReturnsNoRecipientWhenAdministratorIsNotSynchronized(t)
		testReportReadyNotifierSkipsTelegramWithoutPairing(t)
		testReportReadyNotifierReturnsTelegramError(t)
		testReportReadyNotifierReturnsChannelLookupError(t)
		testPreliminaryReportNotifierSendsMissingSummaryToEmailAndTelegram(t)
		testPreliminaryReportNotifierDeduplicatesIdenticalSummaries(t)
		testNewPreliminaryReportNotifierWithProfileUsesConfiguredSMTP(t)
		testPreliminaryReportNotifierReportsAdministratorLookupFailure(t)
		testPreliminaryReportNotifierSaysEverybodySubmitted(t)
		testPreliminaryReportNotifierRequiresTelegramBoundAdmin(t)
		testPreliminaryReportNotifierSkipsMissingAdministratorAndDuplicateMessages(t)
		testPreliminaryReportNotifierPropagatesRecipientAndChannelErrors(t)
		testPreliminaryReportNotifierPropagatesEmailError(t)
		testPreliminaryReportNotifierUsesClassroomAdministratorAndReportsTelegramFailure(t)
		testNewPreliminaryReportNotifierDisablesWithoutSMTP(t)
		testPreliminaryReportNotifierTreatsMissingRecipientsAsDisabled(t)
		testReportReadyNotifierRejectsTemplateWithoutSheetLink(t)
		testReportReadyNotifierReportsMissingTemplate(t)
		testReportReadyNotifierReturnsSMTPError(t)
		testReportReadyNotifierSkipsDisabledAndEmaillessRecipients(t)
		testReportReadyNotifierReturnsRecipientLookupError(t)
		testReportReadyNotifierAllowsMissingRecipientRepository(t)
		testTelegramProviderUsesConfiguredEndpoint(t)
	})
})
