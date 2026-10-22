package ralphloop

import (
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// TestClearablePausedTicket covers the shared predicate's full matrix: pause
// kind (including a missing pause_kind, which must default to
// zero-commit — the conservative choice for tickets paused before that field
// existed) x rendered status x pane liveness x whether the iteration branch
// has new commits.
func TestClearablePausedTicket(t *testing.T) {
	t.Parallel()

	live := func(t *testing.T, d *Deps) { hostLiveAgent(t, *d, "my-epic-iter-01") }
	blocked := func(t *testing.T, d *Deps) {
		live(t, d)
		fakeRunner(*d).SetState("my-epic-iter-01", agentrunner.StateBlocked, "")
	}
	dead := func(t *testing.T, d *Deps) {}
	noCommits := func(t *testing.T, d *Deps) {
		d.CommitsAhead = func(dir, fromExclusive, toRef string) (int, error) { return 0, nil }
	}
	commitsErr := func(t *testing.T, d *Deps) {
		d.CommitsAhead = func(dir, fromExclusive, toRef string) (int, error) { return 0, errors.New("fake CommitsAhead failure") }
	}

	tests := []struct {
		name      string
		status    string
		pauseKind string
		setupDeps func(*testing.T, *Deps)
		want      bool
	}{
		{"blocked-pane live unblocked clearable", "paused", "blocked-pane", live, true},
		{"blocked-pane live still blocked not clearable", "paused", "blocked-pane", blocked, false},
		{"blocked-pane dead pane not clearable", "paused", "blocked-pane", dead, false},
		{"self-reported live unblocked clearable", "paused", "self-reported", live, true},
		{"self-reported dead pane not clearable", "paused", "self-reported", dead, false},
		{"zero-commit live with new commits clearable", "paused", "zero-commit", live, true},
		{"zero-commit live with no new commits not clearable", "paused", "zero-commit", func(t *testing.T, d *Deps) { live(t, d); noCommits(t, d) }, false},
		{"zero-commit dead pane not clearable", "paused", "zero-commit", dead, false},
		{"zero-commit CommitsAhead failure degrades to not clearable", "paused", "zero-commit", func(t *testing.T, d *Deps) { live(t, d); commitsErr(t, d) }, false},
		{"legacy answer status without a reason defaults to zero-commit, live with commits clearable", string(schema.StatusLegacyAnswer), "", live, true},
		{"legacy answer status without a reason defaults to zero-commit, live with no commits not clearable", string(schema.StatusLegacyAnswer), "", func(t *testing.T, d *Deps) { live(t, d); noCommits(t, d) }, false},
		{"repair reason ignores the answer rules, live is enough", "paused", "iteration-error", live, true},
		{"repair reason ignores the answer rules, dead pane not clearable", "paused", "iteration-error", dead, false},
		{"draft ignores pause_reason, live is enough", "draft", "", live, true},
		{"draft ignores pause_reason, dead pane not clearable", "draft", "", dead, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			pauseKindLine := ""
			if tt.pauseKind != "" {
				pauseKindLine = "pause_reason: " + tt.pauseKind + "\n"
			}
			scratchDir := writeEpic(t, "my-epic", map[string]string{
				"01-a.md": "---\nid: \"01\"\nstatus: " + tt.status + "\ntype: implement\n" + pauseKindLine + "---\n# A\n",
			})
			d, _, _ := fakeDeps()
			tt.setupDeps(t, &d)

			epic, err := loadNamedEpic(scratchDir, "my-epic")
			if err != nil {
				t.Fatalf("loadNamedEpic: %v", err)
			}
			ticket := epic.Tickets[0]

			got := clearablePausedTicket(d, "my-epic", "/fake/worktrees", AgentClaude, *epic, ticket)
			if got != tt.want {
				t.Errorf("clearablePausedTicket() = %v, want %v", got, tt.want)
			}
		})
	}
}

// wholeScope resolves epic's whole-epic RunScope, for tests that call
// resumeAnswered directly rather than through Run.
func wholeScope(t *testing.T, epic tickets.Epic) RunScope {
	t.Helper()
	scope, err := ResolveRunScope(epic, nil)
	if err != nil {
		t.Fatalf("ResolveRunScope: %v", err)
	}
	return scope
}

// TestResumeAnswered_LivePaneUnblocked_ReopensAndDemotesStub covers test seam
// 1: a paused (answer) ticket whose pane is still live and has left the blocked
// state is set open and its "## Needs Answer" stub retired into "##
// Comments" — the write resumeAnswered makes on its own, independent of the
// scan loop around it.
func TestResumeAnswered_LivePaneUnblocked_ReopensAndDemotesStub(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "my-epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: needs-answer\ntype: implement\n---\n# A\n\n## Needs Answer\n\nmy-epic-iter-01 is blocked on a prompt gx did not send; answer it in the pane\n",
	})
	path := ticketPath(scratchDir, "my-epic", "01-a.md")
	d, _, _ := fakeDeps()
	hostLiveAgent(t, d, "my-epic-iter-01")

	epic, err := loadNamedEpic(scratchDir, "my-epic")
	if err != nil {
		t.Fatalf("loadNamedEpic: %v", err)
	}
	scope := wholeScope(t, *epic)

	if err := resumeAnswered(d, "my-epic", "/fake/worktrees", AgentClaude, scope, *epic, time.Now()); err != nil {
		t.Fatalf("resumeAnswered: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(raw), "status: open") {
		t.Errorf("ticket not reopened:\n%s", raw)
	}
	if strings.Contains(string(raw), "\n## Needs Answer\n") {
		t.Errorf("Needs Answer stub not retired:\n%s", raw)
	}
	if !strings.Contains(string(raw), "## Comments") || !strings.Contains(string(raw), "retired from `## Needs Answer`") {
		t.Errorf("stub not demoted into Comments:\n%s", raw)
	}
}

