package ralphloop

import (
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/herdr"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// TestClearableParkedTicket covers the shared predicate's full matrix: park
// kind (including a missing park_kind, which must default to
// zero-commit — the conservative choice for tickets parked before that field
// existed) x rendered status x pane liveness x whether the iteration branch
// has new commits.
func TestClearableParkedTicket(t *testing.T) {
	t.Parallel()

	live := func(d *Deps) {
		d.TabList = func(workspaceID string) ([]herdr.Tab, error) {
			return []herdr.Tab{{TabID: "tab-my-epic-iter-01", Label: "my-epic-iter-01", WorkspaceID: workspaceID}}, nil
		}
		d.AgentGet = func(target string) (herdr.Agent, error) {
			return herdr.Agent{PaneID: "pane-" + target, WorkspaceID: "ws1", TabID: "tab-" + target, AgentStatus: "idle"}, nil
		}
	}
	blocked := func(d *Deps) {
		live(d)
		d.AgentGet = func(target string) (herdr.Agent, error) {
			return herdr.Agent{PaneID: "pane-" + target, WorkspaceID: "ws1", TabID: "tab-" + target, AgentStatus: "blocked"}, nil
		}
	}
	dead := func(d *Deps) {
		d.TabList = func(workspaceID string) ([]herdr.Tab, error) { return nil, nil }
	}
	noCommits := func(d *Deps) {
		d.CommitsAhead = func(dir, fromExclusive, toRef string) (int, error) { return 0, nil }
	}
	commitsErr := func(d *Deps) {
		d.CommitsAhead = func(dir, fromExclusive, toRef string) (int, error) { return 0, errors.New("fake CommitsAhead failure") }
	}

	tests := []struct {
		name      string
		status    string
		parkKind  string
		setupDeps func(*Deps)
		want      bool
	}{
		{"blocked-pane live unblocked clearable", "needs-answer", "blocked-pane", live, true},
		{"blocked-pane live still blocked not clearable", "needs-answer", "blocked-pane", blocked, false},
		{"blocked-pane dead pane not clearable", "needs-answer", "blocked-pane", dead, false},
		{"self-reported live unblocked clearable", "needs-answer", "self-reported", live, true},
		{"self-reported dead pane not clearable", "needs-answer", "self-reported", dead, false},
		{"zero-commit live with new commits clearable", "needs-answer", "zero-commit", live, true},
		{"zero-commit live with no new commits not clearable", "needs-answer", "zero-commit", func(d *Deps) { live(d); noCommits(d) }, false},
		{"zero-commit dead pane not clearable", "needs-answer", "zero-commit", dead, false},
		{"zero-commit CommitsAhead failure degrades to not clearable", "needs-answer", "zero-commit", func(d *Deps) { live(d); commitsErr(d) }, false},
		{"missing park_kind defaults to zero-commit, live with commits clearable", "needs-answer", "", live, true},
		{"missing park_kind defaults to zero-commit, live with no commits not clearable", "needs-answer", "", func(d *Deps) { live(d); noCommits(d) }, false},
		{"needs-repair ignores park_kind, live is enough", "needs-repair", "", live, true},
		{"needs-repair ignores park_kind, dead pane not clearable", "needs-repair", "", dead, false},
		{"draft ignores park_kind, live is enough", "draft", "", live, true},
		{"draft ignores park_kind, dead pane not clearable", "draft", "", dead, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			parkKindLine := ""
			if tt.parkKind != "" {
				parkKindLine = "park_kind: " + tt.parkKind + "\n"
			}
			scratchDir := writeEpic(t, "my-epic", map[string]string{
				"01-a.md": "---\nid: \"01\"\nstatus: " + tt.status + "\ntype: task\n" + parkKindLine + "---\n# A\n",
			})
			d, _, _ := fakeDeps()
			tt.setupDeps(&d)

			epic, err := loadNamedEpic(scratchDir, "my-epic")
			if err != nil {
				t.Fatalf("loadNamedEpic: %v", err)
			}
			ticket := epic.Tickets[0]

			got := clearableParkedTicket(d, "ws1", "my-epic", "/fake/worktrees", AgentClaude, *epic, ticket)
			if got != tt.want {
				t.Errorf("clearableParkedTicket() = %v, want %v", got, tt.want)
			}
		})
	}
}

// wholeScope resolves epic's whole-epic RunScope, for tests that call
// unparkAnswered directly rather than through Run.
func wholeScope(t *testing.T, epic tickets.Epic) RunScope {
	t.Helper()
	scope, err := ResolveRunScope(epic, nil)
	if err != nil {
		t.Fatalf("ResolveRunScope: %v", err)
	}
	return scope
}

