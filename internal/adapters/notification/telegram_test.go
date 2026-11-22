package notification

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alex/assistent/internal/domain/notification"
	"github.com/alex/assistent/internal/domain/report"
	"github.com/alex/assistent/internal/domain/scenario"
	"github.com/alex/assistent/internal/platform/config"
)

func TestTelegramProviderSendsReminderAndDelivery(t *testing.T) {
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/botsecret/sendMessage") {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	provider := NewTelegramProvider(config.TelegramConfig{Token: "secret", BaseURL: server.URL})
	if provider.Name() != "telegram" {
		t.Fatalf("Name() = %q", provider.Name())
	}
	reminder := notification.Reminder{Destination: "chat", Subject: "subject", Message: "message"}
	if err := provider.SendReminder(context.Background(), reminder); err != nil {
		t.Fatal(err)
	}
	if err := provider.Deliver(context.Background(), reportResult(), "chat"); err != nil {
		t.Fatal(err)
	}
	if err := provider.SendReportReady(context.Background(), "chat", "same report message"); err != nil {
		t.Fatal(err)
	}
	if requests != 3 {
		t.Fatalf("requests = %d, want 3", requests)
	}
}

func TestTelegramProviderSendsReportReadyThroughConfiguredChatID(t *testing.T) {
	var requestBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		requestBody = string(body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	provider := NewTelegramProvider(config.TelegramConfig{Token: "secret", BaseURL: server.URL, ChatID: "admin-chat"})
	if err := provider.SendReportReady(context.Background(), "", "Sheet: https://sheet.example"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(requestBody, `"chat_id":"admin-chat"`) || !strings.Contains(requestBody, `Sheet: https://sheet.example`) {
		t.Fatalf("request body = %s", requestBody)
	}
}

func TestTelegramProviderSendsDirectMessage(t *testing.T) {
	var requestBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		requestBody = string(body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	provider := NewTelegramProvider(config.TelegramConfig{Token: "secret", BaseURL: server.URL})
	if err := provider.SendMessage(context.Background(), "chat-7", "connected"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(requestBody, `"chat_id":"chat-7"`) || !strings.Contains(requestBody, "connected") {
		t.Fatalf("request body = %s", requestBody)
	}
}

func TestTelegramProviderUsesConfiguredChatIDWhenDestinationIsEmpty(t *testing.T) {
	var requestBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		requestBody = string(body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	provider := NewTelegramProvider(config.TelegramConfig{Token: "secret", BaseURL: server.URL, ChatID: "-1001234567890"})
	if err := provider.SendReminder(context.Background(), notification.Reminder{Subject: "subject", Message: "message"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(requestBody, `"chat_id":"-1001234567890"`) {
		t.Fatalf("request body = %s", requestBody)
	}
}

func reportResult() report.Result {
	return report.Result{Data: scenario.ReportData{TaskID: 42}}
}

func TestTelegramProviderRejectsMissingTokenAndHTTPFailure(t *testing.T) {
	provider := NewTelegramProvider(config.TelegramConfig{})
	if err := provider.SendReminder(context.Background(), notification.Reminder{}); err == nil {
		t.Fatal("missing token unexpectedly succeeded")
	}
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	server.Close()
	provider = NewTelegramProvider(config.TelegramConfig{Token: "secret", BaseURL: server.URL})
	if err := provider.SendReminder(context.Background(), notification.Reminder{}); err == nil {
		t.Fatal("closed server unexpectedly succeeded")
	}
}

func TestTelegramProviderReadsPrivateMessagesAndContacts(t *testing.T) {
	var requestBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/botsecret/getUpdates" {
			if r.URL.Query().Get("offset") != "7" || r.URL.Query().Get("timeout") != "10" {
				t.Fatalf("query=%s", r.URL.RawQuery)
			}
			_, _ = io.WriteString(w, `{"ok":true,"result":[{"update_id":7,"message":{"text":"/start pair_token","chat":{"id":42,"type":"private"}}}]}`)
			return
		}
		body, _ := io.ReadAll(r.Body)
		requestBody = string(body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	provider := NewTelegramProvider(config.TelegramConfig{Token: "secret", BaseURL: server.URL})
	updates, err := provider.GetUpdates(context.Background(), 7, 10)
	if err != nil || len(updates) != 1 || updates[0].ChatID != "42" || updates[0].Text != "/start pair_token" {
		t.Fatalf("updates=%#v err=%v", updates, err)
	}
	if requestBody != "" {
		t.Fatalf("unexpected send request body=%s", requestBody)
	}
}

func TestTelegramProviderRejectsUnsuccessfulUpdatesResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"ok":false,"result":[]}`)
	}))
	defer server.Close()
	provider := NewTelegramProvider(config.TelegramConfig{Token: "secret", BaseURL: server.URL})
	if _, err := provider.GetUpdates(context.Background(), 0, 1); err == nil {
		t.Fatal("unsuccessful Telegram response unexpectedly succeeded")
	}
}

func TestTelegramProviderRejectsMalformedUpdatesAndMarkup(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/botsecret/getUpdates" {
			_, _ = io.WriteString(w, "not-json")
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	provider := NewTelegramProvider(config.TelegramConfig{Token: "secret", BaseURL: server.URL})
	if _, err := provider.GetUpdates(context.Background(), 0, 1); err == nil {
		t.Fatal("malformed updates unexpectedly succeeded")
	}
	if err := provider.sendWithMarkup(context.Background(), "chat", "message", "not-json"); err == nil {
		t.Fatal("malformed markup unexpectedly succeeded")
	}
}

func TestTelegramProviderSendsReplyMarkupAndRejectsMissingUpdateToken(t *testing.T) {
	var body string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		payload, _ := io.ReadAll(r.Body)
		body = string(payload)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	provider := NewTelegramProvider(config.TelegramConfig{Token: "secret", BaseURL: server.URL})
	if err := provider.sendWithMarkup(context.Background(), "chat", "message", `{"inline_keyboard":[]}`); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body, "reply_markup") {
		t.Fatalf("markup payload = %s", body)
	}
	if _, err := NewTelegramProvider(config.TelegramConfig{}).GetUpdates(context.Background(), 0, 1); err == nil {
		t.Fatal("missing token unexpectedly succeeded")
	}
}

func TestTelegramProviderRejectsCanceledRequests(t *testing.T) {
	provider := NewTelegramProvider(config.TelegramConfig{Token: "secret", BaseURL: "http://127.0.0.1:1"})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := provider.SendMessage(ctx, "chat", "message"); err == nil {
		t.Fatal("canceled request unexpectedly succeeded")
	}
}

func TestTelegramProviderReportsTelegramHTTPStatusFailures(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusMethodNotAllowed)
	}))
	defer server.Close()
	provider := NewTelegramProvider(config.TelegramConfig{Token: "secret", BaseURL: server.URL})
	if _, err := provider.GetUpdates(context.Background(), 0, 1); err == nil || !strings.Contains(err.Error(), "405") {
		t.Fatalf("GetUpdates() error = %v", err)
	}
	if err := provider.SendMessage(context.Background(), "chat", "message"); err == nil || !strings.Contains(err.Error(), "405") {
		t.Fatalf("SendMessage() error = %v", err)
	}
}

func TestTelegramProviderReportsInvalidURLsAndTransportFailures(t *testing.T) {
	invalid := NewTelegramProvider(config.TelegramConfig{Token: "secret", BaseURL: "http://[::1"})
	if _, err := invalid.GetUpdates(context.Background(), 0, 1); err == nil {
		t.Fatal("invalid Telegram URL unexpectedly succeeded")
	}
	if err := invalid.SendMessage(context.Background(), "chat", "message"); err == nil {
		t.Fatal("invalid Telegram send URL unexpectedly succeeded")
	}
	closed := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	base := closed.URL
	closed.Close()
	provider := NewTelegramProvider(config.TelegramConfig{Token: "secret", BaseURL: base})
	if _, err := provider.GetUpdates(context.Background(), 0, 1); err == nil {
		t.Fatal("Telegram transport failure was ignored")
	}
}
