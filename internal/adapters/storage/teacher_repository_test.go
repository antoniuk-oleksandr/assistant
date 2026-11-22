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
	var pairings ports.TelegramPairingRepository
	app := fx.New(fx.NopLogger, fx.Supply(config.DatabaseConfig{SQLitePath: filepath.Join(t.TempDir(), "state.db")}), fx.Provide(database.NewSQLite, storage.NewTeacherRepository, storage.NewTelegramPairingRepository), fx.Populate(&db, &repo, &pairings))
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

func TestTelegramPairingRepositoryRoundTrip(t *testing.T) {
	ctx := context.Background()
	var db *sql.DB
	var teachers ports.TeacherRepository
	var pairings ports.TelegramPairingRepository
	app := fx.New(fx.NopLogger,
		fx.Supply(config.DatabaseConfig{SQLitePath: filepath.Join(t.TempDir(), "state.db")}),
		fx.Provide(database.NewSQLite, storage.NewTeacherRepository, storage.NewTelegramPairingRepository),
		fx.Populate(&db, &teachers, &pairings),
	)
	if err := app.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer app.Stop(ctx)
	now := time.Now().UTC()
	person := teacher.Recipient{Name: "Teacher", Email: "teacher@example.com", Status: teacher.Active, CreatedAt: now, UpdatedAt: now}
	if err := teachers.Create(ctx, &person); err != nil {
		t.Fatal(err)
	}
	pairing := teacher.TelegramPairing{RecipientID: person.ID, TokenHash: "hash", ExpiresAt: now.Add(time.Hour)}
	if err := pairings.CreateTelegramPairing(ctx, &pairing); err != nil {
		t.Fatal(err)
	}
	got, err := pairings.GetTelegramPairing(ctx, "hash")
	if err != nil || got.RecipientID != person.ID || got.TokenHash != "hash" {
		t.Fatalf("pairing=%#v err=%v", got, err)
	}
	if err := pairings.ConsumeTelegramPairing(ctx, pairing.ID, now); err != nil {
		t.Fatal(err)
	}
	got, err = pairings.GetTelegramPairing(ctx, "hash")
	if err != nil || got.UsedAt == nil {
		t.Fatalf("consumed pairing=%#v err=%v", got, err)
	}
	if err := pairings.ConsumeTelegramPairing(ctx, pairing.ID, now); err != ports.ErrTeacherNotFound {
		t.Fatalf("second consume error = %v", err)
	}
	if _, err := pairings.GetTelegramPairing(ctx, "missing"); err == nil {
		t.Fatal("missing pairing unexpectedly found")
	}
}

func TestTeacherRepositoryReportsMissingAndClosedDatabase(t *testing.T) {
	ctx := context.Background()
	var db *sql.DB
	var repo ports.TeacherRepository
	var pairings ports.TelegramPairingRepository
	app := fx.New(fx.NopLogger, fx.Supply(config.DatabaseConfig{SQLitePath: filepath.Join(t.TempDir(), "state.db")}), fx.Provide(database.NewSQLite, storage.NewTeacherRepository, storage.NewTelegramPairingRepository), fx.Populate(&db, &repo, &pairings))
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
	if _, err := repo.Get(ctx, 1); err == nil {
		t.Fatal("closed teacher get unexpectedly succeeded")
	}
	if err := pairings.CreateTelegramPairing(ctx, &teacher.TelegramPairing{RecipientID: 1, TokenHash: "hash", ExpiresAt: time.Now()}); err == nil {
		t.Fatal("closed pairing create unexpectedly succeeded")
	}
	if _, err := pairings.GetTelegramPairing(ctx, "hash"); err == nil {
		t.Fatal("closed pairing get unexpectedly succeeded")
	}
	if err := pairings.ConsumeTelegramPairing(ctx, 1, time.Now()); err == nil {
		t.Fatal("closed pairing consume unexpectedly succeeded")
	}
}
