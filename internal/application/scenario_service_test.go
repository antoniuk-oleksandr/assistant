package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alex/assistent/internal/application"
	"github.com/alex/assistent/internal/domain/scenario"
	"github.com/alex/assistent/internal/domain/task"
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

type taskBindingRepositoryStub struct {
	items      []task.Binding
	bindErr    error
	unbindErr  error
	listErr    error
	activeErr  error
	bound      []string
	unbound    []string
	listedName string
}

func (r *taskBindingRepositoryStub) Bind(_ context.Context, scenarioName, taskID string) error {
	r.bound = append(r.bound, scenarioName+":"+taskID)
	return r.bindErr
}
func (r *taskBindingRepositoryStub) Unbind(_ context.Context, taskID string) error {
	r.unbound = append(r.unbound, taskID)
	return r.unbindErr
}
func (r *taskBindingRepositoryStub) List(_ context.Context, scenarioName string) ([]task.Binding, error) {
	r.listedName = scenarioName
	return r.items, r.listErr
}
func (r *taskBindingRepositoryStub) Active(context.Context) ([]task.Binding, error) {
	return r.items, r.activeErr
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
		repo.statusErr, repo.removeErr = nil, nil
		Expect(service.Enable(ctx, "missing")).To(MatchError(`ScenarioNotFound: "missing"`))
		Expect(service.Remove(ctx, "missing")).To(MatchError(`ScenarioNotFound: "missing"`))
	})

	It("coordinates task binding operations and maps binding results", func() {
		repo := &scenarioRepositoryStub{}
		bindings := &taskBindingRepositoryStub{items: []task.Binding{{
			ScenarioName: implementation.Name, ScenarioType: implementation.Type, Status: "active",
			Task: task.Task{ExternalID: "task-1", CourseID: "course-1", Title: "Report"},
		}}}
		service := application.NewScenarioServiceWithBindings(repo, scenario.NewRegistry(implementation), bindings)

		Expect(service.BindTask(ctx, implementation.Name, "task-1")).To(Succeed())
		Expect(bindings.bound).To(Equal([]string{"science-report:task-1"}))
		Expect(service.UnbindTask(ctx, "task-1")).To(Succeed())
		Expect(bindings.unbound).To(Equal([]string{"task-1"}))
		result, err := service.ListTaskBindings(ctx, implementation.Name)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal([]application.TaskBindingResult{{
			ScenarioName: implementation.Name, ScenarioType: implementation.Type, TaskID: "task-1",
			CourseID: "course-1", Title: "Report", Status: "active",
		}}))
		Expect(bindings.listedName).To(Equal(implementation.Name))
		active, err := service.ActiveTaskBindings(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(active).To(Equal(bindings.items))

		bindings.bindErr = errors.New("bind failed")
		Expect(service.BindTask(ctx, implementation.Name, "task-1")).To(MatchError("bind failed"))
		bindings.unbindErr = errors.New("unbind failed")
		Expect(service.UnbindTask(ctx, "task-1")).To(MatchError("unbind failed"))
		bindings.listErr = errors.New("list bindings failed")
		_, err = service.ListTaskBindings(ctx, implementation.Name)
		Expect(err).To(MatchError("list bindings failed"))
		bindings.activeErr = errors.New("active bindings failed")
		_, err = service.ActiveTaskBindings(ctx)
		Expect(err).To(MatchError("active bindings failed"))
	})

	It("reports unavailable task binding operations when no repository is configured", func() {
		service := application.NewScenarioService(repoStubForBindings{}, scenario.NewRegistry(implementation))
		Expect(service.BindTask(ctx, implementation.Name, "task-1")).To(MatchError("task bindings are unavailable"))
		Expect(service.UnbindTask(ctx, "task-1")).To(MatchError("task bindings are unavailable"))
		_, err := service.ListTaskBindings(ctx, implementation.Name)
		Expect(err).To(MatchError("task bindings are unavailable"))
		_, err = service.ActiveTaskBindings(ctx)
		Expect(err).To(MatchError("task bindings are unavailable"))
	})
})

type repoStubForBindings struct{}

func (repoStubForBindings) Create(context.Context, scenario.Implementation) error    { return nil }
func (repoStubForBindings) List(context.Context) ([]scenario.Scenario, error)        { return nil, nil }
func (repoStubForBindings) ListActive(context.Context) ([]scenario.Scenario, error)  { return nil, nil }
func (repoStubForBindings) SetStatus(context.Context, string, scenario.Status) error { return nil }
func (repoStubForBindings) Remove(context.Context, string) error                     { return nil }
