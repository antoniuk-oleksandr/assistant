package mappers

import (
	"github.com/alex/assistent/internal/adapters/models"
	"github.com/alex/assistent/internal/domain/task"
)

func ToTask(row models.TaskRow) task.Task {
	return task.Task{ID: row.ID, Provider: row.Provider, ExternalID: row.ExternalID, CourseID: row.CourseID, Title: row.Title, Description: row.Description, CreatedAtExternal: row.CreatedAtExternal, UpdatedAtExternal: row.UpdatedAtExternal, DueDate: row.DueDate, DueTime: row.DueTime, TimeZone: row.TimeZone, DueAt: row.DueAt, DiscoveredAt: row.DiscoveredAt, LastSeenAt: row.LastSeenAt, Status: task.Status(row.Status)}
}

func ToTaskBinding(row models.TaskBindingRow) task.Binding {
	return task.Binding{ScenarioID: row.ScenarioID, ScenarioName: row.ScenarioName, ScenarioType: row.ScenarioType, Status: row.Status, BoundAt: row.BoundAt, UnboundAt: row.UnboundAt, Task: task.Task{ID: row.TaskID, Provider: row.Provider, ExternalID: row.ExternalID, CourseID: row.CourseID, Title: row.Title, Description: row.Description, CreatedAtExternal: row.CreatedAtExternal, UpdatedAtExternal: row.UpdatedAtExternal, DueDate: row.DueDate, DueTime: row.DueTime, TimeZone: row.TimeZone, DueAt: row.DueAt, DiscoveredAt: row.DiscoveredAt, LastSeenAt: row.LastSeenAt, Status: task.Status(row.TaskStatus)}}
}
