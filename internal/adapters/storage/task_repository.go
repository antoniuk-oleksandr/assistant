package storage

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/alex/assistent/internal/adapters/mappers"
	"github.com/alex/assistent/internal/adapters/models"
	"github.com/alex/assistent/internal/domain/ports"
	"github.com/alex/assistent/internal/domain/scenario"
	"github.com/alex/assistent/internal/domain/task"
)

type SQLiteTaskRepository struct{ db *sql.DB }

func NewTaskRepository(db *sql.DB) ports.TaskRepository { return &SQLiteTaskRepository{db: db} }

func (r *SQLiteTaskRepository) Register(ctx context.Context, item task.Task) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO tasks (provider, external_task_id, course_id, title, description, created_at_external, due_at, discovered_at, last_seen_at, status)
VALUES (?, ?, ?, ?, ?, NULLIF(?, ''), NULLIF(?, ''), `+sqliteNowUTC+`, `+sqliteNowUTC+`, ?)
ON CONFLICT(provider, course_id, external_task_id) DO UPDATE SET title=excluded.title, description=excluded.description, created_at_external=excluded.created_at_external, due_at=excluded.due_at, last_seen_at=excluded.last_seen_at`, item.Provider, item.ExternalID, item.CourseID, item.Title, item.Description, item.CreatedAtExternal, item.DueAt, task.Discovered)
	if err != nil {
		return fmt.Errorf("register task %q: %w", item.ExternalID, err)
	}
	return nil
}

func (r *SQLiteTaskRepository) List(ctx context.Context) ([]task.Task, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, provider, external_task_id, course_id, title, COALESCE(description, ''), COALESCE(created_at_external, ''), COALESCE(due_at, ''), discovered_at, last_seen_at, status FROM tasks ORDER BY course_id, external_task_id`)
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	defer rows.Close()
	result := make([]task.Task, 0)
	for rows.Next() {
		var row models.TaskRow
		if err := rows.Scan(&row.ID, &row.Provider, &row.ExternalID, &row.CourseID, &row.Title, &row.Description, &row.CreatedAtExternal, &row.DueAt, &row.DiscoveredAt, &row.LastSeenAt, &row.Status); err != nil {
			return nil, fmt.Errorf("scan task: %w", err)
		}
		result = append(result, mappers.ToTask(row))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read tasks: %w", err)
	}
	return result, nil
}

type SQLiteTaskBindingRepository struct{ db *sql.DB }

func NewTaskBindingRepository(db *sql.DB) ports.TaskBindingRepository {
	return &SQLiteTaskBindingRepository{db: db}
}

type SQLiteScenarioCourseBindingRepository struct{ db *sql.DB }

func NewScenarioCourseBindingRepository(db *sql.DB) ports.ScenarioCourseBindingRepository {
	return &SQLiteScenarioCourseBindingRepository{db: db}
}

func (r *SQLiteScenarioCourseBindingRepository) Bind(ctx context.Context, scenarioName, courseID string) error {
	var scenarioID, classroomCourseID int64
	if err := r.db.QueryRowContext(ctx, `SELECT id FROM scenarios WHERE name=? AND status='active'`, scenarioName).Scan(&scenarioID); err != nil {
		return fmt.Errorf("find active scenario %q: %w", scenarioName, err)
	}
	if err := r.db.QueryRowContext(ctx, `SELECT id FROM classroom_courses WHERE external_course_id=? AND status='active'`, courseID).Scan(&classroomCourseID); err != nil {
		return fmt.Errorf("find active course %q: %w", courseID, err)
	}
	if _, err := r.db.ExecContext(ctx, `INSERT INTO scenario_courses (scenario_id, course_id, bound_at, status) VALUES (?, ?, `+sqliteNowUTC+`, 'active') ON CONFLICT(scenario_id, course_id) DO UPDATE SET status='active', unbound_at=NULL`, scenarioID, classroomCourseID); err != nil {
		return fmt.Errorf("bind course %q: %w", courseID, err)
	}
	return nil
}

