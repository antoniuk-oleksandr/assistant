package ports

import (
	"context"
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
