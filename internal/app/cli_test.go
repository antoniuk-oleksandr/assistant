package app_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/alex/assistent/internal/app"
	"github.com/alex/assistent/internal/platform/config"
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
	})

	It("starts the application and registers a compiled scenario", func(ctx SpecContext) {
		Expect(app.RunCLI(ctx, []string{"scenario", "add", "science-report"})).To(Succeed())
		Expect(app.RunCLI(ctx, []string{"scenario", "list"})).To(Succeed())
	})

	It("returns the application error for an unknown scenario", func(ctx SpecContext) {
		Expect(app.RunCLI(context.WithValue(ctx, struct{}{}, "value"), []string{"scenario", "add", "unknown"})).To(MatchError(`ScenarioNotFound: "unknown"`))
	})
})
