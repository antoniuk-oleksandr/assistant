package fxmodules

import (
	"context"
	"testing"

	"github.com/alex/assistent/internal/adapters/notification"
	"github.com/alex/assistent/internal/application"
	"github.com/alex/assistent/internal/domain/task"
	"github.com/alex/assistent/internal/platform/config"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"time"
)

type testLifecycle struct{ hook fx.Hook }

func (l *testLifecycle) Append(hook fx.Hook) { l.hook = hook }

func TestModuleConstructors(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Module constructors")
}

var _ = Describe("module constructors", func() {
	It("constructs configured modules", func() {
		Expect(configuredReminderProviders(config.EmailConfig{}, config.TelegramConfig{})).To(HaveLen(2))
		Expect(configuredDeliveryProviders(config.EmailConfig{}, config.TelegramConfig{})).To(HaveLen(2))
		_ = compiledScenarios()
		Expect(newMaterialParsing()).NotTo(BeNil())
		Expect(NewFxLogger(zap.NewNop())).NotTo(BeNil())
		configureTelegramPairing(telegramPairingParams{Telegram: config.TelegramConfig{}})
		lifecycle := &testLifecycle{}
		configureTelegramPairing(telegramPairingParams{Service: application.NewTelegramPairingService(nil, nil, nil, nil), Telegram: config.TelegramConfig{Token: "token"}, Lifecycle: lifecycle})
		Expect(lifecycle.hook.OnStart).NotTo(BeNil())
		Expect(lifecycle.hook.OnStop).NotTo(BeNil())
		Expect(lifecycle.hook.OnStart(context.Background())).To(Succeed())
		Expect(lifecycle.hook.OnStop(context.Background())).To(Succeed())
		_ = notification.NewTelegramProvider(config.TelegramConfig{})
	})

	It("configures polling deadlines", func() {
		service := application.NewPollingService(nil, nil, nil, nil, nil)
		configurePollingDeadlines(pollingDeadlineParams{
			Service:  service,
			Deadline: config.DeadlineConfig{PreliminaryBefore: 2 * time.Minute, RefreshBefore: time.Minute},
		})
		service.SetDeadlineWindows(task.DeadlineWindows{})
		Expect(service).NotTo(BeNil())
	})
})
