package storage_test

import (
	"context"
	"database/sql"
	"github.com/alex/assistent/internal/adapters/storage"
	"github.com/alex/assistent/internal/domain/ports"
	"github.com/alex/assistent/internal/domain/teacher"
	"github.com/alex/assistent/internal/platform/config"
	"github.com/alex/assistent/internal/platform/database"
	"go.uber.org/fx"
	"path/filepath"
	"testing"
	"time"
)

func TestTeacherRepositoryRoundTrip(t *testing.T) {
	ctx := context.Background()
	var db *sql.DB
	var repo ports.TeacherRepository
	var contacts ports.TeacherContactRepository
	app := fx.New(fx.NopLogger, fx.Supply(config.DatabaseConfig{SQLitePath: filepath.Join(t.TempDir(), "state.db")}), fx.Provide(database.NewSQLite, storage.NewTeacherRepository, storage.NewTeacherContactRepository), fx.Populate(&db, &repo, &contacts))
	if err := app.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer app.Stop(ctx)
	now := time.Now().UTC()
	person := teacher.Recipient{Name: "Teacher", Email: "teacher@example.com", ClassroomID: "class-1", Status: teacher.Active, CreatedAt: now, UpdatedAt: now}
	if err := repo.Create(ctx, &person); err != nil {
		t.Fatal(err)
	}
	got, err := repo.Get(ctx, person.ID)
	if err != nil || got.Email != person.Email {
		t.Fatalf("got=%#v err=%v", got, err)
	}
	got.Status = teacher.Disabled
	if err := repo.Update(ctx, &got); err != nil {
		t.Fatal(err)
	}
	list, err := repo.List(ctx)
	if err != nil || len(list) != 1 {
		t.Fatalf("list=%#v err=%v", list, err)
	}
	channel := teacher.Channel{RecipientID: person.ID, Kind: "email", Destination: person.Email, Status: teacher.Active, CreatedAt: now, UpdatedAt: now}
	if err := repo.CreateChannel(ctx, &channel); err != nil {
		t.Fatal(err)
	}
	active, err := repo.ListActiveChannels(ctx, person.ID)
	if err != nil || len(active) != 1 {
		t.Fatalf("active=%#v err=%v", active, err)
	}
	channel.Status = teacher.Disabled
	if err := repo.UpdateChannel(ctx, &channel); err != nil {
		t.Fatal(err)
	}
	active, err = repo.ListActiveChannels(ctx, person.ID)
	if err != nil || len(active) != 0 {
		t.Fatalf("disabled active=%#v err=%v", active, err)
	}
}

func TestTeacherRepositoryReportsMissingAndClosedDatabase(t *testing.T) {
	ctx := context.Background()
	var db *sql.DB
	var repo ports.TeacherRepository
	var contacts ports.TeacherContactRepository
	app := fx.New(fx.NopLogger, fx.Supply(config.DatabaseConfig{SQLitePath: filepath.Join(t.TempDir(), "state.db")}), fx.Provide(database.NewSQLite, storage.NewTeacherRepository, storage.NewTeacherContactRepository), fx.Populate(&db, &repo, &contacts))
	if err := app.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer app.Stop(ctx)
	if _, err := repo.Get(ctx, 999); err == nil {
		t.Fatal("missing teacher unexpectedly succeeded")
	}
	if err := repo.Update(ctx, &teacher.Recipient{ID: 999, Status: teacher.Disabled, UpdatedAt: time.Now()}); err == nil {
		t.Fatal("missing teacher update unexpectedly succeeded")
	}
	if err := repo.UpdateChannel(ctx, &teacher.Channel{ID: 999, Status: teacher.Disabled, UpdatedAt: time.Now()}); err == nil {
		t.Fatal("missing channel update unexpectedly succeeded")
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.List(ctx); err == nil {
		t.Fatal("closed list unexpectedly succeeded")
	}
	if _, err := repo.ListActiveChannels(ctx, 1); err == nil {
		t.Fatal("closed channel list unexpectedly succeeded")
	}
	if err := repo.Create(ctx, &teacher.Recipient{Name: "x", Email: "x@example.com", Status: teacher.Active, CreatedAt: time.Now()}); err == nil {
		t.Fatal("closed teacher create unexpectedly succeeded")
	}
	if err := repo.CreateChannel(ctx, &teacher.Channel{RecipientID: 1, Kind: "email", Destination: "x", Status: teacher.Active, CreatedAt: time.Now()}); err == nil {
		t.Fatal("closed channel create unexpectedly succeeded")
	}
	if err := contacts.RegisterContact(ctx, &teacher.Recipient{Name: "x", Email: "x@example.com", Status: teacher.Active, CreatedAt: time.Now()}, &teacher.Channel{Kind: "phone", Destination: "x", Status: teacher.Active, CreatedAt: time.Now()}); err == nil {
		t.Fatal("closed contact registration unexpectedly succeeded")
	}
}

func TestTeacherContactRepositoryRegistersRecipientAndPhoneAtomically(t *testing.T) {
	ctx := context.Background()
	var db *sql.DB
	var repo ports.TeacherContactRepository
	app := fx.New(fx.NopLogger, fx.Supply(config.DatabaseConfig{SQLitePath: filepath.Join(t.TempDir(), "state.db")}), fx.Provide(database.NewSQLite, storage.NewTeacherContactRepository), fx.Populate(&db, &repo))
	if err := app.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer app.Stop(ctx)
	now := time.Now().UTC()
	recipient := teacher.Recipient{Name: "Teacher", Email: "teacher@example.com", ClassroomID: "user-1", Status: teacher.Active, CreatedAt: now, UpdatedAt: now}
	channel := teacher.Channel{Kind: "phone", Destination: "+380000000", Status: teacher.Active, CreatedAt: now, UpdatedAt: now}
	if err := repo.RegisterContact(ctx, &recipient, &channel); err != nil {
		t.Fatal(err)
	}
	if recipient.ID == 0 || channel.ID == 0 || channel.RecipientID != recipient.ID {
		t.Fatalf("recipient=%#v channel=%#v", recipient, channel)
	}
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM notification_channels WHERE teacher_id=?`, recipient.ID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("channel count=%d err=%v", count, err)
	}
}
