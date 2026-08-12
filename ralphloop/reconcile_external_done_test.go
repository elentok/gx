package ralphloop

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/herdr"
	"github.com/elentok/gx/tickets"
)

// This file covers the "externally landed" ticket class of bug reported
// live: running `gx ralph-loop` (the "i" implement action) against an epic
// whose done tickets were completed and landed on the feature branch outside
// ralph-loop's own iteration/cherry-pick flow (e.g. cherry-picked/merged by
// hand, or by an agent session that never went through landCherryPick) — so
// they have no run-log event, no surviving iteration branch, and no
// Ralph-Loop-Ticket trailer. classifyDoneTicket correctly has no way to
// distinguish that from a ticket that was simply never implemented despite
// being marked done (see
// TestClassifyDoneTicket_RealRepo_TrailerScopedToEpic_NoCrossEpicFalsePositive,
// a real prior incident in the other direction) — so it flags every one of
// them doneUnrecoverable/needs-repair, and that's the intended, safe
// default. Downstream tickets blocked on them then flip open->blocked, the
// frontier empties out, and Run fails with "has no unblocked tickets left
// but isn't all done" — surprising, but not a bug in classification itself.
//
// The supported recovery path once a human (or an auditing agent) has
// confirmed a flagged ticket's commit really is on the feature branch is to
// backfill a cherry-picked event recording its SHA — the same durable
// evidence classifyDoneTicket already trusts for tickets landed the normal
// way. No new persistence mechanism needed; these tests document that the
// existing one is sufficient once the SHA is known.

// TestClassifyDoneTicket_IterationStartedButNeverLanded_StillUnrecoverable
// guards a related edge: once ralph-loop's own run log shows it actually
// started an iteration for this ticket, "no cherry-pick event and no
// surviving branch" is real signal of lost work (a crash between start and
// land) — it must stay doneUnrecoverable regardless of what else is in the
// log.
func TestClassifyDoneTicket_IterationStartedButNeverLanded_StillUnrecoverable(t *testing.T) {
	t.Parallel()
	d, ticket := classifyDoneTicketFixture()
	d.IsAncestor = func(dir, ancestor, descendant string) (bool, error) { return false, nil }
	d.RevParse = func(dir, ref string) (string, error) { return "", fmt.Errorf("unknown revision") }
	d.WorktreeExists = func(path string) (bool, error) { return false, nil }

	events := []Event{
		{Type: eventIterationStarted, Ticket: "03"},
	}

	class, err := classifyDoneTicket(d, reconcilePaths{FeatureWorktree: "/fake/feature", WorktreeDir: "/fake/worktrees"}, "epic", ticket, events, map[string]bool{}, map[string]bool{})
	if err != nil {
		t.Fatalf("classifyDoneTicket() error = %v", err)
	}
	if class != doneUnrecoverable {
		t.Errorf("class = %v, want doneUnrecoverable when ralph-loop's own log shows it started this ticket but no landing evidence survives", class)
	}
}

// TestReconcile_DoneTicketWithNoProvenance_FlaggedNeedsRepairNotSilently
// exercises the full reconcile() entrypoint for the exact shape reported
// live: a done ticket with no run-log history at all (ralph-loop has never
// run against this scratch dir). It documents the intended behavior — this
// is reported, not silently trusted or silently corrupted — with the
// specific reason a human needs to see to know what to check.
func TestReconcile_DoneTicketWithNoProvenance_FlaggedNeedsRepairNotSilently(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "epic", map[string]string{
		"03-c.md": "---\nid: \"03\"\nstatus: done\ntype: task\n---\n# C\n",
	})
	epics, err := tickets.Load(scratchDir)
	if err != nil {
		t.Fatalf("tickets.Load: %v", err)
	}

	d, _, _ := fakeDeps()
	d.TabList = func(workspaceID string) ([]herdr.Tab, error) { return nil, nil }
	d.IsAncestor = func(dir, ancestor, descendant string) (bool, error) { return false, nil }
	d.RevParse = func(dir, ref string) (string, error) { return "", fmt.Errorf("unknown revision") }

	sink := newRecordingEventSink()
	if _, err := reconcile(d, testReconcileParams("ws1", reconcilePaths{ScratchDir: scratchDir, FeatureWorktree: "/fake/feature", WorktreeDir: "/fake/worktrees"}, sink), epics[0]); err != nil {
		t.Fatalf("reconcile() error = %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(scratchDir, "epic", "issues", "03-c.md"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(raw), "status: needs-repair") {
		t.Errorf("ticket not flagged needs-repair for a human to check:\n%s", raw)
	}
	if !hasEvent(sink, LiveEventTicketUnrecoverable, func(ev LiveEvent) bool { return ev.Identifier == "03" }) {
		t.Errorf("events = %+v, want an unrecoverable-mismatch event naming ticket 03", sink.Events())
	}
}