func (r *SQLiteScenarioCourseBindingRepository) Unbind(ctx context.Context, scenarioName, courseID string) error {
	result, err := r.db.ExecContext(ctx, `UPDATE scenario_courses SET status='disabled', unbound_at=`+sqliteNowUTC+` WHERE scenario_id=(SELECT id FROM scenarios WHERE name=?) AND course_id=(SELECT id FROM classroom_courses WHERE external_course_id=?) AND status='active'`, scenarioName, courseID)
	if err != nil {
		return fmt.Errorf("unbind course %q: %w", courseID, err)
	}
	if count, _ := result.RowsAffected(); count == 0 {
		return fmt.Errorf("course binding not found: %q", courseID)
	}
	return nil
}

func (r *SQLiteScenarioCourseBindingRepository) List(ctx context.Context, scenarioName string) ([]scenario.CourseBinding, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT s.name, s.implementation_type, c.external_course_id, sc.status FROM scenario_courses sc JOIN scenarios s ON s.id=sc.scenario_id JOIN classroom_courses c ON c.id=sc.course_id WHERE s.name=? AND sc.status='active' ORDER BY c.external_course_id`, scenarioName)
	if err != nil {
		return nil, fmt.Errorf("list scenario course bindings: %w", err)
	}
	defer rows.Close()
	result := make([]scenario.CourseBinding, 0)
	for rows.Next() {
		var item scenario.CourseBinding
		if err := rows.Scan(&item.ScenarioName, &item.ScenarioType, &item.CourseID, &item.Status); err != nil {
			return nil, fmt.Errorf("scan scenario course binding: %w", err)
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read scenario course bindings: %w", err)
	}
	return result, nil
}

const bindingSelect = `SELECT s.id, s.name, s.implementation_type, t.id, t.provider, t.external_task_id, t.course_id, t.title, COALESCE(t.description, ''), COALESCE(t.due_at, ''), t.status, st.status, st.bound_at, COALESCE(st.unbound_at, '') FROM scenario_tasks st JOIN scenarios s ON s.id=st.scenario_id JOIN tasks t ON t.id=st.task_id`

func (r *SQLiteTaskBindingRepository) Bind(ctx context.Context, scenarioName, externalTaskID string) error {
	var scenarioID, taskID int64
	if err := r.db.QueryRowContext(ctx, `SELECT id FROM scenarios WHERE name = ? AND status = 'active'`, scenarioName).Scan(&scenarioID); err != nil {
		return fmt.Errorf("find active scenario %q: %w", scenarioName, err)
	}
	var taskCount int
	if err := r.db.QueryRowContext(ctx, `SELECT count(*) FROM tasks WHERE external_task_id = ?`, externalTaskID).Scan(&taskCount); err != nil {
		return fmt.Errorf("find task %q: %w", externalTaskID, err)
	}
	if taskCount == 0 {
		return fmt.Errorf("find task %q: %w", externalTaskID, task.ErrNotFound)
	}
	if taskCount > 1 {
		return fmt.Errorf("find task %q: %w", externalTaskID, task.ErrAmbiguous)
	}
	if err := r.db.QueryRowContext(ctx, `SELECT id FROM tasks WHERE external_task_id = ?`, externalTaskID).Scan(&taskID); err != nil {
		return fmt.Errorf("find task %q: %w", externalTaskID, err)
	}
	var courseBound bool
	if err := r.db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM scenario_courses sc JOIN classroom_courses c ON c.id=sc.course_id WHERE sc.scenario_id=? AND sc.status='active' AND c.external_course_id=(SELECT course_id FROM tasks WHERE id=?))`, scenarioID, taskID).Scan(&courseBound); err != nil {
		return fmt.Errorf("check scenario course binding: %w", err)
	}
	if !courseBound {
		return fmt.Errorf("scenario %q is not bound to task course", scenarioName)
	}
	_, err := r.db.ExecContext(ctx, `INSERT INTO scenario_tasks (scenario_id, task_id, bound_at, status) VALUES (?, ?, `+sqliteNowUTC+`, 'active') ON CONFLICT(scenario_id, task_id) DO UPDATE SET status='active', unbound_at=NULL`, scenarioID, taskID)
	if err != nil {
		return fmt.Errorf("bind task %q: %w", externalTaskID, err)
	}
	_, err = r.db.ExecContext(ctx, `UPDATE tasks SET status='bound' WHERE id=?`, taskID)
	return err
}

