package ralphloop

import (
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/elentok/gx/tickets/schema"
	"os/exec"
	"strings"
	"testing"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
)

// mergeFixture is a repo whose main branch can merge a commit from "iter".
func mergeFixture(t *testing.T, conflicting bool) (dir string, lp MergeParams) {
	t.Helper()
	dir = testutil.TempRepo(t)
	base, _ := git.RevParse(dir, "HEAD")
	testutil.MustGitExported(t, dir, "checkout", "-b", "iter", base)
	testutil.WriteFile(t, dir, "shared.txt", "iteration\n")
	testutil.CommitAll(t, dir, "iter shared")
	tip, _ := git.RevParse(dir, "HEAD")
	testutil.MustGitExported(t, dir, "checkout", "main")
	if conflicting {
		testutil.WriteFile(t, dir, "shared.txt", "main\n")
		testutil.CommitAll(t, dir, "main shared")
	}
	return dir, MergeParams{
		FeatureWorktree: dir,
		FeatureBranch:   "main",
		TicketID:        "03",
		SourceRange:     SourceRange{Base: base, Tip: tip},
	}
}

func headMessage(t *testing.T, dir string) string {
	t.Helper()
	out, err := exec.Command("git", "-C", dir, "log", "-1", "--format=%B").Output()
	if err != nil {
		t.Fatalf("git log: %v", err)
	}
	return string(out)
}

func TestMergeTicket_Merged_StampsTrailerScopedByFeatureBranch(t *testing.T) {
	dir, lp := mergeFixture(t, false)

	res, err := MergeTicket(mergeDepsFor(testDeps()), lp)
	if err != nil {
		t.Fatalf("MergeTicket: %v", err)
	}
	if res.Outcome != Merged {
		t.Fatalf("outcome = %q, want %q", res.Outcome, Merged)
	}
	if want := ticketTrailerValue("main", "03"); res.TrailerValue != want {
		t.Errorf("TrailerValue = %q, want %q", res.TrailerValue, want)
	}
	if head, _ := git.RevParse(dir, "HEAD"); res.SHA != head {
		t.Errorf("SHA = %q, want HEAD %q", res.SHA, head)
	}
	if !strings.Contains(headMessage(t, dir), ticketTrailerKey+": main/03") {
		t.Errorf("HEAD message lacks scoped trailer:\n%s", headMessage(t, dir))
	}
}

func TestMergeTicket_NoRecoverableSession_OmitsMetrics(t *testing.T) {
	dir, lp := mergeFixture(t, false)

	res, err := MergeTicket(mergeDepsFor(testDeps()), lp)
	if err != nil {
		t.Fatalf("MergeTicket: %v", err)
	}
	if res.MetricsStamped {
		t.Error("MetricsStamped = true with no session, want false")
	}
	if msg := headMessage(t, dir); strings.Contains(msg, tokensTrailerKey) || strings.Contains(msg, elapsedTrailerKey) {
		t.Errorf("metrics trailers fabricated without a session:\n%s", msg)
	}
}

func TestMergeTicket_SecondCall_IsAlreadyAppliedWithoutRestamp(t *testing.T) {
	dir, lp := mergeFixture(t, false)
	d := mergeDepsFor(testDeps())

	first, err := MergeTicket(d, lp)
	if err != nil || first.Outcome != Merged {
		t.Fatalf("first MergeTicket = (%+v, %v), want Merged", first, err)
	}
	msg := headMessage(t, dir)

	second, err := MergeTicket(d, lp)
	if err != nil {
		t.Fatalf("second MergeTicket: %v", err)
	}
	if second.Outcome != AlreadyApplied {
		t.Fatalf("second outcome = %q, want %q", second.Outcome, AlreadyApplied)
	}
	if second.SHA != first.SHA {
		t.Errorf("second SHA = %q, want %q", second.SHA, first.SHA)
	}
	if got := headMessage(t, dir); got != msg {
		t.Errorf("HEAD message changed on second call:\n%s\nwas:\n%s", got, msg)
	}
}

