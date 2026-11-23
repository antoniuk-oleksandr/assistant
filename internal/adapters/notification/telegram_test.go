package notification

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/alex/assistent/internal/domain/notification"
	"github.com/alex/assistent/internal/domain/report"
	"github.com/alex/assistent/internal/domain/scenario"
	"github.com/alex/assistent/internal/platform/config"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func reportResult() report.Result {
	return report.Result{Data: scenario.ReportData{TaskID: 42}}
}

var _ = Describe("Telegram provider", func() {
	It("sends reminders and deliveries", func() {
		var requests int
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requests++
			if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/botsecret/sendMessage") {
				Expect(r.Method).To(Equal(http.MethodPost))
				Expect(r.URL.Path).To(HaveSuffix("/botsecret/sendMessage"))
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()
		provider := NewTelegramProvider(config.TelegramConfig{Token: "secret", BaseURL: server.URL})
		Expect(provider.Name()).To(Equal("telegram"))
		reminder := notification.Reminder{Destination: "chat", Subject: "subject", Message: "message"}
		Expect(provider.SendReminder(context.Background(), reminder)).To(Succeed())
		Expect(provider.Deliver(context.Background(), reportResult(), "chat")).To(Succeed())
		Expect(provider.SendReportReady(context.Background(), "chat", "same report message")).To(Succeed())
		Expect(requests).To(Equal(3))
	})

	It("sends report-ready messages through the configured chat ID", func() {
		var requestBody string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			requestBody = string(body)
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()
		provider := NewTelegramProvider(config.TelegramConfig{Token: "secret", BaseURL: server.URL, ChatID: "admin-chat"})
		Expect(provider.SendReportReady(context.Background(), "", "Sheet: https://sheet.example")).To(Succeed())
		Expect(requestBody).To(ContainSubstring(`"chat_id":"admin-chat"`))
		Expect(requestBody).To(ContainSubstring(`Sheet: https://sheet.example`))
	})

	It("sends direct messages", func() {
		var requestBody string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			requestBody = string(body)
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()
		provider := NewTelegramProvider(config.TelegramConfig{Token: "secret", BaseURL: server.URL})
		Expect(provider.SendMessage(context.Background(), "chat-7", "connected")).To(Succeed())
		Expect(requestBody).To(ContainSubstring(`"chat_id":"chat-7"`))
		Expect(requestBody).To(ContainSubstring("connected"))
	})

	It("uses the configured chat ID when the destination is empty", func() {
		var requestBody string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			requestBody = string(body)
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()
		provider := NewTelegramProvider(config.TelegramConfig{Token: "secret", BaseURL: server.URL, ChatID: "-1001234567890"})
		Expect(provider.SendReminder(context.Background(), notification.Reminder{Subject: "subject", Message: "message"})).To(Succeed())
		Expect(requestBody).To(ContainSubstring(`"chat_id":"-1001234567890"`))
	})

	It("rejects missing tokens and HTTP failures", func() {
		provider := NewTelegramProvider(config.TelegramConfig{})
		Expect(provider.SendReminder(context.Background(), notification.Reminder{})).To(HaveOccurred())
		server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		server.Close()
		provider = NewTelegramProvider(config.TelegramConfig{Token: "secret", BaseURL: server.URL})
		Expect(provider.SendReminder(context.Background(), notification.Reminder{})).To(HaveOccurred())
	})

	It("reads private messages and contacts", func() {
		var requestBody string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/botsecret/getUpdates" {
				if r.URL.Query().Get("offset") != "7" || r.URL.Query().Get("timeout") != "10" {
					Expect(r.URL.Query().Get("offset")).To(Equal("7"))
					Expect(r.URL.Query().Get("timeout")).To(Equal("10"))
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
		Expect(err).NotTo(HaveOccurred())
		Expect(updates).To(HaveLen(1))
		Expect(updates[0].ChatID).To(Equal("42"))
		Expect(updates[0].Text).To(Equal("/start pair_token"))
		Expect(requestBody).To(BeEmpty())
	})

	It("rejects unsuccessful update responses", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.WriteString(w, `{"ok":false,"result":[]}`)
		}))
		defer server.Close()
		provider := NewTelegramProvider(config.TelegramConfig{Token: "secret", BaseURL: server.URL})
		_, err := provider.GetUpdates(context.Background(), 0, 1)
		Expect(err).To(HaveOccurred())
	})

	It("rejects malformed updates and markup", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/botsecret/getUpdates" {
				_, _ = io.WriteString(w, "not-json")
				return
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()
		provider := NewTelegramProvider(config.TelegramConfig{Token: "secret", BaseURL: server.URL})
		_, err := provider.GetUpdates(context.Background(), 0, 1)
		Expect(err).To(HaveOccurred())
		Expect(provider.sendWithMarkup(context.Background(), "chat", "message", "not-json")).To(HaveOccurred())
	})

	It("sends reply markup and rejects missing update tokens", func() {
		var body string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			payload, _ := io.ReadAll(r.Body)
			body = string(payload)
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()
		provider := NewTelegramProvider(config.TelegramConfig{Token: "secret", BaseURL: server.URL})
		Expect(provider.sendWithMarkup(context.Background(), "chat", "message", `{"inline_keyboard":[]}`)).To(Succeed())
		Expect(body).To(ContainSubstring("reply_markup"))
		_, err := NewTelegramProvider(config.TelegramConfig{}).GetUpdates(context.Background(), 0, 1)
		Expect(err).To(HaveOccurred())
	})

	It("rejects canceled requests", func() {
		provider := NewTelegramProvider(config.TelegramConfig{Token: "secret", BaseURL: "http://127.0.0.1:1"})
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		Expect(provider.SendMessage(ctx, "chat", "message")).To(HaveOccurred())
	})

	It("reports Telegram HTTP status failures", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusMethodNotAllowed)
		}))
		defer server.Close()
		provider := NewTelegramProvider(config.TelegramConfig{Token: "secret", BaseURL: server.URL})
		_, err := provider.GetUpdates(context.Background(), 0, 1)
		Expect(err).To(MatchError(ContainSubstring("405")))
		Expect(provider.SendMessage(context.Background(), "chat", "message")).To(MatchError(ContainSubstring("405")))
	})

	It("reports invalid URLs and transport failures", func() {
		invalid := NewTelegramProvider(config.TelegramConfig{Token: "secret", BaseURL: "http://[::1"})
		_, err := invalid.GetUpdates(context.Background(), 0, 1)
		Expect(err).To(HaveOccurred())
		Expect(invalid.SendMessage(context.Background(), "chat", "message")).To(HaveOccurred())
		closed := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		base := closed.URL
		closed.Close()
		provider := NewTelegramProvider(config.TelegramConfig{Token: "secret", BaseURL: base})
		_, err = provider.GetUpdates(context.Background(), 0, 1)
		Expect(err).To(HaveOccurred())
	})
})
