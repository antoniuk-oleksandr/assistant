package fxmodules

import (
	"testing"

	"github.com/alex/assistent/internal/domain/scenario"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestModules(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Fx Modules")
}

var _ = Describe("application modules", func() {
	It("registers the compiled science report implementation", func() {
		implementation, err := compiledScenarios().Resolve("science-report")
		Expect(err).NotTo(HaveOccurred())
		Expect(implementation).To(Equal(scenario.Implementation{Name: "science-report", Type: "ScienceReportScenario"}))
	})

	It("creates an error-level Fx logger", func() {
		logger, err := NewFxLogger()
		Expect(err).NotTo(HaveOccurred())
		Expect(logger).NotTo(BeNil())
	})
})
