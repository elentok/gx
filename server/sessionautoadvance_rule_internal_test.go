package server

import (
	"testing"

	"github.com/elentok/gx/harness"
)

func continueMeta(edit func(*sessionMeta)) sessionMeta {
	m := sessionMeta{
		Harness:    string(harness.Claude),
		Status:     SessionStatus{State: SessionIdle},
		Actionable: SessionActionable{Reason: "done", Action: []string{"/clear"}, Derived: true},
	}
	if edit != nil {
		edit(&m)
	}
	return m
}

func TestCanAutoAdvance(t *testing.T) {
	tests := []struct {
		name string
		edit func(*sessionMeta)
		want bool
	}{
		{"derived continue on an idle claude session", nil, true},
		{"switch off", func(m *sessionMeta) { m.AutoAdvanceOff = true }, false},
		{"not derived", func(m *sessionMeta) { m.Actionable.Derived = false }, false},
		{"grilling", func(m *sessionMeta) { m.Actionable.Grilling = true }, false},
		{"no action", func(m *sessionMeta) { m.Actionable.Action = nil }, false},
		{"not promptable", func(m *sessionMeta) { m.Status = SessionStatus{State: SessionWorking} }, false},
		{"codex", func(m *sessionMeta) { m.Harness = string(harness.Codex) }, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := continueMeta(tt.edit).canAutoAdvance(); got != tt.want {
				t.Fatalf("canAutoAdvance = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDecideActionable(t *testing.T) {
	cont := SessionActionable{Reason: "next", Action: []string{"/clear", "/gx-design x"}, Derived: true}
	policy := actionablePolicy{Live: true, MaxAutoSends: 5}
	tests := []struct {
		name    string
		cur     func(*sessionMeta)
		matched bool
		policy  actionablePolicy
		want    actionableOutcome
		reason  string
	}{
		{"gx-raised kept", func(m *sessionMeta) { m.Actionable = SessionActionable{Reason: "gx", GxRaised: true} }, true, policy, actionableNone, "gx"},
		{"derived cleared when unmatched", nil, false, policy, actionableCleared, ""},
		{"agent-raised kept when unmatched", func(m *sessionMeta) { m.Actionable.Derived = false }, false, policy, actionableNone, "done"},
		{"duplicate deduped", func(m *sessionMeta) { m.Actionable = cont }, true, policy, actionableNone, "next"},
		{"not promptable untouched", func(m *sessionMeta) { m.Status = SessionStatus{State: SessionWorking} }, true, policy, actionableNone, "done"},
		{"advance", nil, true, policy, actionableAdvance, "next"},
		{"raised when not live", nil, true, actionablePolicy{MaxAutoSends: 5}, actionableRaised, "next"},
		{"raised with the switch off", func(m *sessionMeta) { m.AutoAdvanceOff = true }, true, policy, actionableRaised, "next"},
		{"advance on codex", func(m *sessionMeta) { m.Harness = string(harness.Codex) }, true, policy, actionableAdvance, "next"},
		{"capped", func(m *sessionMeta) { m.AutoSends = 5 }, true, policy, actionableCapped, "next"},
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
