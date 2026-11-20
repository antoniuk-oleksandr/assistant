package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alex/assistent/internal/application"
	"github.com/alex/assistent/internal/domain/scenario"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestScenarioService(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Scenario Service")
}

type scenarioRepositoryStub struct {
	created    []scenario.Implementation
	listed     []scenario.Scenario
	createErr  error
	listErr    error
	statusErr  error
	removeErr  error
	statusName string
	status     scenario.Status
	removed    string
}

func (r *scenarioRepositoryStub) Create(_ context.Context, implementation scenario.Implementation) error {
	if r.createErr != nil {
		return r.createErr
	}
	r.created = append(r.created, implementation)
	return nil
}
func (r *scenarioRepositoryStub) List(context.Context) ([]scenario.Scenario, error) {
	return r.listed, r.listErr
}
func (r *scenarioRepositoryStub) ListActive(context.Context) ([]scenario.Scenario, error) {
	return r.listed, r.listErr
}
func (r *scenarioRepositoryStub) SetStatus(_ context.Context, name string, status scenario.Status) error {
	r.statusName, r.status = name, status
	return r.statusErr
}
func (r *scenarioRepositoryStub) Remove(_ context.Context, name string) error {
	r.removed = name
	return r.removeErr
}

var _ = Describe("scenario service", func() {
	implementation := scenario.Implementation{Name: "science-report", Type: "ScienceReportScenario"}
	ctx := context.Background()

	It("registers a compiled implementation", func() {
		repo := &scenarioRepositoryStub{}
		service := application.NewScenarioService(repo, scenario.NewRegistry(implementation))

		Expect(service.Add(ctx, implementation.Name)).To(Succeed())
		Expect(repo.created).To(ConsistOf(implementation))
	})

	It("rejects adding an implementation that is not compiled", func() {
		repo := &scenarioRepositoryStub{}
		service := application.NewScenarioService(repo, scenario.NewRegistry(implementation))

		Expect(service.Add(ctx, "missing")).To(MatchError(`ScenarioNotFound: "missing"`))
		Expect(repo.created).To(BeEmpty())
	})

	It("returns repository errors while listing and preserves all result fields", func() {
		created := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
		updated := created.Add(time.Hour)
		lastRun := updated.Add(time.Hour)
		repo := &scenarioRepositoryStub{listed: []scenario.Scenario{{
			ID: 7, Implementation: implementation, Status: scenario.Disabled,
			CreatedAt: created, UpdatedAt: updated, LastRunAt: &lastRun,
		}}}
		service := application.NewScenarioService(repo, scenario.NewRegistry(implementation))

		results, err := service.List(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(results).To(Equal([]application.ScenarioResult{{
			Name: implementation.Name, ImplementationType: implementation.Type,
			Status: string(scenario.Disabled), CreatedAt: created, UpdatedAt: updated, LastRunAt: &lastRun,
		}}))

		repo.listErr = errors.New("list failed")
		_, err = service.List(ctx)
		Expect(err).To(MatchError("list failed"))
	})

	It("changes status and removes only compiled implementations", func() {
		repo := &scenarioRepositoryStub{}
		service := application.NewScenarioService(repo, scenario.NewRegistry(implementation))

		Expect(service.Enable(ctx, implementation.Name)).To(Succeed())
		Expect(repo.statusName).To(Equal(implementation.Name))
		Expect(repo.status).To(Equal(scenario.Active))
		Expect(service.Disable(ctx, implementation.Name)).To(Succeed())
		Expect(repo.status).To(Equal(scenario.Disabled))
		Expect(service.Remove(ctx, implementation.Name)).To(Succeed())
		Expect(repo.removed).To(Equal(implementation.Name))
		repo.statusErr, repo.removeErr = errors.New("status failed"), errors.New("remove failed")
		Expect(service.Enable(ctx, implementation.Name)).To(MatchError("status failed"))
		Expect(service.Remove(ctx, implementation.Name)).To(MatchError("remove failed"))
		Expect(service.Disable(ctx, "missing")).To(MatchError(`ScenarioNotFound: "missing"`))
	})
})
