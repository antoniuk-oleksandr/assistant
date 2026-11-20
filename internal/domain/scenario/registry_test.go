package scenario_test

import (
	"testing"

	"github.com/alex/assistent/internal/domain/scenario"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestScenarioRegistry(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Scenario Registry")
}

var _ = Describe("compiled scenario registry", func() {
	var registry scenario.Registry

	BeforeEach(func() {
		registry = scenario.NewRegistry(scenario.Implementation{
			Name: "science-report",
			Type: "ScienceReportScenario",
		})
	})

	It("resolves a compiled implementation by its CLI name", func() {
		implementation, err := registry.Resolve("science-report")

		Expect(err).NotTo(HaveOccurred())
		Expect(implementation).To(Equal(scenario.Implementation{
			Name: "science-report",
			Type: "ScienceReportScenario",
		}))
	})

	It("rejects an implementation that is not compiled into the application", func() {
		_, err := registry.Resolve("unknown")

		Expect(err).To(MatchError(`ScenarioNotFound: "unknown"`))
	})
})
