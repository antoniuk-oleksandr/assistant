package application_test

import (
	"context"
	"errors"
	"github.com/alex/assistent/internal/domain/history"

	"github.com/alex/assistent/internal/application"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type historyRepo struct {
	entries []history.Entry
	err     error
}

func (r historyRepo) ListHistory(context.Context) ([]history.Entry, error) { return r.entries, r.err }

var _ = Describe("History service", func() {
	It("lists history and propagates repository errors", func() {
		expected := []history.Entry{{RunID: 1, RunStatus: "failed", Partial: true, ErrorMessage: "missing"}}
		got, err := application.NewHistoryService(historyRepo{entries: expected}).List(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(got).To(Equal(expected))
		_, err = application.NewHistoryService(historyRepo{err: errors.New("history unavailable")}).List(context.Background())
		Expect(err).To(MatchError("history unavailable"))
	})
})
