package storage

import (
	"errors"
	"testing"
)

type failingSQLResult struct{}

func (failingSQLResult) LastInsertId() (int64, error) { return 0, nil }
func (failingSQLResult) RowsAffected() (int64, error) { return 0, errors.New("rows affected failed") }

func TestCheckedScenarioWriteReturnsRowsAffectedError(t *testing.T) {
	err := checkedScenarioWrite("set status for", "science-report", failingSQLResult{}, nil)
	if err == nil || err.Error() != "set status for scenario \"science-report\": rows affected failed" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestScanBindingsReturnsQueryError(t *testing.T) {
	err := errors.New("query failed")
	items, got := scanBindings(nil, err)
	if items != nil || !errors.Is(got, err) {
		t.Fatalf("unexpected result: items=%v error=%v", items, got)
	}
}