func (r *SQLiteTaskBindingRepository) Unbind(ctx context.Context, externalTaskID string) error {
	var taskCount int
	if err := r.db.QueryRowContext(ctx, `SELECT count(*) FROM tasks WHERE external_task_id = ?`, externalTaskID).Scan(&taskCount); err != nil {
		return fmt.Errorf("find task %q: %w", externalTaskID, err)
	}
	if taskCount == 0 {
		return fmt.Errorf("find task %q: %w", externalTaskID, task.ErrNotFound)
	}
	if taskCount > 1 {
		return fmt.Errorf("find task %q: %w", externalTaskID, task.ErrAmbiguous)
	}
	result, err := r.db.ExecContext(ctx, `UPDATE scenario_tasks SET status='disabled', unbound_at=`+sqliteNowUTC+` WHERE task_id = (SELECT id FROM tasks WHERE external_task_id = ?) AND status='active'`, externalTaskID)
	if err != nil {
		return fmt.Errorf("unbind task %q: %w", externalTaskID, err)
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		var bindingExists bool
		if err := r.db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM scenario_tasks WHERE task_id = (SELECT id FROM tasks WHERE external_task_id = ?))`, externalTaskID).Scan(&bindingExists); err != nil {
			return fmt.Errorf("find binding for task %q: %w", externalTaskID, err)
		}
		if !bindingExists {
			return fmt.Errorf("%w: %q", task.ErrNotFound, externalTaskID)
		}
		return nil
	}
	_, err = r.db.ExecContext(ctx, `UPDATE tasks SET status=CASE WHEN EXISTS (SELECT 1 FROM scenario_tasks WHERE task_id=tasks.id AND status='active') THEN 'bound' ELSE 'discovered' END WHERE external_task_id=?`, externalTaskID)
	if err != nil {
		return fmt.Errorf("update task %q status: %w", externalTaskID, err)
	}
	return nil
}

func (r *SQLiteTaskBindingRepository) List(ctx context.Context, scenarioName string) ([]task.Binding, error) {
	rows, err := r.db.QueryContext(ctx, bindingSelect+` WHERE s.name=? AND st.status='active' ORDER BY t.external_task_id`, scenarioName)
	return scanBindings(rows, err)
}

func (r *SQLiteTaskBindingRepository) Active(ctx context.Context) ([]task.Binding, error) {
	rows, err := r.db.QueryContext(ctx, bindingSelect+` JOIN scenario_courses sc ON sc.scenario_id=s.id JOIN classroom_courses c ON c.id=sc.course_id WHERE s.status='active' AND st.status='active' AND sc.status='active' AND c.external_course_id=t.course_id AND t.status='bound' ORDER BY s.name, t.external_task_id`)
	return scanBindings(rows, err)
}

func scanBindings(rows *sql.Rows, err error) ([]task.Binding, error) {
	if err != nil {
		return nil, fmt.Errorf("list task bindings: %w", err)
	}
	defer rows.Close()
	result := make([]task.Binding, 0)
	for rows.Next() {
		var row models.TaskBindingRow
		if err := rows.Scan(&row.ScenarioID, &row.ScenarioName, &row.ScenarioType, &row.TaskID, &row.Provider, &row.ExternalID, &row.CourseID, &row.Title, &row.Description, &row.DueAt, &row.TaskStatus, &row.Status, &row.BoundAt, &row.UnboundAt); err != nil {
			return nil, fmt.Errorf("scan task binding: %w", err)
		}
		result = append(result, mappers.ToTaskBinding(row))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read task bindings: %w", err)
	}
	return result, nil
}
