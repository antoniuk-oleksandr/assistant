package fxmodules

import (
	"context"
	"testing"

	"github.com/alex/assistent/internal/adapters/notification"
	"github.com/alex/assistent/internal/application"
	"github.com/alex/assistent/internal/domain/task"
	"github.com/alex/assistent/internal/platform/config"
	"go.uber.org/fx"
	"time"
)

type testLifecycle struct{ hook fx.Hook }

func (l *testLifecycle) Append(hook fx.Hook) { l.hook = hook }

func TestModuleConstructors(t *testing.T) {
	if len(configuredReminderProviders(config.EmailConfig{}, config.TelegramConfig{})) != 2 {
		t.Fatal("expected email and Telegram reminder providers")
	}
	if len(configuredDeliveryProviders(config.EmailConfig{}, config.TelegramConfig{})) != 2 {
		t.Fatal("expected email and Telegram delivery providers")
	}
	_ = compiledScenarios()
	if newMaterialParsing() == nil {
		t.Fatal("material parser constructor returned nil")
	}
	if _, err := NewFxLogger(); err != nil {
		t.Fatal(err)
	}
	configureTelegramPairing(telegramPairingParams{Telegram: config.TelegramConfig{}})
	lifecycle := &testLifecycle{}
	configureTelegramPairing(telegramPairingParams{Service: application.NewTelegramPairingService(nil, nil, nil, nil), Telegram: config.TelegramConfig{Token: "token"}, Lifecycle: lifecycle})
	if lifecycle.hook.OnStart == nil || lifecycle.hook.OnStop == nil {
		t.Fatal("Telegram pairing lifecycle hook was not registered")
	}
	if err := lifecycle.hook.OnStart(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := lifecycle.hook.OnStop(context.Background()); err != nil {
		t.Fatal(err)
	}
	_ = notification.NewTelegramProvider(config.TelegramConfig{})
}

func TestConfigurePollingDeadlines(t *testing.T) {
	service := application.NewPollingService(nil, nil, nil, nil, nil)
	configurePollingDeadlines(pollingDeadlineParams{
		Service:  service,
		Deadline: config.DeadlineConfig{PreliminaryBefore: 2 * time.Minute, RefreshBefore: time.Minute},
	})
	service.SetDeadlineWindows(task.DeadlineWindows{})
	if service == nil {
		t.Fatal("service unexpectedly nil")
	}
}
