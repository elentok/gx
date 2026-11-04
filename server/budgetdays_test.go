package server

import (
	"testing"
	"time"

	"github.com/elentok/gx/server/limits"
)

func TestBudgetLedger_DaysCopyKeysAroundMidnight(t *testing.T) {
	l := &budgetLedger{Days: map[string]float64{}}
	late := time.Date(2026, 10, 14, 23, 59, 0, 0, time.UTC)
	early := time.Date(2026, 10, 15, 0, 1, 0, 0, time.UTC)
	l.Days[late.Format(budgetDayLayout)] += 1
	l.Days[early.Format(budgetDayLayout)] += 2

	if budgetDayLayout != limits.DayLayout {
		t.Fatalf("budget day layout %q differs from limits %q", budgetDayLayout, limits.DayLayout)
	}
	days := l.daysCopy()
	if days["2026-10-15"] != 2 || days["2026-10-14"] != 1 {
		t.Fatalf("days = %v, want the late and early day keyed apart", days)
	}
}

func TestBudgetLedger_DaysCopyIsIndependent(t *testing.T) {
	l := &budgetLedger{Days: map[string]float64{"2026-10-14": 1}}
	cp := l.daysCopy()
	cp["2026-10-14"] = 99
	if l.Days["2026-10-14"] != 1 {
		t.Fatal("daysCopy aliases the ledger map")
	}
}
