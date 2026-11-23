package application_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/alex/assistent/internal/application"
	"github.com/alex/assistent/internal/domain/scenario"
	"github.com/alex/assistent/internal/domain/task"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type reportserviceGinkgoTest struct{}

func (*reportserviceGinkgoTest) TempDir() string          { return GinkgoT().TempDir() }
func (*reportserviceGinkgoTest) Setenv(key, value string) { GinkgoT().Setenv(key, value) }
func (*reportserviceGinkgoTest) Cleanup(fn func())        { DeferCleanup(fn) }
func (*reportserviceGinkgoTest) Helper()                  {}
func (*reportserviceGinkgoTest) Fatal(args ...any)        { Expect(false).To(BeTrue(), fmt.Sprint(args...)) }
func (*reportserviceGinkgoTest) Fatalf(format string, args ...any) {
	Expect(false).To(BeTrue(), fmt.Sprintf(format, args...))
}
func (*reportserviceGinkgoTest) Errorf(format string, args ...any) {
	Expect(false).To(BeTrue(), fmt.Sprintf(format, args...))
}
func (*reportserviceGinkgoTest) Run(name string, fn func(*reportserviceGinkgoTest)) bool {
	By(name)
	fn(&reportserviceGinkgoTest{})
	return true
}

func TestApplicationSpecs(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Application")
}

type reportScenarioRepo struct {
	created              bool
	statusErr, createErr error
}

func (r *reportScenarioRepo) Create(context.Context, scenario.Implementation) error {
	r.created = true
	return r.createErr
}
func (r *reportScenarioRepo) List(context.Context) ([]scenario.Scenario, error) { return nil, nil }
func (r *reportScenarioRepo) ListActive(context.Context) ([]scenario.Scenario, error) {
	return nil, nil
}
func (r *reportScenarioRepo) SetStatus(context.Context, string, scenario.Status) error {
	return r.statusErr
}
func (r *reportScenarioRepo) Remove(context.Context, string) error { return nil }

type reportBindings struct {
	items []task.Binding
	err   error
	bound []string
}

func (r *reportBindings) Bind(_ context.Context, name, id string) error {
	r.bound = append(r.bound, name+":"+id)
	return r.err
}
func (r *reportBindings) Unbind(_ context.Context, id string) error {
	r.bound = append(r.bound, "unbind:"+id)
	return r.err
}
func (r *reportBindings) List(context.Context, string) ([]task.Binding, error) { return r.items, r.err }
func (r *reportBindings) Active(context.Context) ([]task.Binding, error)       { return r.items, r.err }

type reportCourses struct{ err error }

func (r *reportCourses) Bind(context.Context, string, string) error   { return r.err }
func (r *reportCourses) Unbind(context.Context, string, string) error { return nil }
func (r *reportCourses) List(context.Context, string) ([]scenario.CourseBinding, error) {
	return nil, nil
}

type reportData struct {
	items []scenario.CourseDataField
	err   error
}

func (r *reportData) Set(context.Context, string, string, map[string]string) error { return r.err }
func (r *reportData) List(context.Context, string, string) ([]scenario.CourseDataField, error) {
	return r.items, r.err
}

func testReportServiceUsesTheBuiltInWorkflow(t *reportserviceGinkgoTest) {
	ctx := context.Background()
	repo := &reportScenarioRepo{}
	bindings := &reportBindings{items: []task.Binding{{Task: task.Task{ExternalID: "task-1", CourseID: "course-1", Title: "Report"}, Status: "active"}}}
	data := &reportData{items: []scenario.CourseDataField{{Name: "department_name", Value: "Science"}}}
	service := application.NewReportService(repo, scenario.NewRegistry(scenario.Implementation{Name: "science-report", Type: "ScienceReportScenario"}), bindings, &reportCourses{}, data)
	if err := service.Configure(ctx, "course-1"); err != nil {
		t.Fatal(err)
	}
	if !repo.created || len(bindings.bound) != 0 {
		t.Fatalf("configuration state = %#v %#v", repo, bindings)
	}
	if err := service.BindTask(ctx, "task-1"); err != nil {
		t.Fatal(err)
	}
	if err := service.UnbindTask(ctx, "task-1"); err != nil {
		t.Fatal(err)
	}
	items, err := service.ListTasks(ctx)
	if err != nil || len(items) != 1 || items[0].TaskID != "task-1" {
		t.Fatalf("tasks = %#v, %v", items, err)
	}
	if err := service.SetData(ctx, "course-1", map[string]string{"department_name": "Science"}); err != nil {
		t.Fatal(err)
	}
	dataItems, err := service.ListData(ctx, "course-1")
	if err != nil || len(dataItems) != 1 {
		t.Fatalf("data = %#v, %v", dataItems, err)
	}
}

