package mappers_test

import (
	"database/sql"
	"testing"
	"time"

	"github.com/alex/assistent/internal/adapters/mappers"
	"github.com/alex/assistent/internal/adapters/models"
	"github.com/alex/assistent/internal/domain/scenario"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestScenarioMapper(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Scenario Mapper")
}

var _ = Describe("scenario row mapper", func() {
	created := time.Date(2026, 2, 3, 4, 5, 6, 123000000, time.UTC)
	updated := created.Add(time.Hour)
	lastRun := updated.Add(time.Hour)
	row := func() models.ScenarioRow {
		return models.ScenarioRow{
			ID: 4, Name: "science-report", ImplementationType: "ScienceReportScenario", Status: "active",
			CreatedAt: created.Format(time.RFC3339Nano), UpdatedAt: updated.Format(time.RFC3339Nano),
			LastRunAt: sql.NullString{String: lastRun.Format(time.RFC3339Nano), Valid: true},
		}
	}

	It("maps all persisted fields including an optional last run", func() {
		result, err := mappers.ToScenario(row())
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(scenario.Scenario{
			ID: 4, Implementation: scenario.Implementation{Name: "science-report", Type: "ScienceReportScenario"},
			Status: scenario.Active, CreatedAt: created, UpdatedAt: updated, LastRunAt: &lastRun,
		}))
	})

	It("maps a null last run without a pointer", func() {
		input := row()
		input.LastRunAt = sql.NullString{}
		result, err := mappers.ToScenario(input)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.LastRunAt).To(BeNil())
	})

	It("reports malformed timestamps with field context", func() {
		input := row()
		input.CreatedAt = "bad"
		_, err := mappers.ToScenario(input)
		Expect(err).To(MatchError(ContainSubstring("parse scenario created_at")))
		input = row()
		input.UpdatedAt = "bad"
		_, err = mappers.ToScenario(input)
		Expect(err).To(MatchError(ContainSubstring("parse scenario updated_at")))
		input = row()
		input.LastRunAt = sql.NullString{String: "bad", Valid: true}
		_, err = mappers.ToScenario(input)
		Expect(err).To(MatchError(ContainSubstring("parse scenario last_run_at")))
	})
})
