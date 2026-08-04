package ralphloop

import (
	"os"
	"os/exec"
	"testing"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets"
)

// realGitDeps wires IsAncestor/RevParse/WorktreeExists to the real git
// package and filesystem, leaving every other Deps field faked — used by the
// classifyDoneTicket tests below that need genuine git plumbing (patch
// hashes, ancestor checks) rather than a closure asserting the answer it was
// told to give.
func realGitDeps() Deps {
	d, _, _ := fakeDeps()
	d.IsAncestor = git.IsAncestor
	d.RevParse = git.RevParse
	d.WorktreeExists = worktreeExists
	d.MergeBase = git.MergeBase
	d.PatchesApplied = git.PatchesApplied
	d.AppendTrailers = git.AppendTrailers
	return d
}

// TestClassifyDoneTicket_RealRepo_CommitLandedAndCleanedUp_OK exercises
// classifyDoneTicket against an actual git repo: an iteration branch's
// commit cherry-picked onto the feature branch (a fresh commit hash, exactly
// as CherryPickRange produces in production), then the iteration branch
// itself deleted — the fully-cleaned-up state a future cleanup step leaves
// behind. Verifies the real git.IsAncestor/git.RevParse wiring, not just the
// classifier's own branching logic.
func TestClassifyDoneTicket_RealRepo_CommitLandedAndCleanedUp_OK(t *testing.T) {
	dir := testutil.TempRepo(t)
	base, err := git.RevParse(dir, "HEAD")
	if err != nil {
		t.Fatalf("RevParse: %v", err)
	}

	testutil.MustGitExported(t, dir, "checkout", "-b", "ralph-loop/main-item-03", base)
	testutil.WriteFile(t, dir, "a.txt", "a\n")
	testutil.CommitAll(t, dir, "add a")
	iterTip, err := git.RevParse(dir, "HEAD")
	if err != nil {
		t.Fatalf("RevParse: %v", err)
	}

	testutil.MustGitExported(t, dir, "checkout", "main")
	if err := git.CherryPickRange(dir, base, iterTip); err != nil {
		t.Fatalf("CherryPickRange: %v", err)
	}
	landedSHA, err := git.RevParse(dir, "HEAD")
	if err != nil {
		t.Fatalf("RevParse: %v", err)
	}

	testutil.MustGitExported(t, dir, "branch", "-D", "ralph-loop/main-item-03")

	d := realGitDeps()
	events := []Event{{Type: eventCherryPicked, Ticket: "03", SHA: landedSHA}}
	class, err := classifyDoneTicket(d, reconcilePaths{FeatureWorktree: dir, WorktreeDir: "/fake/worktrees"}, "main", tickets.Ticket{Number: 3, Identifier: "03", Status: "done"}, events, map[string]bool{}, map[string]bool{})
	if err != nil {
		t.Fatalf("classifyDoneTicket() error = %v", err)
	}
	if class != doneOK {
		t.Errorf("class = %v, want doneOK", class)
	}
}

// TestClassifyDoneTicket_RealRepo_NeverCherryPicked_Recoverable exercises
// classifyDoneTicket against a real repo where the iteration branch's commit
// was never cherry-picked onto the feature branch at all (no run-log event,
// matching a crash before that step), but the iteration branch itself is
// still around to recover it from.
func TestClassifyDoneTicket_RealRepo_NeverCherryPicked_Recoverable(t *testing.T) {
	dir := testutil.TempRepo(t)
	base, err := git.RevParse(dir, "HEAD")
	if err != nil {
		t.Fatalf("RevParse: %v", err)
	}

	testutil.MustGitExported(t, dir, "checkout", "-b", "ralph-loop/main-item-03", base)
	testutil.WriteFile(t, dir, "a.txt", "a\n")
	testutil.CommitAll(t, dir, "add a")

	testutil.MustGitExported(t, dir, "checkout", "main")

	d := realGitDeps()
	class, err := classifyDoneTicket(d, reconcilePaths{FeatureWorktree: dir, WorktreeDir: "/fake/worktrees"}, "main", tickets.Ticket{Number: 3, Identifier: "03", Status: "done"}, nil, map[string]bool{}, map[string]bool{})
	if err != nil {
		t.Fatalf("classifyDoneTicket() error = %v", err)
	}
	if class != doneRecoverable {
		t.Errorf("class = %v, want doneRecoverable", class)
	}
}