// TestUnparkAnswered_LivePaneUnblocked_ReopensAndDemotesStub covers test seam
// 1: a needs-answer ticket whose pane is still live and has left the blocked
// state is set open and its "## Needs Answer" stub retired into "##
// Comments" — the write unparkAnswered makes on its own, independent of the
// scan loop around it.
func TestUnparkAnswered_LivePaneUnblocked_ReopensAndDemotesStub(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "my-epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: needs-answer\ntype: task\n---\n# A\n\n## Needs Answer\n\nmy-epic-iter-01 is blocked on a prompt gx did not send; answer it in the pane\n",
	})
	path := ticketPath(scratchDir, "my-epic", "01-a.md")
	d, _, _ := fakeDeps()
	d.TabList = func(workspaceID string) ([]herdr.Tab, error) {
		return []herdr.Tab{{TabID: "tab-my-epic-iter-01", Label: "my-epic-iter-01", WorkspaceID: workspaceID}}, nil
	}
	d.AgentGet = func(target string) (herdr.Agent, error) {
		return herdr.Agent{PaneID: "pane-" + target, WorkspaceID: "ws1", TabID: "tab-" + target, AgentStatus: "idle"}, nil
	}

	epic, err := loadNamedEpic(scratchDir, "my-epic")
	if err != nil {
		t.Fatalf("loadNamedEpic: %v", err)
	}
	scope := wholeScope(t, *epic)

	if err := unparkAnswered(d, "ws1", "my-epic", "/fake/worktrees", AgentClaude, scope, *epic, time.Now()); err != nil {
		t.Fatalf("unparkAnswered: %v", err)
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

// TestUnparkAnswered_DeadPane_LeftForHuman covers test seam 3: a needs-answer
// ticket whose pane is already gone — an announce-and-stop park, per
// unparkAnswered's doc — is skipped and left needs-answer for a person to
// answer in the file. The live-pane predicate is the only thing telling this
// apart from the gate-park case above.
func TestUnparkAnswered_DeadPane_LeftForHuman(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "my-epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: needs-answer\ntype: task\n---\n# A\n\n## Needs Answer\n\nunanswered\n",
	})
	path := ticketPath(scratchDir, "my-epic", "01-a.md")
	d, _, _ := fakeDeps() // TabList returns no tabs by default: the pane is already released

	epic, err := loadNamedEpic(scratchDir, "my-epic")
	if err != nil {
		t.Fatalf("loadNamedEpic: %v", err)
	}
	scope := wholeScope(t, *epic)

	if err := unparkAnswered(d, "ws1", "my-epic", "/fake/worktrees", AgentClaude, scope, *epic, time.Now()); err != nil {
		t.Fatalf("unparkAnswered: %v", err)
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

// TestUnparkAnswered_LiveButStillBlocked_LeftParked is the live-pane
// predicate's other half: a pane that is still live but still reporting
// blocked hasn't actually been answered yet, so it must be left parked too.
func TestUnparkAnswered_LiveButStillBlocked_LeftParked(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "my-epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: needs-answer\ntype: task\n---\n# A\n\n## Needs Answer\n\nstill waiting\n",
	})
	path := ticketPath(scratchDir, "my-epic", "01-a.md")
	d, _, _ := fakeDeps()
	d.TabList = func(workspaceID string) ([]herdr.Tab, error) {
		return []herdr.Tab{{TabID: "tab-my-epic-iter-01", Label: "my-epic-iter-01", WorkspaceID: workspaceID}}, nil
	}
	d.AgentGet = func(target string) (herdr.Agent, error) {
		return herdr.Agent{PaneID: "pane-" + target, WorkspaceID: "ws1", TabID: "tab-" + target, AgentStatus: "blocked"}, nil
	}

	epic, err := loadNamedEpic(scratchDir, "my-epic")
	if err != nil {
		t.Fatalf("loadNamedEpic: %v", err)
	}
	scope := wholeScope(t, *epic)

	if err := unparkAnswered(d, "ws1", "my-epic", "/fake/worktrees", AgentClaude, scope, *epic, time.Now()); err != nil {
		t.Fatalf("unparkAnswered: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(raw), "status: needs-answer") {
		t.Errorf("still-blocked ticket should stay needs-answer, got:\n%s", raw)
	}
}

