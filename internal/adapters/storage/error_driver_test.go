package storage_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"time"

	"github.com/alex/assistent/internal/adapters/storage"
	"github.com/alex/assistent/internal/domain/classroom"
	"github.com/alex/assistent/internal/domain/notification"
	"github.com/alex/assistent/internal/domain/ports"
	"github.com/alex/assistent/internal/domain/report"
	"github.com/alex/assistent/internal/domain/scenario"
	"github.com/alex/assistent/internal/domain/submission"
	"github.com/alex/assistent/internal/domain/task"
	"github.com/alex/assistent/internal/domain/teacher"
	. "github.com/onsi/ginkgo/v2"
)

type ginkgoTest struct{}

func (*ginkgoTest) Fatal(args ...any)                 { Fail(fmt.Sprint(args...)) }
func (*ginkgoTest) Fatalf(format string, args ...any) { Fail(fmt.Sprintf(format, args...)) }
func (*ginkgoTest) Errorf(format string, args ...any) { Fail(fmt.Sprintf(format, args...)) }
func (*ginkgoTest) Run(name string, fn func(*ginkgoTest)) bool {
	By(name)
	fn(&ginkgoTest{})
	return true
}

var errStorageDriver = errors.New("storage driver failure")

type failingStorageDriver struct{}
type failingStorageConn struct{}

func (failingStorageDriver) Open(string) (driver.Conn, error)  { return failingStorageConn{}, nil }
func (failingStorageConn) Prepare(string) (driver.Stmt, error) { return nil, errStorageDriver }
func (failingStorageConn) Close() error                        { return nil }
func (failingStorageConn) Begin() (driver.Tx, error)           { return nil, errStorageDriver }
func (failingStorageConn) BeginTx(context.Context, driver.TxOptions) (driver.Tx, error) {
	return nil, errStorageDriver
}
func (failingStorageConn) ExecContext(context.Context, string, []driver.NamedValue) (driver.Result, error) {
	return nil, errStorageDriver
}
func (failingStorageConn) QueryContext(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
	return nil, errStorageDriver
}