func TestMergeTicket_TrailerOnly_IsAmbiguousNamingTheSHAs(t *testing.T) {
	_, lp := mergeFixture(t, false)
	lp.RecordedSHA = "recorded"
	d := mergeDepsFor(testDeps())
	d.IsAncestor = func(dir, ancestor, descendant string) (bool, error) { return false, nil }
	d.PatchesApplied = func(dir, upstream, base, branch string) (bool, error) { return false, nil }
	d.MergedTickets = func(dir, branch string) (map[string]bool, error) { return map[string]bool{"03": true}, nil }

	res, err := MergeTicket(d, lp)
	var amb *ambiguousMergeError
	if !errors.As(err, &amb) {
		t.Fatalf("MergeTicket = (%+v, %v), want an ambiguous-merge error", res, err)
	}
	for _, want := range []string{"03", "main", "recorded", lp.SourceRange.Base, lp.SourceRange.Tip} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("reason %q does not name %q", err.Error(), want)
		}
	}
}

func TestMergeTicket_TrailerOnlyWithoutRecordedSHA_SaysNone(t *testing.T) {
	_, lp := mergeFixture(t, false)
	d := mergeDepsFor(testDeps())
	d.PatchesApplied = func(dir, upstream, base, branch string) (bool, error) { return false, nil }
	d.MergedTickets = func(dir, branch string) (map[string]bool, error) { return map[string]bool{"03": true}, nil }

	_, err := MergeTicket(d, lp)
	if err == nil || !strings.Contains(err.Error(), "recorded SHA none") {
		t.Fatalf("MergeTicket err = %v, want it to say recorded SHA none", err)
	}
}

func TestMergeTicket_RecordedSHARung_DetectsAlreadyApplied(t *testing.T) {
	_, lp := mergeFixture(t, false)
	lp.RecordedSHA = "recorded"
	d := mergeDepsFor(testDeps())
	d.IsAncestor = func(dir, ancestor, descendant string) (bool, error) { return ancestor == "recorded", nil }

	res, err := MergeTicket(d, lp)
	if err != nil || res.Outcome != AlreadyApplied || res.SHA != "recorded" {
		t.Fatalf("MergeTicket = (%+v, %v), want AlreadyApplied at recorded SHA", res, err)
	}
}

// A range whose commits are already on the feature branch (e.g. `merge --from
// X^ --to X` for a merged X) has no git cherry output at all, which
// PatchesApplied reads as "unverified"; re-picking it would conflict with
// itself or come up empty.
func TestMergeTicket_RangeAlreadyOnFeatureBranch_IsAlreadyApplied(t *testing.T) {
	dir, lp := mergeFixture(t, false)
	testutil.MustGitExported(t, dir, "merge", "--ff-only", lp.SourceRange.Tip)

	res, err := MergeTicket(mergeDepsFor(testDeps()), lp)
	if err != nil || res.Outcome != AlreadyApplied {
		t.Fatalf("MergeTicket = (%+v, %v), want AlreadyApplied", res, err)
	}
	if res.SHA != lp.SourceRange.Tip {
		t.Errorf("SHA = %q, want the range's tip %q", res.SHA, lp.SourceRange.Tip)
	}
}

func TestMergeTicket_Conflict_IsAResultNotAnError(t *testing.T) {
	dir, lp := mergeFixture(t, true)

	res, err := MergeTicket(mergeDepsFor(testDeps()), lp)
	if err != nil {
		t.Fatalf("MergeTicket error = %v, want nil on conflict", err)
	}
	if res.Outcome != Conflicted {
		t.Fatalf("outcome = %q, want %q", res.Outcome, Conflicted)
	}
	if inProgress, _ := git.CherryPickInProgress(dir); !inProgress {
		t.Error("sequencer not left in progress after conflict")
	}
}

