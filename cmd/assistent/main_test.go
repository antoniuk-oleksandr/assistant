package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestAssistent(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Assistent Suite")
}

var _ = Describe("the application entry point", func() {
	var dir string

	BeforeEach(func() {
		dir = GinkgoT().TempDir()
	})

	It("persists the scenario lifecycle across separate invocations", func() {
		run := func(args ...string) string {
			GinkgoHelper()
			stdout, stderr, err := runScenarioCLI(dir, args...)
			Expect(err).NotTo(HaveOccurred(), "scenario %v: %s", args, stderr)
			Expect(stderr).To(BeEmpty())
			return stdout
		}
		list := func() []string { return strings.Split(strings.TrimSpace(run("list")), "\n") }
		Expect(list()).To(HaveLen(1))
		run("add", "science-report")
		active := list()
		Expect(active).To(HaveLen(2))
		Expect(strings.Fields(active[0])).To(Equal(strings.Fields("NAME STATUS IMPLEMENTATION CREATED AT UPDATED AT LAST RUN")))
		row := strings.Fields(active[1])
		Expect(row).To(HaveLen(6))
		Expect(row[:3]).To(Equal([]string{"science-report", "active", "ScienceReportScenario"}))
		for _, stamp := range row[3:5] {
			_, err := time.Parse(time.RFC3339Nano, stamp)
			Expect(err).NotTo(HaveOccurred())
			Expect(stamp).To(HaveSuffix("Z"))
		}
		Expect(row[5]).To(Equal("-"))

		run("disable", "science-report")
		disabled := list()
		Expect(disabled).To(HaveLen(2))
		disabledRow := strings.Fields(disabled[1])
		Expect(disabledRow).To(HaveLen(6))
		Expect(disabledRow[:3]).To(Equal([]string{"science-report", "disabled", "ScienceReportScenario"}))
		run("add", "science-report")
		Expect(list()).To(Equal(disabled))

		run("enable", "science-report")
		enabled := list()
		Expect(enabled).To(HaveLen(2))
		enabledRow := strings.Fields(enabled[1])
		Expect(enabledRow).To(HaveLen(6))
		Expect(enabledRow[:3]).To(Equal(row[:3]))
		run("remove", "science-report")
		removed := list()
		Expect(removed).To(HaveLen(1))
		Expect(strings.Fields(removed[0])).To(Equal(strings.Fields(active[0])))
	})

	DescribeTable("rejects invalid scenario commands without registering anything",
		func(args []string, message string) {
			stdout, stderr, err := runScenarioCLI(dir, args...)
			var exit *exec.ExitError
			Expect(err).To(BeAssignableToTypeOf(exit))
			Expect(err.(*exec.ExitError).ExitCode()).To(Equal(1))
			Expect(stdout).To(BeEmpty())
			Expect(stderr).To(Equal(message + "\n"))

			stdout, stderr, err = runScenarioCLI(dir, "list")
			Expect(err).NotTo(HaveOccurred(), stderr)
			Expect(strings.Split(strings.TrimSpace(stdout), "\n")).To(HaveLen(1))
		},
		Entry("unknown implementation", []string{"add", "unknown"}, `ScenarioNotFound: "unknown"`),
		Entry("compiled but not registered", []string{"enable", "science-report"}, `ScenarioNotFound: "science-report"`),
		Entry("missing name", []string{"add"}, "accepts 1 arg(s), received 0"),
		Entry("extra name", []string{"add", "science-report", "extra"}, "accepts 1 arg(s), received 2"),
	)
})

func runScenarioCLI(dir string, args ...string) (string, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	executable, err := os.Executable()
	if err != nil {
		return "", "", err
	}
	cmd := exec.CommandContext(ctx, executable, append([]string{"-test.run=^TestScenarioCLIProcess$", "--", "scenario"}, args...)...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "ASSISTENT_TEST_CLI=1", "REPORT_RENDERER=local-xlsx", "SQLITE_DATABASE_PATH="+filepath.Join(dir, "data", "state.db"))
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err = cmd.Run()
	return stdout.String(), stderr.String(), err
}

func TestScenarioCLIProcess(t *testing.T) {
	if os.Getenv("ASSISTENT_TEST_CLI") != "1" {
		return
	}
	os.Args = append(os.Args[:1], os.Args[3:]...)
	main()
	os.Exit(0)
}