func testSQLiteRepositoriesWrapDriverFailures(t *ginkgoTest) {
	driverName := "assistent-failing-storage-driver"
	sql.Register(driverName, failingStorageDriver{})
	db, err := sql.Open(driverName, "")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	now := time.Now().UTC()
	item := task.Task{ExternalID: "task", CourseID: "course", Title: "Task"}
	implementation := scenario.Implementation{Name: "scenario", Type: "type"}
	reminder := notification.Reminder{TaskID: 1, PersonID: "person", Channel: "email", Kind: "missing", CreatedAt: now}

	course := storage.NewClassroomCourseRepository(db)
	if err := course.Register(ctx, classroom.Course{ExternalID: "course"}); err == nil {
		t.Fatal("course Register succeeded")
	}
	if _, err := course.List(ctx); err == nil {
		t.Fatal("course List succeeded")
	}
	if err := course.Disable(ctx, "course"); err == nil {
		t.Fatal("course Disable succeeded")
	}

	scenarios := storage.NewScenarioRepository(db)
	if err := scenarios.Create(ctx, implementation); err == nil {
		t.Fatal("scenario Create succeeded")
	}
	if _, err := scenarios.List(ctx); err == nil {
		t.Fatal("scenario List succeeded")
	}
	if _, err := scenarios.ListActive(ctx); err == nil {
		t.Fatal("scenario ListActive succeeded")
	}
	if err := scenarios.SetStatus(ctx, implementation.Name, scenario.Active); err == nil {
		t.Fatal("scenario SetStatus succeeded")
	}
	if err := scenarios.Remove(ctx, implementation.Name); err == nil {
		t.Fatal("scenario Remove succeeded")
	}

	tasks := storage.NewTaskRepository(db)
	if err := tasks.Register(ctx, item); err == nil {
		t.Fatal("task Register succeeded")
	}
	if _, err := tasks.List(ctx); err == nil {
		t.Fatal("task List succeeded")
	}
	bindings := storage.NewTaskBindingRepository(db)
	if err := bindings.Bind(ctx, implementation.Name, item.ExternalID); err == nil {
		t.Fatal("task Bind succeeded")
	}
	if err := bindings.Unbind(ctx, item.ExternalID); err == nil {
		t.Fatal("task Unbind succeeded")
	}
	if _, err := bindings.List(ctx, implementation.Name); err == nil {
		t.Fatal("task binding List succeeded")
	}
	if _, err := bindings.Active(ctx); err == nil {
		t.Fatal("task binding Active succeeded")
	}
	courseBindings := storage.NewScenarioCourseBindingRepository(db)
	if err := courseBindings.Bind(ctx, implementation.Name, "course"); err == nil {
		t.Fatal("course Bind succeeded")
	}
	if err := courseBindings.Unbind(ctx, implementation.Name, "course"); err == nil {
		t.Fatal("course Unbind succeeded")
	}
	if _, err := courseBindings.List(ctx, implementation.Name); err == nil {
		t.Fatal("course binding List succeeded")
	}
	data := storage.NewScenarioCourseDataRepository(db)
	if err := data.Set(ctx, implementation.Name, "course", map[string]string{"field": "value"}); err == nil {
		t.Fatal("course data Set succeeded")
	}
	if _, err := data.List(ctx, implementation.Name, "course"); err == nil {
		t.Fatal("course data List succeeded")
	}

	submissions := storage.NewSubmissionRepository(db)
	if err := submissions.Upsert(ctx, submission.Submission{TaskExternalID: "task", ExternalID: "submission"}); err == nil {
		t.Fatal("submission Upsert succeeded")
	}
	if _, err := submissions.ListByTask(ctx, 1); err == nil {
		t.Fatal("submission List succeeded")
	}
	participants := storage.NewParticipantRosterRepository(db)
	if err := participants.Replace(ctx, 1, nil); err == nil {
		t.Fatal("participant Replace succeeded")
	}
	if _, err := participants.List(ctx, 1); err == nil {
		t.Fatal("participant List succeeded")
	}
	missing := storage.NewMissingSubmitterRepository(db)
	if err := missing.Reconcile(ctx, 1, nil); err == nil {
		t.Fatal("missing Reconcile succeeded")
	}
	if _, err := missing.List(ctx, 1); err == nil {
		t.Fatal("missing List succeeded")
	}

	runs := storage.NewReportRunRepository(db)
	run := report.NewRun(1, 1, now)
	if err := runs.Create(ctx, &run); err == nil {
		t.Fatal("run Create succeeded")
	}
	if err := runs.Update(ctx, &run); err == nil {
		t.Fatal("run Update succeeded")
	}
	if _, err := runs.Get(ctx, 1); err == nil {
		t.Fatal("run Get succeeded")
	}
	if lookup, ok := runs.(interface {
		GetByScenarioTask(context.Context, int64, int64) (report.Run, error)
	}); ok {
		if _, err := lookup.GetByScenarioTask(ctx, 1, 1); err == nil {
			t.Fatal("run lookup succeeded")
		}
	}
	outputs := storage.NewReportOutputRepository(db)
	if err := outputs.Create(ctx, ports.ReportOutput{ReportRunID: 1, Format: "local", Path: "report", CreatedAt: now}); err == nil {
		t.Fatal("output Create succeeded")
	}
	results := storage.NewReportResultRepository(db)
	result := report.NewResult(1, 1, 1, scenario.ReportData{}, now)
	if err := results.Create(ctx, &result); err == nil {
		t.Fatal("result Create succeeded")
	}
	if err := results.Update(ctx, &result); err == nil {
		t.Fatal("result Update succeeded")
	}
	if _, err := results.GetLatestByRun(ctx, 1); err == nil {
		t.Fatal("result Get succeeded")
	}
	deliveries := storage.NewReportDeliveryRepository(db)
	delivery := report.Delivery{ResultID: 1, Channel: "email", Destination: "person", CreatedAt: now}
	if err := deliveries.Create(ctx, &delivery); err == nil {
		t.Fatal("delivery Create succeeded")
	}
	if err := deliveries.Update(ctx, &delivery); err == nil {
		t.Fatal("delivery Update succeeded")
	}
	if _, err := deliveries.GetByResult(ctx, 1); err == nil {
		t.Fatal("delivery List succeeded")
	}
	if _, err := deliveries.GetByResultTarget(ctx, 1, "email", "person"); err == nil {
		t.Fatal("delivery target succeeded")
	}

	reminders := storage.NewReminderRepository(db)
	if err := reminders.Create(ctx, &reminder); err == nil {
		t.Fatal("reminder Create succeeded")
	}
	if err := reminders.Update(ctx, &reminder); err == nil {
		t.Fatal("reminder Update succeeded")
	}
	if _, err := reminders.Get(ctx, 1, "person", "email", "missing"); err == nil {
		t.Fatal("reminder Get succeeded")
	}
	if _, err := reminders.GetByID(ctx, 1); err == nil {
		t.Fatal("reminder GetByID succeeded")
	}
	if _, err := reminders.ListByTask(ctx, 1); err == nil {
		t.Fatal("reminder List succeeded")
	}
	if _, err := reminders.ListRetryable(ctx, now); err == nil {
		t.Fatal("reminder retry List succeeded")
	}
	teachers := storage.NewTeacherRepository(db)
	if err := teachers.Create(ctx, &teacher.Recipient{Email: "person@example.com", CreatedAt: now}); err == nil {
		t.Fatal("teacher Create succeeded")
	}
	if err := teachers.Update(ctx, &teacher.Recipient{ID: 1, UpdatedAt: now}); err == nil {
		t.Fatal("teacher Update succeeded")
	}
	if _, err := teachers.Get(ctx, 1); err == nil {
		t.Fatal("teacher Get succeeded")
	}
	if _, err := teachers.List(ctx); err == nil {
		t.Fatal("teacher List succeeded")
	}
	if err := teachers.CreateChannel(ctx, &teacher.Channel{RecipientID: 1, CreatedAt: now, UpdatedAt: now}); err == nil {
		t.Fatal("channel Create succeeded")
	}
	if err := teachers.UpdateChannel(ctx, &teacher.Channel{ID: 1, UpdatedAt: now}); err == nil {
		t.Fatal("channel Update succeeded")
	}
	if _, err := teachers.ListActiveChannels(ctx, 1); err == nil {
		t.Fatal("channel List succeeded")
	}
	pairings := storage.NewTelegramPairingRepository(db)
	if err := pairings.CreateTelegramPairing(ctx, &teacher.TelegramPairing{RecipientID: 1, ExpiresAt: now}); err == nil {
		t.Fatal("pairing Create succeeded")
	}
	if _, err := pairings.GetTelegramPairing(ctx, "hash"); err == nil {
		t.Fatal("pairing Get succeeded")
	}
	if err := pairings.ConsumeTelegramPairing(ctx, 1, now); err == nil {
		t.Fatal("pairing Consume succeeded")
	}
}

var _ = Describe("SQLite repository driver failures", func() {
	It("wraps driver failures", func() { testSQLiteRepositoriesWrapDriverFailures(&ginkgoTest{}) })
})
