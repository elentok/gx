package ralphloop

import (
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

// landFixture is a repo whose main branch can land a commit from "iter".
func landFixture(t *testing.T, conflicting bool) (dir string, lp LandParams) {
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
	return dir, LandParams{
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

func TestLandTicket_Landed_StampsTrailerScopedByFeatureBranch(t *testing.T) {
	dir, lp := landFixture(t, false)

	res, err := LandTicket(landDepsFor(testDeps()), lp)
	if err != nil {
		t.Fatalf("LandTicket: %v", err)
	}
	if res.Outcome != Landed {
		t.Fatalf("outcome = %q, want %q", res.Outcome, Landed)
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

func TestLandTicket_NoRecoverableSession_OmitsMetrics(t *testing.T) {
	dir, lp := landFixture(t, false)

	res, err := LandTicket(landDepsFor(testDeps()), lp)
	if err != nil {
		t.Fatalf("LandTicket: %v", err)
	}
	if res.MetricsStamped {
		t.Error("MetricsStamped = true with no session, want false")
	}
	if msg := headMessage(t, dir); strings.Contains(msg, tokensTrailerKey) || strings.Contains(msg, elapsedTrailerKey) {
		t.Errorf("metrics trailers fabricated without a session:\n%s", msg)
	}
}

func TestLandTicket_SecondCall_IsAlreadyAppliedWithoutRestamp(t *testing.T) {
	dir, lp := landFixture(t, false)
	d := landDepsFor(testDeps())

	first, err := LandTicket(d, lp)
	if err != nil || first.Outcome != Landed {
		t.Fatalf("first LandTicket = (%+v, %v), want Landed", first, err)
	}
	msg := headMessage(t, dir)

	second, err := LandTicket(d, lp)
	if err != nil {
		t.Fatalf("second LandTicket: %v", err)
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

func TestLandTicket_TrailerRung_DetectsAlreadyApplied(t *testing.T) {
	_, lp := landFixture(t, false)
	d := landDepsFor(testDeps())
	d.PatchesApplied = func(dir, upstream, base, branch string) (bool, error) { return false, nil }
	d.LandedTickets = func(dir, branch string) (map[string]bool, error) { return map[string]bool{"03": true}, nil }

	res, err := LandTicket(d, lp)
	if err != nil || res.Outcome != AlreadyApplied {
		t.Fatalf("LandTicket = (%+v, %v), want AlreadyApplied", res, err)
	}
}

func TestLandTicket_RecordedSHARung_DetectsAlreadyApplied(t *testing.T) {
	_, lp := landFixture(t, false)
	lp.RecordedSHA = "recorded"
	d := landDepsFor(testDeps())
	d.IsAncestor = func(dir, ancestor, descendant string) (bool, error) { return ancestor == "recorded", nil }

	res, err := LandTicket(d, lp)
	if err != nil || res.Outcome != AlreadyApplied || res.SHA != "recorded" {
		t.Fatalf("LandTicket = (%+v, %v), want AlreadyApplied at recorded SHA", res, err)
	}
}

func TestLandTicket_Conflict_IsAResultNotAnError(t *testing.T) {
	dir, lp := landFixture(t, true)

	res, err := LandTicket(landDepsFor(testDeps()), lp)
	if err != nil {
		t.Fatalf("LandTicket error = %v, want nil on conflict", err)
	}
	if res.Outcome != Conflicted {
		t.Fatalf("outcome = %q, want %q", res.Outcome, Conflicted)
	}
	if inProgress, _ := git.CherryPickInProgress(dir); !inProgress {
		t.Error("sequencer not left in progress after conflict")
	}
}

// appliedSessionFixture lands the fixture's commit, then points lp at a ticket
// file with the given frontmatter cost and a recoverable Claude session.
func appliedSessionFixture(t *testing.T, cost string) (lp LandParams, d LandDeps, ticketPath string) {
	t.Helper()
	setHomeEnv(t, t.TempDir())
	_, lp = landFixture(t, false)
	d = landDepsFor(testDeps())
	if res, err := LandTicket(d, lp); err != nil || res.Outcome != Landed {
		t.Fatalf("first LandTicket = (%+v, %v), want Landed", res, err)
	}

	ticketPath = filepath.Join(t.TempDir(), "03-a.md")
	body := "---\nid: \"03\"\nstatus: claimed\ntype: task\n" + cost + "---\n# A\n"
	if err := os.WriteFile(ticketPath, []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cwd := iterationWorktreePath("/fake/worktrees", "epic", "03")
	writeFakeTranscript(t, "", cwd, "sess-applied", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		[3]any{"claude-sonnet-5", 1000, 0},
		[3]any{"claude-sonnet-5", 2000, 5000},
	)
	lp.TicketPath = ticketPath
	lp.Session = LandSession{Agent: AgentClaude, Cwd: cwd, ID: "sess-applied"}
	return lp, d, ticketPath
}

func TestLandTicket_AlreadyApplied_ZeroCostWithSession_StampsMetrics(t *testing.T) {
	lp, d, ticketPath := appliedSessionFixture(t, "")
	msg := headMessage(t, lp.FeatureWorktree)

	res, err := LandTicket(d, lp)
	if err != nil || res.Outcome != AlreadyApplied {
		t.Fatalf("LandTicket = (%+v, %v), want AlreadyApplied", res, err)
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

func TestLandTicket_AlreadyApplied_NonZeroCost_StampsNothing(t *testing.T) {
	lp, d, ticketPath := appliedSessionFixture(t, "actual_cost: 1.5\n")

	res, err := LandTicket(d, lp)
	if err != nil || res.Outcome != AlreadyApplied {
		t.Fatalf("LandTicket = (%+v, %v), want AlreadyApplied", res, err)
	}
	if res.MetricsStamped {
		t.Error("MetricsStamped = true with non-zero cost, want false")
	}
	raw, _ := os.ReadFile(ticketPath)
	if !strings.Contains(string(raw), "actual_cost: 1.5") {
		t.Errorf("cost overwritten:\n%s", raw)
	}
}

func TestLandTicket_AlreadyApplied_NoSession_StampsNothing(t *testing.T) {
	lp, d, ticketPath := appliedSessionFixture(t, "")
	lp.Session = LandSession{}

	res, err := LandTicket(d, lp)
	if err != nil || res.Outcome != AlreadyApplied {
		t.Fatalf("LandTicket = (%+v, %v), want AlreadyApplied", res, err)
	}
	if res.MetricsStamped {
		t.Error("MetricsStamped = true with no session, want false")
	}
	raw, _ := os.ReadFile(ticketPath)
	if strings.Contains(string(raw), "actual_cost") {
		t.Errorf("metrics written without a session:\n%s", raw)
	}
}
