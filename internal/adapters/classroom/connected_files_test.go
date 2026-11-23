package classroom

import (
	"context"
	"fmt"
	. "github.com/onsi/ginkgo/v2"

	"github.com/alex/assistent/internal/domain/submission"
	. "github.com/onsi/gomega"
)

type connectedfilesGinkgoTest struct{}

func (*connectedfilesGinkgoTest) TempDir() string          { return GinkgoT().TempDir() }
func (*connectedfilesGinkgoTest) Setenv(key, value string) { GinkgoT().Setenv(key, value) }
func (*connectedfilesGinkgoTest) Cleanup(fn func())        { DeferCleanup(fn) }
func (*connectedfilesGinkgoTest) Helper()                  {}
func (*connectedfilesGinkgoTest) Fatal(args ...any)        { Expect(false).To(BeTrue(), fmt.Sprint(args...)) }
func (*connectedfilesGinkgoTest) Fatalf(format string, args ...any) {
	Expect(false).To(BeTrue(), fmt.Sprintf(format, args...))
}
func (*connectedfilesGinkgoTest) Errorf(format string, args ...any) {
	Expect(false).To(BeTrue(), fmt.Sprintf(format, args...))
}
func (*connectedfilesGinkgoTest) Run(name string, fn func(*connectedfilesGinkgoTest)) bool {
	By(name)
	fn(&connectedfilesGinkgoTest{})
	return true
}

func testSubmissionSourceNormalizesClassroomStates(t *connectedfilesGinkgoTest) {
	for _, tc := range []struct {
		state string
		want  submission.State
	}{
		{"NEW", submission.New},
		{"CREATED", submission.Created},
		{"RECLAIMED_BY_STUDENT", submission.Created},
		{"TURNED_IN", submission.Submitted},
		{"submitted", submission.Submitted},
		{"RETURNED", submission.Returned},
		{"UNRECOGNIZED", submission.Unknown},
	} {
		t.Run(tc.state, func(t *connectedfilesGinkgoTest) {
			g := NewWithT(t)
			ctx := context.Background()
			source := NewSubmissionSourceWithClient(&fakeClassroomClient{
				coursework:  []CourseWork{{ID: "task-1"}},
				submissions: []StudentSubmission{{ID: "report-1", State: tc.state}},
			})
			_, err := source.ListTasks(ctx, "course-1")
			g.Expect(err).NotTo(HaveOccurred())
			items, err := source.ListSubmissions(ctx, "task-1")
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(items).To(HaveLen(1))
			g.Expect(items[0].State).To(Equal(tc.want))
		})
	}
}

var _ = Describe("connected_files legacy tests", func() {
	It("runs the converted standard-library cases", func() {
		t := &connectedfilesGinkgoTest{}
		testSubmissionSourceNormalizesClassroomStates(t)
	})
})