// TestClassifyDoneTicket_RealRepo_RebasedAfterLanding_OK reproduces the
// postmortem scenario that motivated the PatchesApplied fallback: a ticket
// lands normally (cherry-picked, event recorded), but the feature branch is
// later rebased onto a newer upstream, rewriting the landed commit's hash out
// from under the recorded event's SHA — while the iteration branch (holding
// the pre-rebase original) never got cleaned up. Without the fallback,
// IsAncestor against the stale SHA reports false and the surviving iteration
// branch would make this misclassify as doneRecoverable, re-cherry-picking
// already-landed content onto the live feature worktree. PatchesApplied
// should recognize the rebased commit as patch-equivalent and classify this
// doneStaleCleanup instead — landed, just needs its leftover branch cleaned
// up.
func TestClassifyDoneTicket_RealRepo_RebasedAfterLanding_OK(t *testing.T) {
	dir := testutil.TempRepo(t)
	base, err := git.RevParse(dir, "HEAD")
	if err != nil {
		t.Fatalf("RevParse: %v", err)
	}

	testutil.MustGitExported(t, dir, "checkout", "-b", "ralph-loop/main-item-03", base)
	testutil.WriteFile(t, dir, "a.txt", "a\n")
	testutil.CommitAll(t, dir, "add a")
	iterTip, err := git.RevParse(dir, "HEAD")
	if err != nil {
		t.Fatalf("RevParse: %v", err)
	}

	testutil.MustGitExported(t, dir, "checkout", "main")
	if err := git.CherryPickRange(dir, base, iterTip); err != nil {
		t.Fatalf("CherryPickRange: %v", err)
	}
	landedSHA, err := git.RevParse(dir, "HEAD")
	if err != nil {
		t.Fatalf("RevParse: %v", err)
	}

	// Simulate an unrelated upstream advance, then rebase main onto it — this
	// rewrites landedSHA to a new hash, exactly as happened to this repo's own
	// ralphloop-tui-impl branch.
	testutil.MustGitExported(t, dir, "checkout", "-b", "upstream", base)
	testutil.WriteFile(t, dir, "upstream.txt", "u\n")
	testutil.CommitAll(t, dir, "unrelated upstream change")
	testutil.MustGitExported(t, dir, "checkout", "main")
	testutil.MustGitExported(t, dir, "rebase", "upstream")

	// ralph-loop/iter-03 was never cleaned up, still pointing at the
	// pre-rebase original.
	d := realGitDeps()
	events := []Event{{Type: eventCherryPicked, Ticket: "03", SHA: landedSHA}}
	class, err := classifyDoneTicket(d, reconcilePaths{FeatureWorktree: dir, WorktreeDir: "/fake/worktrees"}, "main", tickets.Ticket{Number: 3, Identifier: "03", Status: "done"}, events, map[string]bool{}, map[string]bool{})
	if err != nil {
		t.Fatalf("classifyDoneTicket() error = %v", err)
	}
	if class != doneStaleCleanup {
		t.Errorf("class = %v, want doneStaleCleanup — content already landed pre-rebase, just needs the leftover iteration branch cleaned up", class)
	}
}