// TestReconcile_CommitlessDoneTicket_NotFlaggedUnrecoverable is the
// commitless counterpart to TestReconcile_DoneTicketWithNoProvenance_
// FlaggedNeedsRepairNotSilently above: a done ticket with commitless: true
// has no landed commit by design, so classifyDoneTicket's verification must
// be skipped for it entirely rather than flagging it needs-repair for
// having no provenance.
func TestReconcile_CommitlessDoneTicket_NotFlaggedUnrecoverable(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "epic", map[string]string{
		"03-c.md": "---\nid: \"03\"\nstatus: done\ntype: task\ncommitless: true\n---\n# C\n",
	})
	epics, err := tickets.Load(scratchDir)
	if err != nil {
		t.Fatalf("tickets.Load: %v", err)
	}

	d, _, _ := fakeDeps()
	d.TabList = func(workspaceID string) ([]herdr.Tab, error) { return nil, nil }
	d.IsAncestor = func(dir, ancestor, descendant string) (bool, error) { return false, nil }
	d.RevParse = func(dir, ref string) (string, error) { return "", fmt.Errorf("unknown revision") }

	if _, err := reconcile(d, testReconcileParams("ws1", reconcilePaths{ScratchDir: scratchDir, FeatureWorktree: "/fake/feature", WorktreeDir: "/fake/worktrees"}, noopEventSink{}), epics[0]); err != nil {
		t.Fatalf("reconcile() error = %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(scratchDir, "epic", "issues", "03-c.md"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if strings.Contains(string(raw), "status: needs-repair") {
		t.Errorf("commitless done ticket was flagged needs-repair, want its no-commit verification skipped entirely:\n%s", raw)
	}
	if !strings.Contains(string(raw), "status: done") {
		t.Errorf("commitless done ticket's status changed unexpectedly:\n%s", raw)
	}
}

// TestReconcile_ResearchGrillingCodeReviewDoneTickets_NotFlaggedUnrecoverable
// is the type-inferred counterpart to
// TestReconcile_CommitlessDoneTicket_NotFlaggedUnrecoverable above:
// research/grilling/code-review tickets never land a commit on the feature
// branch even when finished correctly (their deliverable is the ticket body
// itself, or — for code-review — the follow-up tickets it opens), so
// schema.Ticket.IsCommitless treats them as commitless by type — no
// per-ticket commitless: true needed.
func TestReconcile_ResearchGrillingCodeReviewDoneTickets_NotFlaggedUnrecoverable(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-r.md": "---\nid: \"01\"\nstatus: done\ntype: research\n---\n# R\n",
		"02-g.md": "---\nid: \"02\"\nstatus: done\ntype: grilling\n---\n# G\n",
		"03-c.md": "---\nid: \"03\"\nstatus: done\ntype: code-review\n---\n# C\n",
	})
	epics, err := tickets.Load(scratchDir)
	if err != nil {
		t.Fatalf("tickets.Load: %v", err)
	}

	d, _, _ := fakeDeps()
	d.TabList = func(workspaceID string) ([]herdr.Tab, error) { return nil, nil }
	d.IsAncestor = func(dir, ancestor, descendant string) (bool, error) { return false, nil }
	d.RevParse = func(dir, ref string) (string, error) { return "", fmt.Errorf("unknown revision") }

	if _, err := reconcile(d, testReconcileParams("ws1", reconcilePaths{ScratchDir: scratchDir, FeatureWorktree: "/fake/feature", WorktreeDir: "/fake/worktrees"}, noopEventSink{}), epics[0]); err != nil {
		t.Fatalf("reconcile() error = %v", err)
	}

	for _, name := range []string{"01-r.md", "02-g.md", "03-c.md"} {
		raw, err := os.ReadFile(filepath.Join(scratchDir, "epic", "issues", name))
		if err != nil {
			t.Fatalf("ReadFile %s: %v", name, err)
		}
		if strings.Contains(string(raw), "status: needs-repair") {
			t.Errorf("%s flagged needs-repair, want its no-commit verification skipped by type:\n%s", name, raw)
		}
		if !strings.Contains(string(raw), "status: done") {
			t.Errorf("%s status changed unexpectedly:\n%s", name, raw)
		}
	}
}

