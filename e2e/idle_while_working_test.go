package e2e

import (
	"testing"
	"time"

	"github.com/elentok/gx/herdr"
	"github.com/elentok/gx/testutil/herdrctl"
)

// TestIdleWhileWorking_AgentStatusNeverReportsWorking pins a real, currently
// live herdr bug (the map's "Idle-while-working (agent-status scrape)" fog
// item, confirmed here): an agent's exposed `agent_status` never transitions
// to "working" for the whole duration it is genuinely busy, staying stuck at
// "idle" from the moment it's started — even though herdr's own
// `terminal_title` field correctly shows the busy spinner glyph the entire
// time (confirmed hands-on against real herdr 0.8.0 with a fake agent
// visibly busy for 20s: agent_status read "idle" at every one-second sample
// until the title reverted). `agent wait --until working` genuinely times
// out rather than ever firing.
//
// This is the inverse failure mode from the compaction false-idle scenario
// (e2e/compaction_false_idle_test.go: turn reported settled too early).
// Here the turn is reported settled the *whole time* it's running — a
// consumer polling agent_status to show "is the agent currently working"
// gets a false "idle" for the whole busy window.
//
// This test intentionally asserts the CURRENT (buggy) behavior, not the
// desired one: a test asserting "status reads working while busy" would be
// permanently red against real herdr today. When herdr fixes this, this
// test starts failing — that failure is the signal to flip it to assert the
// correct behavior instead of silently drifting.
func TestIdleWhileWorking_AgentStatusNeverReportsWorking(t *testing.T) {
	herdrctl.RequireHerdr(t)

	const workingDuration = 6 * time.Second

	fakeDir := agentfakeBinary(t)
	repoDir := t.TempDir()

	ws := herdrctl.NewWorkspace(t, repoDir)
	ws.PrependPath(fakeDir)

	started := ws.AgentStart(herdr.AgentStartOptions{
		Name: "idle-while-working",
		Kind: "claude",
		AgentArgs: []string{
			"--mode=slow-working",
			"--duration=" + workingDuration.String(),
		},
	})
	// Bug: AgentStart's own snapshot already reports "idle" even though the
	// fake has just written its working title.
	if started.AgentStatus != "idle" {
		t.Fatalf("agent status right after start = %q, want idle (herdr's known idle-while-working bug appears to be fixed — flip this test to assert the correct \"working\" status instead)", started.AgentStatus)
	}

	// Sample mid-window: still busy, status should (bug notwithstanding)
	// still read "idle", not "working".
	time.Sleep(workingDuration / 2)
	mid := ws.AgentGet("")
	if mid.AgentStatus != "idle" {
		t.Fatalf("agent status mid-window = %q, want idle (herdr's known idle-while-working bug appears to be fixed — flip this test to assert the correct \"working\" status instead)", mid.AgentStatus)
	}

	// `agent wait --until working` should (bug notwithstanding) never fire
	// during the busy window — call the non-fatal herdr.AgentWait directly
	// since a timeout error is the expected outcome here.
	if _, err := herdr.AgentWait(herdr.AgentWaitOptions{
		Target:    ws.RootPaneID,
		Until:     []string{"working"},
		TimeoutMs: int(workingDuration.Milliseconds()) + 3000,
	}); err == nil {
		t.Fatalf(`agent wait --until working returned successfully (herdr's known idle-while-working bug appears to be fixed — flip this test to assert the correct "working" status instead)`)
	}

	// Regardless of the bug above, status must still correctly settle to
	// idle/done once the fake's busy window actually elapses.
	final := ws.AgentWait(herdr.AgentWaitOptions{
		Until:     []string{"idle", "done"},
		TimeoutMs: 10000,
	})
	if final.AgentStatus != "idle" && final.AgentStatus != "done" {
		t.Fatalf("agent status after working duration elapsed = %q, want idle or done", final.AgentStatus)
	}
}
