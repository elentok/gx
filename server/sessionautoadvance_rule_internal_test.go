package server

import (
	"testing"

	"github.com/elentok/gx/server/sessions"
	"github.com/elentok/gx/server/wire"

	"github.com/elentok/gx/harness"
)

func continueMeta(edit func(*sessions.Meta)) sessions.Meta {
	m := sessions.Meta{
		Harness:    string(harness.Claude),
		Status:     wire.SessionStatus{State: wire.SessionIdle},
		Actionable: wire.SessionActionable{Reason: "done", Action: []string{"/clear"}, Derived: true},
	}
	if edit != nil {
		edit(&m)
	}
	return m
}

func TestCanAutoAdvance(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		edit func(*sessions.Meta)
		want bool
	}{
		{"derived continue on an idle claude session", nil, true},
		{"switch off", func(m *sessions.Meta) { m.AutoAdvanceOff = true }, false},
		{"not derived", func(m *sessions.Meta) { m.Actionable.Derived = false }, false},
		{"grilling", func(m *sessions.Meta) { m.Actionable.Grilling = true }, false},
		{"no action", func(m *sessions.Meta) { m.Actionable.Action = nil }, false},
		{"not promptable", func(m *sessions.Meta) { m.Status = wire.SessionStatus{State: wire.SessionWorking} }, false},
		{"codex", func(m *sessions.Meta) { m.Harness = string(harness.Codex) }, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := continueMeta(tt.edit).CanAutoAdvance(); got != tt.want {
				t.Fatalf("canAutoAdvance = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDecideActionable(t *testing.T) {
	t.Parallel()
	cont := wire.SessionActionable{Reason: "next", Action: []string{"/clear", "/gx-design x"}, Derived: true}
	policy := actionablePolicy{Live: true, MaxAutoSends: 5}
	tests := []struct {
		name    string
		cur     func(*sessions.Meta)
		matched bool
		policy  actionablePolicy
		want    actionableOutcome
		reason  string
	}{
		{"gx-raised kept", func(m *sessions.Meta) { m.Actionable = wire.SessionActionable{Reason: "gx", GxRaised: true} }, true, policy, actionableNone, "gx"},
		{"derived cleared when unmatched", nil, false, policy, actionableCleared, ""},
		{"agent-raised kept when unmatched", func(m *sessions.Meta) { m.Actionable.Derived = false }, false, policy, actionableNone, "done"},
		{"duplicate deduped", func(m *sessions.Meta) { m.Actionable = cont }, true, policy, actionableNone, "next"},
		{"not promptable untouched", func(m *sessions.Meta) { m.Status = wire.SessionStatus{State: wire.SessionWorking} }, true, policy, actionableNone, "done"},
		{"advance", nil, true, policy, actionableAdvance, "next"},
		{"raised when not live", nil, true, actionablePolicy{MaxAutoSends: 5}, actionableRaised, "next"},
		{"raised with the switch off", func(m *sessions.Meta) { m.AutoAdvanceOff = true }, true, policy, actionableRaised, "next"},
		{"advance on codex", func(m *sessions.Meta) { m.Harness = string(harness.Codex) }, true, policy, actionableAdvance, "next"},
		{"capped", func(m *sessions.Meta) { m.AutoSends = 5 }, true, policy, actionableCapped, "next"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			next, got := decideActionable(continueMeta(tt.cur), cont, tt.matched, tt.policy)
			if got != tt.want {
				t.Fatalf("outcome = %v, want %v", got, tt.want)
			}
			if next.Reason != tt.reason {
				t.Fatalf("next reason = %q, want %q", next.Reason, tt.reason)
			}
		})
	}
}
