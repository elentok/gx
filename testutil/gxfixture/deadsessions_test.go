package main

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil"
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
	var list []wire.SessionInfo
	var holders, agents int
	// The server marks the dead sessions and reaps their processes after it
	// starts, so a one-shot read can see the state half way.
	settled := testutil.Poll(15*time.Second, 100*time.Millisecond, func() bool {
		list = r.sessions(t)
		if len(list) != n {
			return false
		}
		for _, s := range list {
			if s.Status.State != wire.SessionExited || s.Exit != wire.SessionExitCrashed {
				return false
			}
		}
		holders, agents = r.ProcessCounts(t)
		return holders == 0 && agents == 0
	})
	if !settled {
		t.Fatalf("sessions never settled to %d exited/crashed with no processes: holders = %d, agents = %d, list = %+v", n, holders, agents, list)
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