// TestResumeAnswered_DeadPane_LeftForHuman covers test seam 3: a paused (answer)
// ticket whose pane is already gone — an announce-and-stop pause, per
// resumeAnswered's doc — is skipped and left paused (answer) for a person to
// answer in the file. The live-pane predicate is the only thing telling this
// apart from the gate-pause case above.
func TestResumeAnswered_DeadPane_LeftForHuman(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "my-epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: needs-answer\ntype: implement\n---\n# A\n\n## Needs Answer\n\nunanswered\n",
	})
	path := ticketPath(scratchDir, "my-epic", "01-a.md")
	d, _, _ := fakeDeps() // TabList returns no tabs by default: the pane is already released

	epic, err := loadNamedEpic(scratchDir, "my-epic")
	if err != nil {
		t.Fatalf("loadNamedEpic: %v", err)
	}
	scope := wholeScope(t, *epic)

	if err := resumeAnswered(d, "my-epic", "/fake/worktrees", AgentClaude, scope, *epic, time.Now()); err != nil {
		t.Fatalf("resumeAnswered: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(raw), "status: needs-answer") {
		t.Errorf("ticket should be left needs-answer for a human, got:\n%s", raw)
	}
	if !strings.Contains(string(raw), "## Needs Answer") {
		t.Errorf("stub should still be present for a human to read:\n%s", raw)
	}
}

// TestResumeAnswered_LiveButStillBlocked_LeftPaused is the live-pane
// predicate's other half: a pane that is still live but still reporting
// blocked hasn't actually been answered yet, so it must be left paused too.
func TestResumeAnswered_LiveButStillBlocked_LeftPaused(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "my-epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: needs-answer\ntype: implement\n---\n# A\n\n## Needs Answer\n\nstill waiting\n",
	})
	path := ticketPath(scratchDir, "my-epic", "01-a.md")
	d, _, _ := fakeDeps()
	hostLiveAgent(t, d, "my-epic-iter-01")
	fakeRunner(d).SetState("my-epic-iter-01", agentrunner.StateBlocked, "")

	epic, err := loadNamedEpic(scratchDir, "my-epic")
	if err != nil {
		t.Fatalf("loadNamedEpic: %v", err)
	}
	scope := wholeScope(t, *epic)

	if err := resumeAnswered(d, "my-epic", "/fake/worktrees", AgentClaude, scope, *epic, time.Now()); err != nil {
		t.Fatalf("resumeAnswered: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(raw), "status: needs-answer") {
		t.Errorf("still-blocked ticket should stay needs-answer, got:\n%s", raw)
	}
}

