package cli_test

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alex/assistent/internal/application"
	cli "github.com/alex/assistent/internal/presentation/cobra"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestScenarioCommands(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Scenario Commands")
}

type serviceStub struct {
	items []application.ScenarioResult
	err   error
	calls []string
}

func (s *serviceStub) Add(_ context.Context, name string) error {
	s.calls = append(s.calls, "add:"+name)
	return s.err
}
func (s *serviceStub) List(context.Context) ([]application.ScenarioResult, error) {
	s.calls = append(s.calls, "list")
	return s.items, s.err
}
func (s *serviceStub) Enable(_ context.Context, name string) error {
	s.calls = append(s.calls, "enable:"+name)
	return s.err
}
func (s *serviceStub) Disable(_ context.Context, name string) error {
	s.calls = append(s.calls, "disable:"+name)
	return s.err
}
func (s *serviceStub) Remove(_ context.Context, name string) error {
	s.calls = append(s.calls, "remove:"+name)
	return s.err
}

var _ = Describe("scenario commands", func() {
	It("invokes each administrative action with its argument", func() {
		for _, tc := range []struct {
			args []string
			call string
		}{
			{[]string{"add", "science-report"}, "add:science-report"},
			{[]string{"enable", "science-report"}, "enable:science-report"},
			{[]string{"disable", "science-report"}, "disable:science-report"},
			{[]string{"remove", "science-report"}, "remove:science-report"},
		} {
			service := &serviceStub{}
			command := cli.NewScenarioCommand(service)
			command.SetArgs(tc.args)
			Expect(command.Execute()).To(Succeed())
			Expect(service.calls).To(Equal([]string{tc.call}))
		}
	})

	It("prints registered scenarios and propagates service errors", func() {
		created := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
		lastRun := created.Add(time.Hour)
		service := &serviceStub{items: []application.ScenarioResult{{
			Name: "science-report", ImplementationType: "ScienceReportScenario", Status: "active",
			CreatedAt: created, UpdatedAt: created, LastRunAt: &lastRun,
		}}}
		command := cli.NewScenarioCommand(service)
		var output bytes.Buffer
		command.SetOut(&output)
		command.SetArgs([]string{"list"})
		Expect(command.Execute()).To(Succeed())
		Expect(output.String()).To(ContainSubstring("NAME            STATUS  IMPLEMENTATION"))
		Expect(output.String()).To(ContainSubstring("science-report"))
		Expect(output.String()).To(ContainSubstring("ScienceReportScenario"))
		Expect(service.calls).To(Equal([]string{"list"}))

		service.err = errors.New("service failed")
		command = cli.NewScenarioCommand(service)
		command.SetArgs([]string{"list"})
		Expect(command.Execute()).To(MatchError("service failed"))
	})

	It("rejects missing action arguments", func() {
		command := cli.NewScenarioCommand(&serviceStub{})
		for _, action := range []string{"add", "enable", "disable", "remove"} {
			command.SetArgs([]string{action})
			Expect(command.Execute()).To(MatchError(ContainSubstring("accepts 1 arg(s)")))
		}
	})

	It("assembles a root command with the scenario command", func() {
		root := cli.NewRootCommand(cli.NewScenarioCommand(&serviceStub{}))
		root.SetArgs([]string{"scenario", "add", "science-report"})
		Expect(root.Execute()).To(Succeed())
	})
})
