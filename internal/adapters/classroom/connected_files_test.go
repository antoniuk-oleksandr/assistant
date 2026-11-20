package classroom

import (
	"context"
	"testing"

	"github.com/alex/assistent/internal/domain/submission"
	. "github.com/onsi/gomega"
)

func TestSubmissionSourceNormalizesClassroomStates(t *testing.T) {
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
		t.Run(tc.state, func(t *testing.T) {
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
