package ralphloop

import (
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