func testReportServicePropagatesAndReportsUnavailableDependencies(t *reportserviceGinkgoTest) {
	ctx := context.Background()
	repo := &reportScenarioRepo{createErr: errors.New("create failed")}
	service := application.NewReportService(repo, scenario.NewRegistry(scenario.Implementation{Name: "science-report", Type: "ScienceReportScenario"}), nil, nil, nil)
	if err := service.Add(ctx); err == nil || err.Error() != "create failed" {
		t.Fatalf("Add() error = %v", err)
	}
	repo.createErr = nil
	repo.statusErr = errors.New("status failed")
	if err := service.Enable(ctx); err == nil || err.Error() != "status failed" {
		t.Fatalf("Enable() error = %v", err)
	}
	if err := service.BindCourse(ctx, "course"); err == nil || err.Error() != "report course bindings are unavailable" {
		t.Fatalf("BindCourse() error = %v", err)
	}
	if err := service.BindTask(ctx, "task"); err == nil || err.Error() != "report task bindings are unavailable" {
		t.Fatalf("BindTask() error = %v", err)
	}
	if err := service.UnbindTask(ctx, "task"); err == nil || err.Error() != "report task bindings are unavailable" {
		t.Fatalf("UnbindTask() error = %v", err)
	}
	if _, err := service.ListTasks(ctx); err == nil || err.Error() != "report task bindings are unavailable" {
		t.Fatalf("ListTasks() error = %v", err)
	}
	if err := service.SetData(ctx, "course", nil); err == nil || err.Error() != "report course data is unavailable" {
		t.Fatalf("SetData() error = %v", err)
	}
	if _, err := service.ListData(ctx, "course"); err == nil || err.Error() != "report course data is unavailable" {
		t.Fatalf("ListData() error = %v", err)
	}
	missing := application.NewReportService(&reportScenarioRepo{}, scenario.NewRegistry(), &reportBindings{}, &reportCourses{}, &reportData{})
	if err := missing.Add(ctx); err == nil {
		t.Fatal("Add() unexpectedly resolved missing implementation")
	}
	if err := missing.Enable(ctx); err == nil {
		t.Fatal("Enable() unexpectedly resolved missing implementation")
	}
	failingBindings := &reportBindings{err: errors.New("binding failed")}
	failing := application.NewReportService(&reportScenarioRepo{}, scenario.NewRegistry(scenario.Implementation{Name: "science-report", Type: "ScienceReportScenario"}), failingBindings, &reportCourses{}, &reportData{})
	if err := failing.UnbindTask(ctx, "task"); err == nil || err.Error() != "binding failed" {
		t.Fatalf("UnbindTask() error = %v", err)
	}
	if _, err := failing.ListTasks(ctx); err == nil || err.Error() != "binding failed" {
		t.Fatalf("ListTasks() error = %v", err)
	}
	failingData := &reportData{err: errors.New("data failed")}
	failing = application.NewReportService(&reportScenarioRepo{}, scenario.NewRegistry(scenario.Implementation{Name: "science-report", Type: "ScienceReportScenario"}), &reportBindings{}, &reportCourses{}, failingData)
	if err := failing.SetData(ctx, "course", nil); err == nil || err.Error() != "data failed" {
		t.Fatalf("SetData() error = %v", err)
	}
	if _, err := failing.ListData(ctx, "course"); err == nil || err.Error() != "data failed" {
		t.Fatalf("ListData() error = %v", err)
	}
}

func testReportServiceConfigureStopsAtEachFailedStep(t *reportserviceGinkgoTest) {
	ctx := context.Background()
	registry := scenario.NewRegistry(scenario.Implementation{Name: "science-report", Type: "ScienceReportScenario"})
	createFailed := &reportScenarioRepo{createErr: errors.New("create")}
	if err := application.NewReportService(createFailed, registry, &reportBindings{}, &reportCourses{}, &reportData{}).Configure(ctx, "course"); err == nil || err.Error() != "create" {
		t.Fatalf("Configure create error = %v", err)
	}
	statusFailed := &reportScenarioRepo{statusErr: errors.New("status")}
	if err := application.NewReportService(statusFailed, registry, &reportBindings{}, &reportCourses{}, &reportData{}).Configure(ctx, "course"); err == nil || err.Error() != "status" {
		t.Fatalf("Configure status error = %v", err)
	}
	courseFailed := &reportCourses{err: errors.New("course")}
	if err := application.NewReportService(&reportScenarioRepo{}, registry, &reportBindings{}, courseFailed, &reportData{}).Configure(ctx, "course"); err == nil || err.Error() != "course" {
		t.Fatalf("Configure course error = %v", err)
	}
}

var _ = Describe("report_service legacy tests", func() {
	It("runs the converted standard-library cases", func() {
		t := &reportserviceGinkgoTest{}
		testReportServiceUsesTheBuiltInWorkflow(t)
		testReportServicePropagatesAndReportsUnavailableDependencies(t)
		testReportServiceConfigureStopsAtEachFailedStep(t)
	})
})
