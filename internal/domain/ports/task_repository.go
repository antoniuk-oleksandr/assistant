package ports

import (
	"context"
	"github.com/alex/assistent/internal/domain/scenario"
	"github.com/alex/assistent/internal/domain/task"
)

type TaskRepository interface {
	Register(context.Context, task.Task) error
	List(context.Context) ([]task.Task, error)
}

type TaskBindingRepository interface {
	Bind(context.Context, string, string) error
	Unbind(context.Context, string) error
	List(context.Context, string) ([]task.Binding, error)
	Active(context.Context) ([]task.Binding, error)
}

type ScenarioCourseBindingRepository interface {
	Bind(context.Context, string, string) error
	Unbind(context.Context, string, string) error
	List(context.Context, string) ([]scenario.CourseBinding, error)
}

type ScenarioCourseDataRepository interface {
	Set(context.Context, string, string, map[string]string) error
	List(context.Context, string, string) ([]scenario.CourseDataField, error)
}
