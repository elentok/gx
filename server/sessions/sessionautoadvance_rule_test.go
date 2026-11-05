package sessions_test

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
