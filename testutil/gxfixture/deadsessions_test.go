package main

import (
	"encoding/json"
	"testing"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil/gxfixture/spec"
)

func (r *running) sessions(t *testing.T) []wire.SessionInfo {
	t.Helper()
	var list []wire.SessionInfo
	if err := json.Unmarshal([]byte(r.Gx(t, "agents", "list", "--json")), &list); err != nil {
		t.Fatal(err)
	}
	return list
}

func (r *running) wantAllCrashed(t *testing.T, n int) []wire.SessionInfo {
	t.Helper()
	list := r.sessions(t)
	if len(list) != n {
		t.Fatalf("%d sessions, want %d", len(list), n)
	}
	for _, s := range list {
		if s.Status.State != wire.SessionExited || s.Exit != wire.SessionExitCrashed {
			t.Errorf("session %s = %s/%s, want exited/crashed", s.ID, s.Status.State, s.Exit)
		}
	}
	if holders, agents := r.ProcessCounts(t); holders != 0 || agents != 0 {
		t.Errorf("holders = %d, agents = %d, want none", holders, agents)
	}
	return list
}

func TestDeadMany(t *testing.T) {
	r := start(t, "dead-many")
	r.wantAllCrashed(t, spec.DeadSessionCount)
}

func TestDeadQuestion(t *testing.T) {
	r := start(t, "dead-question")
	s := r.wantAllCrashed(t, 1)[0]
	if !s.Unread.Unread {
		t.Error("the dead session lost its Unread")
	}
}

func TestDeadDesign(t *testing.T) {
	r := start(t, "dead-design")
	if s := r.wantAllCrashed(t, 1)[0]; !s.Design {
		t.Errorf("session %s is not a design session", s.ID)
	}
}
