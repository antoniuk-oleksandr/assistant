package classroom

import "testing"

func TestSubmissionSourceRegisterTaskCourseStoresCourse(t *testing.T) {
	source := NewSubmissionSourceWithClient(nil)
	source.RegisterTaskCourse("task-1", "course-1")
	source.mu.RLock()
	defer source.mu.RUnlock()
	if source.courses["task-1"] != "course-1" {
		t.Fatalf("registered course = %q", source.courses["task-1"])
	}
}
