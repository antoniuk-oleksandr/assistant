package storage_test

import (
	"context"
	"database/sql"
	"github.com/alex/assistent/internal/adapters/storage"
	"github.com/alex/assistent/internal/domain/ports"
	"github.com/alex/assistent/internal/domain/teacher"
	"github.com/alex/assistent/internal/platform/config"
	"github.com/alex/assistent/internal/platform/database"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/fx"
	"path/filepath"
	"time"
)

var _ = Describe("teacher repository", func() {
	It("round trips teachers and channels", func() {
		ctx := context.Background()
		var db *sql.DB
		var repo ports.TeacherRepository
		var pairings ports.TelegramPairingRepository
		app := fx.New(fx.NopLogger, fx.Supply(config.DatabaseConfig{SQLitePath: filepath.Join(GinkgoT().TempDir(), "state.db")}), fx.Provide(database.NewSQLite, storage.NewTeacherRepository, storage.NewTelegramPairingRepository), fx.Populate(&db, &repo, &pairings))
		Expect(app.Start(ctx)).To(Succeed())
		defer app.Stop(ctx)
		now := time.Now().UTC()
		person := teacher.Recipient{Name: "Teacher", Email: "teacher@example.com", ClassroomID: "class-1", Status: teacher.Active, CreatedAt: now, UpdatedAt: now}
		Expect(repo.Create(ctx, &person)).To(Succeed())
		got, err := repo.Get(ctx, person.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(got.Email).To(Equal(person.Email))
		got.Status = teacher.Disabled
		Expect(repo.Update(ctx, &got)).To(Succeed())
		list, err := repo.List(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(list).To(HaveLen(1))
		channel := teacher.Channel{RecipientID: person.ID, Kind: "email", Destination: person.Email, Status: teacher.Active, CreatedAt: now, UpdatedAt: now}
		Expect(repo.CreateChannel(ctx, &channel)).To(Succeed())
		active, err := repo.ListActiveChannels(ctx, person.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(active).To(HaveLen(1))
		channel.Status = teacher.Disabled
		Expect(repo.UpdateChannel(ctx, &channel)).To(Succeed())
		active, err = repo.ListActiveChannels(ctx, person.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(active).To(BeEmpty())
	})

	It("round trips Telegram pairings", func() {
		ctx := context.Background()
		var db *sql.DB
		var teachers ports.TeacherRepository
		var pairings ports.TelegramPairingRepository
		app := fx.New(fx.NopLogger,
			fx.Supply(config.DatabaseConfig{SQLitePath: filepath.Join(GinkgoT().TempDir(), "state.db")}),
			fx.Provide(database.NewSQLite, storage.NewTeacherRepository, storage.NewTelegramPairingRepository),
			fx.Populate(&db, &teachers, &pairings),
		)
		Expect(app.Start(ctx)).To(Succeed())
		defer app.Stop(ctx)
		now := time.Now().UTC()
		person := teacher.Recipient{Name: "Teacher", Email: "teacher@example.com", Status: teacher.Active, CreatedAt: now, UpdatedAt: now}
		Expect(teachers.Create(ctx, &person)).To(Succeed())
		pairing := teacher.TelegramPairing{RecipientID: person.ID, TokenHash: "hash", ExpiresAt: now.Add(time.Hour)}
		Expect(pairings.CreateTelegramPairing(ctx, &pairing)).To(Succeed())
		got, err := pairings.GetTelegramPairing(ctx, "hash")
		Expect(err).NotTo(HaveOccurred())
		Expect(got.RecipientID).To(Equal(person.ID))
		Expect(got.TokenHash).To(Equal("hash"))
		Expect(pairings.ConsumeTelegramPairing(ctx, pairing.ID, now)).To(Succeed())
		got, err = pairings.GetTelegramPairing(ctx, "hash")
		Expect(err).NotTo(HaveOccurred())
		Expect(got.UsedAt).NotTo(BeNil())
		Expect(pairings.ConsumeTelegramPairing(ctx, pairing.ID, now)).To(Equal(ports.ErrTeacherNotFound))
		_, err = pairings.GetTelegramPairing(ctx, "missing")
		Expect(err).To(HaveOccurred())
	})

	It("reports missing records and closed database errors", func() {
		ctx := context.Background()
		var db *sql.DB
		var repo ports.TeacherRepository
		var pairings ports.TelegramPairingRepository
		app := fx.New(fx.NopLogger, fx.Supply(config.DatabaseConfig{SQLitePath: filepath.Join(GinkgoT().TempDir(), "state.db")}), fx.Provide(database.NewSQLite, storage.NewTeacherRepository, storage.NewTelegramPairingRepository), fx.Populate(&db, &repo, &pairings))
		Expect(app.Start(ctx)).To(Succeed())
		defer app.Stop(ctx)
		_, err := repo.Get(ctx, 999)
		Expect(err).To(HaveOccurred())
		Expect(repo.Update(ctx, &teacher.Recipient{ID: 999, Status: teacher.Disabled, UpdatedAt: time.Now()})).To(HaveOccurred())
		Expect(repo.UpdateChannel(ctx, &teacher.Channel{ID: 999, Status: teacher.Disabled, UpdatedAt: time.Now()})).To(HaveOccurred())
		Expect(db.Close()).To(Succeed())
		_, err = repo.List(ctx)
		Expect(err).To(HaveOccurred())
		_, err = repo.ListActiveChannels(ctx, 1)
		Expect(err).To(HaveOccurred())
		Expect(repo.Create(ctx, &teacher.Recipient{Name: "x", Email: "x@example.com", Status: teacher.Active, CreatedAt: time.Now()})).To(HaveOccurred())
		Expect(repo.CreateChannel(ctx, &teacher.Channel{RecipientID: 1, Kind: "email", Destination: "x", Status: teacher.Active, CreatedAt: time.Now()})).To(HaveOccurred())
		_, err = repo.Get(ctx, 1)
		Expect(err).To(HaveOccurred())
		Expect(pairings.CreateTelegramPairing(ctx, &teacher.TelegramPairing{RecipientID: 1, TokenHash: "hash", ExpiresAt: time.Now()})).To(HaveOccurred())
		_, err = pairings.GetTelegramPairing(ctx, "hash")
		Expect(err).To(HaveOccurred())
		Expect(pairings.ConsumeTelegramPairing(ctx, 1, time.Now())).To(HaveOccurred())
	})
})