// TestRun_AnsweredPauseWithSiblingRunning_ResumesWithoutWaitingForSibling
// covers test seam 2 — "the case that matters most": ticket 01 gate-pauses
// (its pane stays live, per waitForFinish's pauseOnBlockedPane) while ticket
// 02 is still genuinely running. Answering ticket 01's pane must reattach it
// before ticket 02 finishes, not after — the ticket-15 wake-source gap a
// plain blocking receive on results left open. ticket02Gate blocks ticket
// 02's own finish until the test explicitly releases it, so a pass here can
// only happen if the scheduler noticed ticket 01's answer on its own, not by
// coincidentally waiting for ticket 02 anyway.
func TestRun_AnsweredPauseWithSiblingRunning_ResumesWithoutWaitingForSibling(t *testing.T) {
	scratchDir := writeEpic(t, "my-epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: implement\n---\n# A\n",
		"02-b.md": "---\nid: \"02\"\nstatus: open\ntype: implement\n---\n# B\n",
	})
	path01 := ticketPath(scratchDir, "my-epic", "01-a.md")
	path02 := ticketPath(scratchDir, "my-epic", "02-b.md")
	d, _, removed := fakeDeps()
	// The "status: done" write merges before the worktree removal, so the
	// removal count is only meaningful once a removal has been signalled.
	removedSignal := make(chan struct{}, 2)
	removeWorktree := d.RemoveWorktree
	d.RemoveWorktree = func(repoDir, path string, force bool) error {
		err := removeWorktree(repoDir, path, force)
		removedSignal <- struct{}{}
		return err
	}

	var mu sync.Mutex
	unblocked01 := false
	ticket02Gate := make(chan struct{})

	onRunnerWait(d, func(s agentrunner.Session) {
		if !strings.Contains(s.Label, "iter-01") {
			<-ticket02Gate
			return
		}
		mu.Lock()
		state := agentrunner.StateBlocked
		if unblocked01 {
			state = agentrunner.StateIdle
		}
		mu.Unlock()
		fakeRunner(d).SetState(s.Label, state, "")
	})

	sink := &recordingSink{}
	done := make(chan error, 1)
	go func() {
		done <- Run(RunOptions{
			EpicName: "my-epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo",
			MaxParallel: 2,
		}, d, sink)
	}()

	// Wait for ticket 01 to gate-pause (its pane stays blocked and live).
	waitForFileContains(t, path01, "status: paused")

	// Answer the pane, but do NOT release ticket 02 yet.
	mu.Lock()
	unblocked01 = true
	mu.Unlock()
	fakeRunner(d).SetState("my-epic-iter-01", agentrunner.StateIdle, "")

	// If the fix didn't work, this blocks until the test times out, since the
	// old code wouldn't notice ticket 01's answer until ticket 02's result
	// arrived — which the still-held ticket02Gate prevents.
	waitForFileContains(t, path01, "status: done")

	select {
	case <-removedSignal:
	case <-time.After(5 * time.Second):
		t.Fatal("ticket 01's worktree was never removed after it merged")
	}
	if len(*removed) != 1 {
		t.Errorf("removed worktree branches after ticket 01 reattached+merged = %v, want exactly one (ticket 01), ticket 02 still running", *removed)
	}

	close(ticket02Gate)
	if err := <-done; err != nil {
		t.Fatalf("Run() error = %v, want nil", err)
	}

	raw02, err := os.ReadFile(path02)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(raw02), "status: done") {
		t.Errorf("ticket 02 not marked done:\n%s", raw02)
	}
}

// TestUnmuteTicket_Paused_ClearsMutesDemotesReopens covers a mute that
// actually paused its ticket (status: paused (repair), written by muteSource's
// pauseTicket callback): UnmuteTicket clears Mutes, demotes the "## Needs
// Repair" section into "## Comments", reopens the ticket, and appends its
// own unmute audit note.
func TestUnmuteTicket_Paused_ClearsMutesDemotesReopens(t *testing.T) {
	t.Parallel()
	path := writeTicket(t, "---\nid: \"01\"\nstatus: needs-repair\ntype: implement\nmutes:\n    - event_type: notification-storm\n      tripped_at: 2026-01-01T00:00:00Z\n---\n# A\n\n## Needs Repair\n\nnotification storm tripped a mute\n")

	if err := UnmuteTicket(path, time.Now()); err != nil {
		t.Fatalf("UnmuteTicket: %v", err)
	}

	ticket := mustParse(t, path)
	if ticket.Status != schema.StatusOpen {
		t.Errorf("Status = %q, want %q", ticket.Status, schema.StatusOpen)
	}
	if len(ticket.Mutes) != 0 {
		t.Errorf("Mutes = %v, want empty", ticket.Mutes)
	}

	raw := mustRead(t, path)
	if strings.Contains(raw, "\n## Needs Repair\n") {
		t.Errorf("Needs Repair section not demoted:\n%s", raw)
	}
	if !strings.Contains(raw, "## Comments") || !strings.Contains(raw, "retired from `## Needs Repair`") {
		t.Errorf("Needs Repair not retired into Comments:\n%s", raw)
	}
	if !strings.Contains(raw, "— unmuted") {
		t.Errorf("unmute audit note missing:\n%s", raw)
	}
}

// TestUnmuteTicket_Resumed_ClearsMutesLeavesStatus covers a mute that
// tripped with no pauseTicket callback available (see muteSource) — the
// ticket's status was never touched, so there's no pause to undo: clearing
// Mutes (plus the audit note) is the whole action.
func TestUnmuteTicket_Resumed_ClearsMutesLeavesStatus(t *testing.T) {
	t.Parallel()
	path := writeTicket(t, "---\nid: \"01\"\nstatus: open\ntype: implement\nmutes:\n    - event_type: notification-storm\n      tripped_at: 2026-01-01T00:00:00Z\n---\n# A\n\nBody text.\n")

	if err := UnmuteTicket(path, time.Now()); err != nil {
		t.Fatalf("UnmuteTicket: %v", err)
	}

	ticket := mustParse(t, path)
	if ticket.Status != schema.StatusOpen {
		t.Errorf("Status = %q, want unchanged %q", ticket.Status, schema.StatusOpen)
	}
	if len(ticket.Mutes) != 0 {
		t.Errorf("Mutes = %v, want empty", ticket.Mutes)
	}

	raw := mustRead(t, path)
	if !strings.Contains(raw, "— unmuted") {
		t.Errorf("unmute audit note missing:\n%s", raw)
	}
}

// waitForFileContains polls path until its content contains want, failing
// the test if that never happens within a few seconds.
func waitForFileContains(t *testing.T, path, want string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		raw, err := os.ReadFile(path)
		if err == nil && strings.Contains(string(raw), want) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s to contain %q", path, want)
}
