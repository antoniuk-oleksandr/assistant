package app_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alex/assistent/internal/app"
	"github.com/alex/assistent/internal/platform/config"
	"github.com/alex/assistent/internal/platform/locking"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestCLI(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Application CLI")
}

var _ = Describe("RunCLI", func() {
	BeforeEach(func() {
		Expect(os.Setenv(config.SQLitePathEnv, filepath.Join(GinkgoT().TempDir(), "state.db"))).To(Succeed())
		Expect(os.Setenv("REPORT_RENDERER", "local-xlsx")).To(Succeed())
	})

	It("starts the application and configures the report workflow", func(ctx SpecContext) {
		Expect(app.RunCLI(ctx, []string{"course", "add", "900423945297"})).To(Succeed())
		Expect(app.RunCLI(ctx, []string{"report", "configure", "900423945297"})).To(Succeed())
		Expect(app.RunCLI(ctx, []string{"report", "task-list"})).To(Succeed())
	})

	It("returns the application error for an unregistered report course", func(ctx SpecContext) {
		Expect(app.RunCLI(context.WithValue(ctx, struct{}{}, "value"), []string{"report", "configure", "unknown"})).To(MatchError(ContainSubstring("find active course")))
	})

	It("returns an Fx startup error when configuration is invalid", func(ctx SpecContext) {
		Expect(os.Unsetenv(config.SQLitePathEnv)).To(Succeed())
		Expect(os.Chdir(GinkgoT().TempDir())).To(Succeed())
		Expect(os.WriteFile(".env", []byte("INVALID=\"unterminated\n"), 0600)).To(Succeed())
		Expect(app.RunCLI(ctx, []string{"report", "task-list"})).NotTo(Succeed())
	})

	It("returns a polling startup error for a canceled background context", func(ctx SpecContext) {
		canceled, cancel := context.WithCancel(ctx)
		cancel()
		Expect(app.RunCLI(canceled, nil)).NotTo(Succeed())
	})

	It("returns an error when another process owns the application lock", func(ctx SpecContext) {
		path := filepath.Join(GinkgoT().TempDir(), "state.db")
		Expect(os.Setenv(config.SQLitePathEnv, path)).To(Succeed())
		lock := locking.NewProcessLock(config.DatabaseConfig{SQLitePath: path})
		Expect(lock.Acquire()).To(Succeed())
		DeferCleanup(lock.Release)
		Expect(app.RunCLI(ctx, nil)).To(MatchError(ContainSubstring("ApplicationAlreadyRunning")))
	})

	It("runs the scheduled application until its context is canceled", func(ctx SpecContext) {
		short, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
		defer cancel()
		Expect(app.RunCLI(short, nil)).To(Succeed())
	})

	It("returns a command error for an unknown CLI command", func(ctx SpecContext) {
		Expect(app.RunCLI(ctx, []string{"unknown-command"})).NotTo(Succeed())
	})

	It("dispatches the report and administration command families", func(ctx SpecContext) {
		for _, args := range [][]string{
			{"course", "list"},
			{"report", "data-list", "900423945297"},
			{"report", "task-list"},
			{"teacher", "list"},
			{"notification", "dlq", "list"},
			{"report", "task-unbind", "missing-task"},
		} {
			_ = app.RunCLI(ctx, args)
		}
	})
})
