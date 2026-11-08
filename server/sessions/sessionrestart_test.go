package sessions_test

import (
	"path/filepath"
	"testing"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil/agentfake"
)

func TestRestartFresh_KeepsIdAndTierAndTypesThePromptAgain(t *testing.T) {
	t.Parallel()
	h, ctx, evs, id, conv := startFakeSessionWith(t, wire.CreateSessionRequest{Ticket: "proj:epic/01", Prompt: "fix the bug"})
	dir := filepath.Join(h.SessionsDir, id)
	report(t, conv, agentfake.Control{State: "done"})
	waitForScreen(t, waitForHolder(t, dir), "fix the bug")
	before := readMeta(t, h, id)
	cwd, _ := before["dir"].(string)
	writeTranscriptTurns(t, h, cwd, conv, 100, 200)
	if _, ok := before["first_prompt"]; ok {
		t.Fatal("first prompt still saved after it was typed")
	}
	if _, ok := before["vim_relaunch_run"]; ok {
		t.Fatal("vim_relaunch_run set before any restart")
	}

	h.Server.SessionManager().RestartFresh(dir, id, "fix the bug again")

	after := readMeta(t, h, id)
	if after["id"] != id || after["tier"] != before["tier"] {
		t.Errorf("id/tier = %v/%v, want %v/%v", after["id"], after["tier"], id, before["tier"])
	}
	if after["conversation_id"] == conv || after["conversation_id"] == "" {
		t.Errorf("conversation_id = %v, want a new one (was %s)", after["conversation_id"], conv)
	}
	run := after["run"].(float64)
	if prev, _ := before["run"].(float64); run <= prev {
		t.Errorf("run = %v, want above %v", run, prev)
	}
	if after["vim_relaunch_run"] != run {
		t.Errorf("vim_relaunch_run = %v, want %v", after["vim_relaunch_run"], run)
	}
	if first := recordsByID(t, h.ConversationsDir)[conv].Totals; first == nil || first.TotalTokens != 320 {
		t.Errorf("old conversation totals = %+v, want 320 tokens", first)
	}
	// The new watch types the handed-back prompt into the new agent once it is idle.
	report(t, after["conversation_id"].(string), agentfake.Control{State: "done"})
	waitForSession(ctx, t, h, evs, id, func(s wire.SessionInfo) bool {
		return s.Status.State == wire.SessionIdle || s.Status.State == wire.SessionWorking
	})
	waitForScreen(t, waitForHolder(t, dir), "fix the bug again")
}
