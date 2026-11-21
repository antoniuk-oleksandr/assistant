package notification

import (
	"context"
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
	if requests != 2 {
		t.Fatalf("requests = %d, want 2", requests)
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