// TestReconcile_PrototypeDoneTicket_StillFlaggedUnrecoverable is a
// regression guard against over-broadening the type-inferred commitless fix
// above: a prototype ticket can legitimately land a real spike/scaffold
// commit as its actual output, so — unlike research/grilling/code-review —
// it must stay on the crash-recovery path unless explicitly flagged
// commitless: true, same as a plain task ticket.
func TestReconcile_PrototypeDoneTicket_StillFlaggedUnrecoverable(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "epic", map[string]string{
		"04-p.md": "---\nid: \"04\"\nstatus: done\ntype: prototype\n---\n# P\n",
	})
	epics, err := tickets.Load(scratchDir)
	if err != nil {
		t.Fatalf("tickets.Load: %v", err)
	}

	d, _, _ := fakeDeps()
	d.TabList = func(workspaceID string) ([]herdr.Tab, error) { return nil, nil }
	d.IsAncestor = func(dir, ancestor, descendant string) (bool, error) { return false, nil }
	d.RevParse = func(dir, ref string) (string, error) { return "", fmt.Errorf("unknown revision") }

	if _, err := reconcile(d, testReconcileParams("ws1", reconcilePaths{ScratchDir: scratchDir, FeatureWorktree: "/fake/feature", WorktreeDir: "/fake/worktrees"}, noopEventSink{}), epics[0]); err != nil {
		t.Fatalf("reconcile() error = %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(scratchDir, "epic", "issues", "04-p.md"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(raw), "status: needs-repair") {
		t.Errorf("prototype ticket not flagged needs-repair, want unchanged crash-recovery behavior:\n%s", raw)
	}
}

// TestReconcile_OutOfScopeDoneTicket_NotVerified covers the bug reported
// live: a queue run scoped to a handful of tickets (e.g. via the Queue UI's
// checked selection) still swept every done ticket in the whole epic for
// landed-commit verification, flagging unrelated done tickets the run never
// touched needs-repair. Scope now gates this loop the same way it already
// gated claim/needs-repair reattachment above.
func TestReconcile_OutOfScopeDoneTicket_NotVerified(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: done\ntype: task\n---\n# A\n",
		"02-b.md": "---\nid: \"02\"\nstatus: open\ntype: task\n---\n# B\n",
	})
	epics, err := tickets.Load(scratchDir)
	if err != nil {
		t.Fatalf("tickets.Load: %v", err)
	}
	epic := epics[0]

	scope, err := ResolveRunScope(epic, []string{"02"})
	if err != nil {
		t.Fatalf("ResolveRunScope: %v", err)
	}

	d, _, _ := fakeDeps()
	d.TabList = func(workspaceID string) ([]herdr.Tab, error) { return nil, nil }
	d.IsAncestor = func(dir, ancestor, descendant string) (bool, error) { return false, nil }
	d.RevParse = func(dir, ref string) (string, error) { return "", fmt.Errorf("unknown revision") }

	params := testReconcileParams("ws1", reconcilePaths{ScratchDir: scratchDir, FeatureWorktree: "/fake/feature", WorktreeDir: "/fake/worktrees"}, noopEventSink{})
	params.Scope = scope
	if _, err := reconcile(d, params, epic); err != nil {
		t.Fatalf("reconcile() error = %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(scratchDir, "epic", "issues", "01-a.md"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if strings.Contains(string(raw), "status: needs-repair") {
		t.Errorf("out-of-scope done ticket 01 was verified and flagged needs-repair, want it left untouched:\n%s", raw)
	}
}

