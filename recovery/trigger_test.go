package recovery

import (
	"testing"

	"github.com/elentok/gx/events"
)

func TestFailure_Triggers(t *testing.T) {
	tests := []struct {
		name string
		typ  events.Type
		kind events.Kind
		want bool
	}{
		{"needs-repair iteration error", events.NeedsRepair, events.IterationError, true},
		{"needs-repair manual park", events.NeedsRepair, events.ManualPark, true},
		{"needs-repair budget-killed", events.NeedsRepair, events.BudgetKilled, false},
		{"needs-repair self-reported", events.NeedsRepair, events.SelfReported, false},
		{"needs-answer zero-commit", events.NeedsAnswer, events.ZeroCommit, true},
		{"needs-answer blocked-pane", events.NeedsAnswer, events.BlockedPane, true},
		{"needs-answer self-reported", events.NeedsAnswer, events.SelfReported, false},
		{"needs-answer ambiguous-base", events.NeedsAnswer, events.AmbiguousBase, false},
		{"commitless finish", events.Commitless, "", false},
		{"deadlocked", events.Deadlocked, events.AllParked, true},
		{"rate-limit pause is healthy waiting", events.PausedRateLimit, "", false},
	}
	for _, tt := range tests {
		if got := (Failure{Type: tt.typ, Kind: tt.kind}).Triggers(); got != tt.want {
			t.Errorf("%s: Triggers() = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestEntry_Runnable(t *testing.T) {
	remedy := func(Failure, Verbs) error { return nil }
	park := Failure{Type: events.NeedsRepair, Kind: events.IterationError}
	tests := []struct {
		name  string
		entry Entry
		f     Failure
		want  bool
	}{
		{"low rule", Entry{Executor: ExecutorRule, Authority: AuthorityLow, Remedy: remedy}, park, true},
		{"medium rule", Entry{Executor: ExecutorRule, Authority: AuthorityMedium, Remedy: remedy}, park, true},
		{"high rule", Entry{Executor: ExecutorRule, Authority: AuthorityHigh, Remedy: remedy}, park, false},
		{"agent entry", Entry{Executor: ExecutorAgent, Authority: AuthorityLow, Remedy: remedy}, park, false},
		{"rule without remedy", Entry{Executor: ExecutorRule, Authority: AuthorityLow}, park, false},
		{"deadlock is diagnosis only", Entry{Executor: ExecutorRule, Authority: AuthorityLow, Remedy: remedy}, Failure{Type: events.Deadlocked}, false},
	}
	for _, tt := range tests {
		if got := tt.entry.Runnable(tt.f); got != tt.want {
			t.Errorf("%s: Runnable = %v, want %v", tt.name, got, tt.want)
		}
	}
}