// appliedSessionFixture merges the fixture's commit, then points lp at a ticket
// file with the given frontmatter cost and a recoverable Claude session.
func appliedSessionFixture(t *testing.T, cost string) (lp MergeParams, d MergeDeps, ticketPath string) {
	t.Helper()
	home := t.TempDir()
	_, lp = mergeFixture(t, false)
	d = mergeDepsFor(testDeps())
	d.SessionHomes = SessionHomes{Claude: home}
	if res, err := MergeTicket(d, lp); err != nil || res.Outcome != Merged {
		t.Fatalf("first MergeTicket = (%+v, %v), want Merged", res, err)
	}

	ticketPath = filepath.Join(t.TempDir(), "03-a.md")
	body := "---\nid: \"03\"\nstatus: claimed\ntype: implement\n" + cost + "---\n# A\n"
	if err := os.WriteFile(ticketPath, []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cwd := iterationWorktreePath("/fake/worktrees", "epic", "03")
	writeFakeTranscript(t, home, cwd, "sess-applied", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		[3]any{"claude-sonnet-5", 1000, 0},
		[3]any{"claude-sonnet-5", 2000, 5000},
	)
	lp.TicketPath = ticketPath
	lp.Session = MergeSession{Agent: AgentClaude, Cwd: cwd, ID: "sess-applied"}
	return lp, d, ticketPath
}

func TestMergeTicket_AlreadyApplied_ZeroCostWithSession_StampsMetrics(t *testing.T) {
	lp, d, ticketPath := appliedSessionFixture(t, "")
	msg := headMessage(t, lp.FeatureWorktree)

	res, err := MergeTicket(d, lp)
	if err != nil || res.Outcome != AlreadyApplied {
		t.Fatalf("MergeTicket = (%+v, %v), want AlreadyApplied", res, err)
	}
	if !res.MetricsStamped {
		t.Error("MetricsStamped = false, want true")
	}
	raw, _ := os.ReadFile(ticketPath)
	ticket, err := schema.ParseTicketFromRaw(string(raw), ticketPath)
	if err != nil || ticket.ActualCost == 0 {
		t.Errorf("ActualCost = %v (err %v), want non-zero:\n%s", ticket.ActualCost, err, raw)
	}
	if got := headMessage(t, lp.FeatureWorktree); got != msg {
		t.Errorf("HEAD message changed (trailer re-stamped):\n%s\nwas:\n%s", got, msg)
	}
}

func TestMergeTicket_AlreadyApplied_AlreadyStamped_StampsNothing(t *testing.T) {
	lp, d, ticketPath := appliedSessionFixture(t, "total_tokens: 100\nactual_cost: 1.5\n")

	res, err := MergeTicket(d, lp)
	if err != nil || res.Outcome != AlreadyApplied {
		t.Fatalf("MergeTicket = (%+v, %v), want AlreadyApplied", res, err)
	}
	if res.MetricsStamped {
		t.Error("MetricsStamped = true with total_tokens already stamped, want false")
	}
	raw, _ := os.ReadFile(ticketPath)
	if !strings.Contains(string(raw), "actual_cost: 1.5") {
		t.Errorf("cost overwritten:\n%s", raw)
	}
}

func TestMergeTicket_AlreadyApplied_NoSession_StampsNothing(t *testing.T) {
	lp, d, ticketPath := appliedSessionFixture(t, "")
	lp.Session = MergeSession{}

	res, err := MergeTicket(d, lp)
	if err != nil || res.Outcome != AlreadyApplied {
		t.Fatalf("MergeTicket = (%+v, %v), want AlreadyApplied", res, err)
	}
	if res.MetricsStamped {
		t.Error("MetricsStamped = true with no session, want false")
	}
	raw, _ := os.ReadFile(ticketPath)
	if strings.Contains(string(raw), "actual_cost") {
		t.Errorf("metrics written without a session:\n%s", raw)
	}
}
