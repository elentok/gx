package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/ralphloop"
)

func verifyFixture(t *testing.T) string {
	t.Helper()
	epicPath := filepath.Join(t.TempDir(), "widget-epic")
	issues := filepath.Join(epicPath, "issues")
	if err := os.MkdirAll(issues, 0755); err != nil {
		t.Fatal(err)
	}
	for name, status := range map[string]string{"01-a.md": "done", "02-b.md": "claimed", "03-c.md": "done"} {
		body := "---\nid: \"" + name[:2] + "\"\nstatus: " + status + "\ntype: implement\n---\n# T\n"
		if err := os.WriteFile(filepath.Join(issues, name), []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return epicPath
}

// verifyDepsFor has no iteration branches or recorded SHAs, so a ticket only
// counts as landed through the trailer rung (landedIDs).
func verifyDepsFor(t *testing.T, landedIDs map[string]bool, landedErr error) ralphloop.VerifyDeps {
	t.Helper()
	return ralphloop.VerifyDeps{
		IsAncestor:     func(string, string, string) (bool, error) { return false, nil },
		MergeBase:      func(string, string, string) (string, error) { return "", nil },
		PatchesApplied: func(string, string, string, string) (bool, error) { return false, nil },
		RevParse:       func(string, string) (string, error) { return "", os.ErrNotExist },
		WorktreeExists: func(string) (bool, error) { return false, nil },
		LandedTickets:  func(string, string) (map[string]bool, error) { return landedIDs, landedErr },
	}
}

func runVerify(t *testing.T, run verifyRun, all, jsonMode bool) (string, error) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	err := runTicketsVerify(run, all, jsonMode, &stdout, &stderr)
	return stdout.String(), err
}

func TestRunTicketsVerify_HumanFiltersLandedDone(t *testing.T) {
	t.Parallel()
	run := verifyRun{EpicPath: verifyFixture(t), Deps: verifyDepsFor(t, map[string]bool{"01": true}, nil)}
	out, err := runVerify(t, run, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "\n01 ") {
		t.Errorf("landed+done ticket 01 should be hidden:\n%s", out)
	}
	if !strings.Contains(out, "02") || !strings.Contains(out, "03") {
		t.Errorf("tickets needing attention missing:\n%s", out)
	}
	if !strings.Contains(out, "1 hidden") {
		t.Errorf("hidden-count footer missing:\n%s", out)
	}
}

func TestRunTicketsVerify_LandedButNotDoneStaysVisible(t *testing.T) {
	t.Parallel()
	run := verifyRun{EpicPath: verifyFixture(t), Deps: verifyDepsFor(t, map[string]bool{"02": true}, nil)}
	out, _ := runVerify(t, run, false, false)
	if !strings.Contains(out, "02") {
		t.Errorf("landed-but-claimed 02 should be shown:\n%s", out)
	}
}

func TestRunTicketsVerify_AllShowsEverythingNoFooter(t *testing.T) {
	t.Parallel()
	run := verifyRun{EpicPath: verifyFixture(t), Deps: verifyDepsFor(t, map[string]bool{"01": true}, nil)}
	out, _ := runVerify(t, run, true, false)
	if !strings.Contains(out, "01") || strings.Contains(out, "hidden") {
		t.Errorf("--all output wrong:\n%s", out)
	}
}

func TestRunTicketsVerify_JSONUnfilteredEvenWithoutAll(t *testing.T) {
	t.Parallel()
	run := verifyRun{EpicPath: verifyFixture(t), Deps: verifyDepsFor(t, map[string]bool{"01": true}, nil)}
	out, err := runVerify(t, run, false, true)
	if err != nil {
		t.Fatal(err)
	}
	var res struct {
		Tickets         []map[string]any `json:"tickets"`
		LandingInFlight any              `json:"landing_in_flight"`
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("stdout %q: %v", out, err)
	}
	if len(res.Tickets) != 3 || res.LandingInFlight != nil {
		t.Errorf("got %d tickets, in-flight %v", len(res.Tickets), res.LandingInFlight)
	}
}

func TestRunTicketsVerify_UnknownExitsZero(t *testing.T) {
	t.Parallel()
	run := verifyRun{EpicPath: verifyFixture(t), Deps: verifyDepsFor(t, nil, os.ErrPermission)}
	out, err := runVerify(t, run, false, false)
	if err != nil {
		t.Fatalf("unknown must exit 0, got %v", err)
	}
	if !strings.Contains(out, "unknown") {
		t.Errorf("unknown landing not shown:\n%s", out)
	}
}

func TestRunTicketsVerify_InFlightLabelAndNoLock(t *testing.T) {
	t.Parallel()
	epicPath := verifyFixture(t)
	if err := ralphloop.WriteLandMarker(epicPath, ralphloop.LandMarker{Epic: "widget-epic", Ticket: "02", SourceRange: "a..b", PrePickHead: "c"}); err != nil {
		t.Fatal(err)
	}
	run := verifyRun{EpicPath: epicPath, Deps: verifyDepsFor(t, nil, nil)}
	out, err := runVerify(t, run, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "landing in flight: ticket 02") {
		t.Errorf("in-flight label missing:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(epicPath, "land.lock")); !os.IsNotExist(err) {
		t.Errorf("verify must not take the land lock (stat err = %v)", err)
	}
}

func TestRunTicketsVerify_MissingTicketRefusesInEnvelope(t *testing.T) {
	t.Parallel()
	run := verifyRun{EpicPath: verifyFixture(t), ID: "99", Deps: verifyDepsFor(t, nil, nil)}
	out, err := runVerify(t, run, false, true)
	if err == nil {
		t.Fatal("expected refusal")
	}
	var env RefusalEnvelope
	if jerr := json.Unmarshal([]byte(out), &env); jerr != nil || !env.Refused {
		t.Errorf("envelope = %q (%v)", out, jerr)
	}
}

func TestRunTicketsVerify_ReportsOrphanLock(t *testing.T) {
	t.Parallel()
	epicPath := verifyFixture(t)
	if err := ralphloop.AcquireLandLockFor(epicPath, "widget-epic", "02"); err != nil {
		t.Fatal(err)
	}
	run := verifyRun{EpicPath: epicPath, Deps: verifyDepsFor(t, nil, nil)}
	out, err := runVerify(t, run, false, true)
	if err != nil {
		t.Fatal(err)
	}
	var res verifyResult
	if err := json.Unmarshal([]byte(out), &res); err != nil || res.OrphanLandLock == nil || res.OrphanLandLock.Ticket != "02" {
		t.Errorf("orphan lock not reported: %s (err %v)", out, err)
	}
	human, _ := runVerify(t, run, false, false)
	if !strings.Contains(human, "land lock with no marker") {
		t.Errorf("human output missing orphan lock:\n%s", human)
	}
}
