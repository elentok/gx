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
		{"repair iteration error", events.Paused, events.IterationError, true},
		{"repair manual pause", events.Paused, events.Manual, false},
		{"repair budget-killed", events.Paused, events.BudgetKilled, false},
		{"answer zero-commit", events.Paused, events.ZeroCommit, true},
		{"answer blocked-pane", events.Paused, events.BlockedPane, true},
		{"answer self-reported", events.Paused, events.SelfReported, false},
		{"answer ambiguous-base", events.Paused, events.AmbiguousBase, false},
		{"commitless finish", events.Commitless, "", false},
		{"deadlocked", events.Deadlocked, events.AllPaused, true},
		{"rate-limit pause is healthy waiting", events.PausedRateLimit, "", false},
		{"notification-failed", events.NotificationFailed, "", false},
		{"background gate held", events.BackgroundTaskGateHeld, events.BackgroundTaskGate, true},
		{"ticket-graph defect", events.TicketGraphDefect, events.ParentDefect, true},
	}
	for _, tt := range tests {
		if got := (Failure{Type: tt.typ, Kind: tt.kind}).Triggers(); got != tt.want {
			t.Errorf("%s: Triggers() = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestFailure_NeedsMatch(t *testing.T) {
	if !(Failure{Type: events.Paused, Kind: events.BlockedPane}).NeedsMatch() {
		t.Error("blocked-pane: NeedsMatch() = false, want true")
	}
	if (Failure{Type: events.Paused, Kind: events.ZeroCommit}).NeedsMatch() {
		t.Error("zero-commit: NeedsMatch() = true, want false")
	}
}

func TestEntry_Runnable(t *testing.T) {
	remedy := func(Failure, Verbs) error { return nil }
	pause := Failure{Type: events.Paused, Kind: events.IterationError}
	tests := []struct {
		name  string
		entry Entry
		f     Failure
		want  bool
	}{
		{"low rule", Entry{Executor: ExecutorRule, Authority: AuthorityLow, Remedy: remedy}, pause, true},
		{"medium rule", Entry{Executor: ExecutorRule, Authority: AuthorityMedium, Remedy: remedy}, pause, true},
		{"high rule", Entry{Executor: ExecutorRule, Authority: AuthorityHigh, Remedy: remedy}, pause, false},
		{"agent entry", Entry{Executor: ExecutorAgent, Authority: AuthorityLow, Remedy: remedy}, pause, false},
		{"rule without remedy", Entry{Executor: ExecutorRule, Authority: AuthorityLow}, pause, false},
		{"deadlock is diagnosis only", Entry{Executor: ExecutorRule, Authority: AuthorityLow, Remedy: remedy}, Failure{Type: events.Deadlocked}, false},
	}
	for _, tt := range tests {
		if got := tt.entry.Runnable(tt.f); got != tt.want {
			t.Errorf("%s: Runnable = %v, want %v", tt.name, got, tt.want)
		}
	}
}
