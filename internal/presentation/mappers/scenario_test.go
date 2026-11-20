package mappers_test

import (
	"testing"
	"time"

	"github.com/alex/assistent/internal/application"
	"github.com/alex/assistent/internal/presentation/mappers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestPresentationMapper(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Presentation Mapper")
}

var _ = Describe("scenario DTO mapper", func() {
	It("formats timestamps in UTC and uses a placeholder for an absent last run", func() {
		created := time.Date(2026, 3, 4, 5, 6, 7, 0, time.FixedZone("offset", 2*60*60))
		updated := created.Add(time.Hour)
		result := mappers.ScenarioToDTO(application.ScenarioResult{
			Name: "science-report", ImplementationType: "ScienceReportScenario", Status: "active",
			CreatedAt: created, UpdatedAt: updated,
		})
		Expect(result.Name).To(Equal("science-report"))
		Expect(result.ImplementationType).To(Equal("ScienceReportScenario"))
		Expect(result.Status).To(Equal("active"))
		Expect(result.CreatedAt).To(Equal(created.UTC().Format(time.RFC3339Nano)))
		Expect(result.UpdatedAt).To(Equal(updated.UTC().Format(time.RFC3339Nano)))
		Expect(result.LastRunAt).To(Equal("-"))
	})

	It("formats a present last run in UTC", func() {
		lastRun := time.Date(2026, 3, 4, 5, 6, 7, 0, time.FixedZone("offset", -3*60*60))
		result := mappers.ScenarioToDTO(application.ScenarioResult{LastRunAt: &lastRun})
		Expect(result.LastRunAt).To(Equal(lastRun.UTC().Format(time.RFC3339Nano)))
	})
})