// TestClassifyDoneTicket_BackfilledCherryPickEvent_RecognizedAsLanded is the
// recovery path for a ticket flagged needs-repair per the test above,
// once a human/auditing agent has confirmed its commit really is on the
// feature branch: append a cherry-picked event recording that SHA to the run
// log, exactly as landCherryPick does for a normal iteration. No new
// persistence format needed — this is the same evidence classifyDoneTicket
// already trusts.
func TestClassifyDoneTicket_BackfilledCherryPickEvent_RecognizedAsLanded(t *testing.T) {
	t.Parallel()
	d, ticket := classifyDoneTicketFixture()
	d.IsAncestor = func(dir, ancestor, descendant string) (bool, error) {
		return ancestor == "confirmed-landed-sha", nil
	}
	d.RevParse = func(dir, ref string) (string, error) { return "", fmt.Errorf("unknown revision") }
	d.WorktreeExists = func(path string) (bool, error) { return false, nil }

	// A human/auditing agent confirmed 03's work landed at this SHA and
	// backfilled the missing record.
	events := []Event{{Type: eventCherryPicked, Ticket: "03", SHA: "confirmed-landed-sha"}}

	class, err := classifyDoneTicket(d, reconcilePaths{FeatureWorktree: "/fake/feature", WorktreeDir: "/fake/worktrees"}, "epic", ticket, events, map[string]bool{}, map[string]bool{})
	if err != nil {
		t.Fatalf("classifyDoneTicket() error = %v", err)
	}
	if class != doneOK {
		t.Errorf("class = %v, want doneOK once the landed SHA is backfilled into the run log", class)
	}
}

// TestRun_BackfilledProvenance_UnblocksDependentsAndCompletesEpic is the
// full end-to-end reproduction of the reported incident and its recovery: an
// epic where a done ticket was landed outside ralph-loop's tracked flow (no
// evidence), with a dependent ticket blocked on it. Before backfilling, Run
// must fail loudly rather than corrupt/silently trust anything (asserted
// first); after backfilling the confirmed landed SHA as a cherry-picked
// event, Run must unblock the dependent and finish the epic — this is the
// supported way out of the exact stuck state from the field report, not a
// hand-edit of ticket frontmatter.
func TestRun_BackfilledProvenance_UnblocksDependentsAndCompletesEpic(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "my-epic", map[string]string{
		"02-b.md": "---\nid: \"02\"\nstatus: done\ntype: task\n---\n# B\n",
		"03-d.md": "---\nid: \"03\"\nstatus: open\ntype: task\nblocked_by: [\"02\"]\n---\n# D\n",
	})

	d, _, _ := fakeDeps()
	d.TabList = func(workspaceID string) ([]herdr.Tab, error) { return nil, nil }
	d.IsAncestor = func(dir, ancestor, descendant string) (bool, error) {
		return ancestor == "confirmed-landed-sha-02", nil
	}
	// Ticket 02's iteration branch is gone (no evidence to recover it), but
	// other RevParse calls — e.g. a fresh iteration landing ticket 03's own
	// commit and recording its tip SHA — must resolve normally.
	d.RevParse = func(dir, ref string) (string, error) {
		if ref == iterBranch("my-epic", "02") {
			return "", fmt.Errorf("unknown revision")
		}
		return "deadbeef", nil
	}

	// Ticket 02 is flagged needs-repair, which blocks ticket 03 and leaves
	// nothing runnable: the run parks on the flag rather than trusting the
	// unproven done, and waits for the backfill below.
	runUntilParked(t, RunOptions{EpicName: "my-epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, noopEventSink{})

	raw, err := os.ReadFile(filepath.Join(scratchDir, "my-epic", "issues", "02-b.md"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(raw), "status: needs-repair") {
		t.Fatalf("ticket 02 not flagged needs-repair before backfill:\n%s", raw)
	}

	// Recovery: a human/auditing agent confirms ticket 02's work is really on
	// the feature branch and backfills the run log accordingly. Also revert
	// the needs-repair flag back to done — reconcile only reports
	// mismatches, repairing the ticket file itself is this recovery step's
	// job (mirrors gx tickets set --status done).
	if err := SetStatus(filepath.Join(scratchDir, "my-epic", "issues", "02-b.md"), "done"); err != nil {
		t.Fatalf("SetStatus: %v", err)
	}
	if err := logEvent(scratchDir, "my-epic", Event{Type: eventCherryPicked, Ticket: "02", SHA: "confirmed-landed-sha-02"}); err != nil {
		t.Fatalf("logEvent: %v", err)
	}

	if err := Run(RunOptions{EpicName: "my-epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, noopEventSink{}); err != nil {
		t.Fatalf("Run() (after backfill) error = %v, want the epic to unblock and finish", err)
	}

	for _, name := range []string{"02-b.md", "03-d.md"} {
		raw, err := os.ReadFile(filepath.Join(scratchDir, "my-epic", "issues", name))
		if err != nil {
			t.Fatalf("ReadFile %s: %v", name, err)
		}
		if !strings.Contains(string(raw), "status: done") {
			t.Errorf("%s not done after backfill+rerun:\n%s", name, raw)
		}
	}
}
