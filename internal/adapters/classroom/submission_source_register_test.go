package classroom

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestSubmissionSourceRegisterTaskCourse(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Submission source registration")
}

var _ = Describe("Submission source task registration", func() {
	It("stores the course for a task", func() {
		source := NewSubmissionSourceWithClient(nil)
		source.RegisterTaskCourse("task-1", "course-1")
		source.mu.RLock()
		defer source.mu.RUnlock()
		Expect(source.courses["task-1"]).To(Equal("course-1"))
	})
})
