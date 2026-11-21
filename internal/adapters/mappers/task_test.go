package mappers_test

import (
	"github.com/alex/assistent/internal/adapters/mappers"
	"github.com/alex/assistent/internal/adapters/models"
	"github.com/alex/assistent/internal/domain/task"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("task row mapper", func() {
	It("maps every persisted task field", func() {
		row := models.TaskRow{
			ID: 4, Provider: "google-classroom", ExternalID: "task-1", CourseID: "course-1",
			Title: "Report", Description: "Details", CreatedAtExternal: "2026-01-02T03:04:05Z",
			UpdatedAtExternal: "2026-01-02T04:04:05Z", DueDate: "2026-01-03", DueTime: "03:04:05", TimeZone: "Europe/Kyiv",
			DueAt: "2026-01-03T03:04:05Z", DiscoveredAt: "2026-01-02T05:04:05Z", LastSeenAt: "2026-01-02T06:04:05Z", Status: "bound",
		}
		Expect(mappers.ToTask(row)).To(Equal(task.Task{
			ID: 4, Provider: "google-classroom", ExternalID: "task-1", CourseID: "course-1",
			Title: "Report", Description: "Details", CreatedAtExternal: "2026-01-02T03:04:05Z",
			UpdatedAtExternal: "2026-01-02T04:04:05Z", DueDate: "2026-01-03", DueTime: "03:04:05", TimeZone: "Europe/Kyiv",
			DueAt: "2026-01-03T03:04:05Z", DiscoveredAt: "2026-01-02T05:04:05Z", LastSeenAt: "2026-01-02T06:04:05Z", Status: task.Bound,
		}))
	})

	It("maps every binding and nested task field", func() {
		row := models.TaskBindingRow{
			ScenarioID: 2, ScenarioName: "science-report", ScenarioType: "ScienceReportScenario",
			TaskID: 4, Provider: "google-classroom", ExternalID: "task-1", CourseID: "course-1",
			Title: "Report", Description: "Details", CreatedAtExternal: "2026-01-02T03:04:05Z", UpdatedAtExternal: "2026-01-02T04:04:05Z", DueDate: "2026-01-03", DueTime: "03:04:05", TimeZone: "Europe/Kyiv", DueAt: "2026-01-03T03:04:05Z", DiscoveredAt: "2026-01-02T05:04:05Z", LastSeenAt: "2026-01-02T06:04:05Z",
			TaskStatus: "bound", Status: "active", BoundAt: "2026-01-02T03:04:05Z",
			UnboundAt: "",
		}
		Expect(mappers.ToTaskBinding(row)).To(Equal(task.Binding{
			ScenarioID: 2, ScenarioName: "science-report", ScenarioType: "ScienceReportScenario",
			Status: "active", BoundAt: "2026-01-02T03:04:05Z", UnboundAt: "",
			Task: task.Task{ID: 4, Provider: "google-classroom", ExternalID: "task-1", CourseID: "course-1", Title: "Report", Description: "Details", CreatedAtExternal: "2026-01-02T03:04:05Z", UpdatedAtExternal: "2026-01-02T04:04:05Z", DueDate: "2026-01-03", DueTime: "03:04:05", TimeZone: "Europe/Kyiv", DueAt: "2026-01-03T03:04:05Z", DiscoveredAt: "2026-01-02T05:04:05Z", LastSeenAt: "2026-01-02T06:04:05Z", Status: task.Bound},
		}))
	})
})