// TestClassifyDoneTicket_RealRepo_RebasedWithConflictResolution_OK reproduces
// the scenario PatchesApplied still can't handle: the feature branch is
// rebased onto an upstream that conflicts with the already-landed commit, so
// landing it again requires a manually re-resolved merge — which changes the
// commit's hash *and* its diff (so PatchesApplied's patch-id comparison also
// reports "not found"), same as what happened to this repo's own tickets
// 01-03 after ralph-loop hit a real conflict re-landing a commit mid-rebase.
// The iteration branch is also gone by this point (already cleaned up), so
// PatchesApplied's hasBranch guard never even runs. Only the
// Ralph-Loop-Ticket trailer landCherryPick stamps on every landed commit
// survives a rebase's conflict-resolution message-preserving default,
// letting classifyDoneTicket still recognize this as landed instead of
// flagging a genuinely-done ticket doneUnrecoverable.
func TestClassifyDoneTicket_RealRepo_RebasedWithConflictResolution_OK(t *testing.T) {
	dir := testutil.TempRepo(t)
	base, err := git.RevParse(dir, "HEAD")
	if err != nil {
		t.Fatalf("RevParse: %v", err)
	}

	testutil.MustGitExported(t, dir, "checkout", "-b", "ralph-loop/main-item-03", base)
	testutil.WriteFile(t, dir, "a.txt", "iteration content\n")
	testutil.CommitAll(t, dir, "add a")
	iterTip, err := git.RevParse(dir, "HEAD")
	if err != nil {
		t.Fatalf("RevParse: %v", err)
	}

	testutil.MustGitExported(t, dir, "checkout", "main")
	if err := git.CherryPickRange(dir, base, iterTip); err != nil {
		t.Fatalf("CherryPickRange: %v", err)
	}
	// landCherryPick always stamps the ticket trailer right after landing,
	// scoped to the epic (see ticketTrailerValue) so a same-numbered ticket
	// from an unrelated epic can never satisfy this lookup.
	if err := git.AppendTrailer(dir, ticketTrailerKey, ticketTrailerValue("main", "03")); err != nil {
		t.Fatalf("AppendTrailer: %v", err)
	}
	landedSHA, err := git.RevParse(dir, "HEAD")
	if err != nil {
		t.Fatalf("RevParse: %v", err)
	}

	// An unrelated upstream commit that conflicts on the same file, then a
	// rebase forcing a manual re-resolution — this both rewrites landedSHA's
	// hash and changes its diff, defeating IsAncestor and PatchesApplied both.
	testutil.MustGitExported(t, dir, "checkout", "-b", "upstream", base)
	testutil.WriteFile(t, dir, "a.txt", "upstream content\n")
	testutil.CommitAll(t, dir, "unrelated conflicting change")
	testutil.MustGitExported(t, dir, "checkout", "main")
	// The rebase is expected to conflict — resolved by hand below, exactly
	// like cherryPickWithConflictResolution's real conflict-resolution path.
	rebaseCmd := exec.Command("git", "rebase", "upstream")
	rebaseCmd.Dir = dir
	_ = rebaseCmd.Run()
	testutil.WriteFile(t, dir, "a.txt", "resolved content\n")
	testutil.MustGitExported(t, dir, "add", "a.txt")
	continueCmd := exec.Command("git", "rebase", "--continue")
	continueCmd.Dir = dir
	continueCmd.Env = append(os.Environ(), "GIT_EDITOR=true")
	if out, err := continueCmd.CombinedOutput(); err != nil {
		t.Fatalf("git rebase --continue: %v\n%s", err, out)
	}

	// The iteration branch is already cleaned up by this point.
	testutil.MustGitExported(t, dir, "branch", "-D", "ralph-loop/main-item-03")

	d := realGitDeps()
	events := []Event{{Type: eventCherryPicked, Ticket: "03", SHA: landedSHA}}
	landed, err := LandedTickets(dir, "main")
	if err != nil {
		t.Fatalf("LandedTickets: %v", err)
	}
	class, err := classifyDoneTicket(d, reconcilePaths{FeatureWorktree: dir, WorktreeDir: "/fake/worktrees"}, "main", tickets.Ticket{Number: 3, Identifier: "03", Status: "done"}, events, map[string]bool{}, landed)
	if err != nil {
		t.Fatalf("classifyDoneTicket() error = %v", err)
	}
	if class != doneOK {
		t.Errorf("class = %v, want doneOK — trailer marker should recognize the rebased-and-reconflicted commit as landed", class)
	}
}

// TestClassifyDoneTicket_RealRepo_TrailerScopedToEpic_NoCrossEpicFalsePositive
// reproduces a real production incident: an unrelated, much earlier epic's
// own ticket "05" landed and stamped a trailer, still reachable from main's
// history (trailers never expire). This epic's own ticket "05" was never
// cherry-picked at all — no run-log event, no surviving iteration branch. An
// unscoped trailer value ("05") would collide across the two unrelated
// epics, since ticket numbering restarts from 01 every epic; ticketTrailerValue
// scopes the stamped/searched value to the epic name specifically so this
// can't happen (this is exactly what happened in production to gx's own
// tickets-ralph epic's ticket 05: classifyDoneTicket's trailer fallback
// matched an unrelated older epic's same-numbered ticket, misclassified this
// one doneOK, and its worktree/branch were deleted without ever landing).
func TestClassifyDoneTicket_RealRepo_TrailerScopedToEpic_NoCrossEpicFalsePositive(t *testing.T) {
	dir := testutil.TempRepo(t)

	// An unrelated, older epic's own ticket "05" landed here long ago,
	// stamped with the bare (pre-scoping) trailer value historical commits
	// carry — still reachable from main.
	testutil.WriteFile(t, dir, "unrelated.txt", "unrelated\n")
	testutil.CommitAll(t, dir, "old-epic: unrelated ticket 05")
	if err := git.AppendTrailer(dir, ticketTrailerKey, "05"); err != nil {
		t.Fatalf("AppendTrailer: %v", err)
	}

	// This epic's own ticket "05" was never cherry-picked: no run-log event,
	// and its iteration branch is already gone (e.g. cleaned up by an
	// earlier, unrelated pass).
	d := realGitDeps()
	class, err := classifyDoneTicket(d, reconcilePaths{FeatureWorktree: dir, WorktreeDir: "/fake/worktrees"}, "main", tickets.Ticket{Number: 5, Identifier: "05", Status: "done"}, nil, map[string]bool{}, map[string]bool{})
	if err != nil {
		t.Fatalf("classifyDoneTicket() error = %v", err)
	}
	if class == doneOK || class == doneStaleCleanup {
		t.Fatalf("class = %v, want doneRecoverable or doneUnrecoverable — must not treat an unrelated epic's same-numbered ticket's trailer as this ticket landed", class)
	}
}