// TestRun_AnsweredParkWithSiblingRunning_UnparksWithoutWaitingForSibling
// covers test seam 2 — "the case that matters most": ticket 01 gate-parks
// (its pane stays live, per waitForFinish's parkOnBlockedPane) while ticket
// 02 is still genuinely running. Answering ticket 01's pane must reattach it
// before ticket 02 finishes, not after — the ticket-15 wake-source gap a
// plain blocking receive on results left open. ticket02Gate blocks ticket
// 02's own finish until the test explicitly releases it, so a pass here can
// only happen if the scheduler noticed ticket 01's answer on its own, not by
// coincidentally waiting for ticket 02 anyway.
func TestRun_AnsweredParkWithSiblingRunning_UnparksWithoutWaitingForSibling(t *testing.T) {
	// not parallel-safe: asserts len(*removed) immediately after observing
	// ticket 01's "status: done" write, with no synchronization between that
	// write and the worktree-removal step it's racing — under the CPU
	// contention of running alongside many other parallel tests, the removal
	// can lag behind and flake the assertion.
	scratchDir := writeEpic(t, "my-epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# A\n",
		"02-b.md": "---\nid: \"02\"\nstatus: open\ntype: task\n---\n# B\n",
	})
	path01 := ticketPath(scratchDir, "my-epic", "01-a.md")
	path02 := ticketPath(scratchDir, "my-epic", "02-b.md")
	d, _, removed := fakeDeps()

	var mu sync.Mutex
	unblocked01 := false
	ticket02Gate := make(chan struct{})

	d.TabList = func(workspaceID string) ([]herdr.Tab, error) {
		return []herdr.Tab{
			{TabID: "tab-my-epic-iter-01", Label: "my-epic-iter-01", WorkspaceID: workspaceID},
			{TabID: "tab-my-epic-iter-02", Label: "my-epic-iter-02", WorkspaceID: workspaceID},
		}, nil
	}
	d.AgentGet = func(target string) (herdr.Agent, error) {
		status := "idle"
		if strings.Contains(target, "iter-01") {
			mu.Lock()
			if !unblocked01 {
				status = "blocked"
			}
			mu.Unlock()
		}
		return herdr.Agent{PaneID: "pane-" + target, WorkspaceID: "ws1", TabID: "tab-" + target, AgentStatus: status}, nil
	}
	d.AgentWait = func(opts herdr.AgentWaitOptions) (herdr.Agent, error) {
		if strings.Contains(opts.Target, "iter-01") {
			mu.Lock()
			done := unblocked01
			mu.Unlock()
			if !done {
				return herdr.Agent{PaneID: opts.Target, AgentStatus: "blocked"}, nil
			}
			return herdr.Agent{PaneID: opts.Target, AgentStatus: "idle"}, nil
		}
		// ticket 02 stays "running" until the test releases it — proving the
		// scheduler didn't just happen to wait it out.
		<-ticket02Gate
		return herdr.Agent{PaneID: opts.Target, AgentStatus: "idle"}, nil
	}

	sink := &recordingSink{}
	done := make(chan error, 1)
	go func() {
		done <- Run(RunOptions{
			EpicName: "my-epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo",
			MaxParallel: 2,
		}, d, sink)
	}()

	// Wait for ticket 01 to gate-park (its pane stays blocked and live).
	waitForFileContains(t, path01, "status: needs-answer")

	// Answer the pane, but do NOT release ticket 02 yet.
	mu.Lock()
	unblocked01 = true
	mu.Unlock()

	// If the fix didn't work, this blocks until the test times out, since the
	// old code wouldn't notice ticket 01's answer until ticket 02's result
	// arrived — which the still-held ticket02Gate prevents.
	waitForFileContains(t, path01, "status: done")

	if len(*removed) != 1 {
		t.Errorf("removed worktree branches after ticket 01 reattached+landed = %v, want exactly one (ticket 01), ticket 02 still running", *removed)
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

// TestUnmuteTicket_Parked_ClearsMutesDemotesReopens covers a mute that
// actually parked its ticket (status: needs-repair, written by muteSource's
// parkTicket callback): UnmuteTicket clears Mutes, demotes the "## Needs
// Repair" section into "## Comments", reopens the ticket, and appends its
// own unmute audit note.
func TestUnmuteTicket_Parked_ClearsMutesDemotesReopens(t *testing.T) {
	t.Parallel()
	path := writeTicket(t, "---\nid: \"01\"\nstatus: needs-repair\ntype: task\nmutes:\n    - event_type: notification-storm\n      tripped_at: 2026-01-01T00:00:00Z\n---\n# A\n\n## Needs Repair\n\nnotification storm tripped a mute\n")

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
	if !strings.Contains(raw, "unmuted via Suggested Actions") {
		t.Errorf("unmute audit note missing:\n%s", raw)
	}
}

// TestUnmuteTicket_Unparked_ClearsMutesLeavesStatus covers a mute that
// tripped with no parkTicket callback available (see muteSource) — the
// ticket's status was never touched, so there's no park to undo: clearing
// Mutes (plus the audit note) is the whole action.
func TestUnmuteTicket_Unparked_ClearsMutesLeavesStatus(t *testing.T) {
	t.Parallel()
	path := writeTicket(t, "---\nid: \"01\"\nstatus: open\ntype: task\nmutes:\n    - event_type: notification-storm\n      tripped_at: 2026-01-01T00:00:00Z\n---\n# A\n\nBody text.\n")

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
	if !strings.Contains(raw, "unmuted via Suggested Actions") {
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
