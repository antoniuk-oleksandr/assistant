package storage

import (
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type failingSQLResult struct{}

func (failingSQLResult) LastInsertId() (int64, error) { return 0, nil }
func (failingSQLResult) RowsAffected() (int64, error) { return 0, errors.New("rows affected failed") }

var _ = Describe("Storage error paths", func() {
	It("reports rows affected errors", func() {
		err := checkedScenarioWrite("set status for", "science-report", failingSQLResult{}, nil)
		Expect(err).To(MatchError(`set status for scenario "science-report": rows affected failed`))
	})
	It("returns query errors when scanning bindings", func() {
		err := errors.New("query failed")
		items, got := scanBindings(nil, err)
		Expect(items).To(BeNil())
		Expect(errors.Is(got, err)).To(BeTrue())
	})
})
