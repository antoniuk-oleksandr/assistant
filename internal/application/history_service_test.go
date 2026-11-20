package application_test

import (
	"context"
	"errors"
	"github.com/alex/assistent/internal/application"
	"github.com/alex/assistent/internal/domain/history"
	"testing"
)

type historyRepo struct {
	entries []history.Entry
	err     error
}

func (r historyRepo) ListHistory(context.Context) ([]history.Entry, error) { return r.entries, r.err }
func TestHistoryServiceListsActionableHistory(t *testing.T) {
	expected := []history.Entry{{RunID: 1, RunStatus: "failed", Partial: true, ErrorMessage: "missing"}}
	got, err := application.NewHistoryService(historyRepo{entries: expected}).List(context.Background())
	if err != nil || len(got) != 1 || got[0].ErrorMessage != "missing" {
		t.Fatalf("got=%#v err=%v", got, err)
	}
	if _, err := application.NewHistoryService(historyRepo{err: errors.New("history unavailable")}).List(context.Background()); err == nil {
		t.Fatal("error missing")
	}
}
