package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
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
	BeforeEach(func() { dir = GinkgoT().TempDir() })

	It("configures the report workflow across separate invocations", func() {
		run := func(args ...string) string {
			GinkgoHelper()
			stdout, stderr, err := runCLI(dir, args...)
			Expect(err).NotTo(HaveOccurred(), "command %v: %s", args, stderr)
			Expect(stderr).To(BeEmpty())
			return stdout
		}
		run("course", "add", "123")
		run("report", "configure", "123")
		Expect(run("report", "task-list")).To(ContainSubstring("TASK ID"))
	})

	DescribeTable("rejects invalid report commands", func(args []string, message string) {
		stdout, stderr, err := runCLI(dir, append([]string{"report"}, args...)...)
		var exit *exec.ExitError
		Expect(err).To(BeAssignableToTypeOf(exit))
		Expect(err.(*exec.ExitError).ExitCode()).To(Equal(1))
		Expect(stdout).To(BeEmpty())
		Expect(stderr).To(Equal(message + "\n"))
	},
		Entry("missing course", []string{"configure"}, "accepts 1 arg(s), received 0"),
		Entry("extra course", []string{"configure", "123", "extra"}, "accepts 1 arg(s), received 2"),
	)
})

func runCLI(dir string, args ...string) (string, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	executable, err := os.Executable()
	if err != nil {
		return "", "", err
	}
	cmd := exec.CommandContext(ctx, executable, append([]string{"-test.run=^TestCLIProcess$", "--"}, args...)...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "ASSISTENT_TEST_CLI=1", "REPORT_RENDERER=local-xlsx", "SQLITE_DATABASE_PATH="+filepath.Join(dir, "data", "state.db"))
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err = cmd.Run()
	return stdout.String(), stderr.String(), err
}

func TestCLIProcess(t *testing.T) {
	if os.Getenv("ASSISTENT_TEST_CLI") != "1" {
		return
	}
	os.Args = append(os.Args[:1], os.Args[3:]...)
	main()
	os.Exit(0)
}
