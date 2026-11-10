package webflows

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/gxfixture/fixturetest"
	"github.com/elentok/gx/testutil/gxfixture/spec"
)

// prFlowRow is the pr-flow epic's row of /v1/epics.
func (r *running) prFlowRow(t *testing.T) wire.EpicListRow {
	t.Helper()
	var list wire.EpicList
	if err := json.Unmarshal([]byte(r.Gx(t, "server", "epics", "--json")), &list); err != nil {
		t.Fatal(err)
	}
	for _, row := range list.Epics {
		if row.Name == spec.PRFlowEpic {
			return row
		}
	}
	t.Fatalf("no epic %s in %+v", spec.PRFlowEpic, list.Epics)
	return wire.EpicListRow{}
}

func TestPRFlowScenarios_CardsShowTheScriptedPR(t *testing.T) {
	t.Parallel()
	cases := []struct {
		scenario   string
		done       int
		words      string
		checks     string
		chipHas    string
		wantFixing bool
	}{
		{"pr-flow-draft-ci-running", 8, "PR draft · CI running", "2 of 5 checks running", "draft", false},
		{"pr-flow-fixing", 9, "Fixing CI (attempt 1 of 3)", "go-test failed", "#412", true},
		{"pr-flow-ready", 9, "Ready for review", "5 of 5 checks passed", "ready", false},
	}
	for _, tc := range cases {
		t.Run(tc.scenario, func(t *testing.T) {
			t.Parallel()
			r := start(t, tc.scenario)
			if _, err := os.Stat(r.GHState); err != nil || r.Origin == "" {
				t.Fatalf("gh_state = %q, origin = %q, want a state file and an origin", r.GHState, r.Origin)
			}
			// The first watcher poll fills the live PR info.
			var row wire.EpicListRow
			if !testutil.Poll(30*time.Second, 100*time.Millisecond, func() bool {
				row = r.prFlowRow(t)
				return row.PR != nil && row.PR.Checks == tc.checks && row.StatusWords == tc.words
			}) {
				t.Fatalf("row = %+v (pr %+v), want status words %q and checks %q", row, row.PR, tc.words, tc.checks)
			}
			if row.Done != tc.done || row.Total != tc.done+boolInt(tc.wantFixing) {
				t.Errorf("done/total = %d/%d, want %d done of %d", row.Done, row.Total, tc.done, tc.done+boolInt(tc.wantFixing))
			}
			if row.PR.Number != spec.PRFlowNumber || row.PR.URL != spec.PRFlowURL || !strings.Contains(row.PR.Chip, tc.chipHas) {
				t.Errorf("pr = %+v, want #%d at %s with chip containing %q", row.PR, spec.PRFlowNumber, spec.PRFlowURL, tc.chipHas)
			}
			r.assertNotStamped(t)
			r.Stop(t)
		})
	}
}

// assertNotStamped reads the row after the stamp delay: a PR epic that is
// still open must stay unmerged, which only a late read can catch.
func (r *running) assertNotStamped(t *testing.T) {
	t.Helper()
	time.Sleep(4 * time.Second)
	if row := r.prFlowRow(t); !row.MergedAt.IsZero() {
		t.Errorf("open PR epic stamped merged_at: %+v", row)
	}
}

func kinds(as []wire.CardAction) (out []wire.CardActionKind) {
	for _, a := range as {
		out = append(out, a.Kind)
	}
	return out
}

func TestPRFlowScenarios_BlockedAndAwaitingCards(t *testing.T) {
	t.Parallel()
	cases := []struct {
		scenario, words, pill string
		actions               []wire.CardActionKind
		after                 func(t *testing.T, r *running, row wire.EpicListRow)
	}{
		{"pr-flow-ci-failing", "CI failing", "CI failing", []wire.CardActionKind{wire.CardActionTryAgain}, checkActivityList},
		{"pr-flow-merge-blocked", "PR merged", "Merge blocked", []wire.CardActionKind{wire.CardActionStashMerge, wire.CardActionMerge}, checkMergeAfterCleaning},
		{"pr-flow-awaiting", "Implemented", "Awaiting PR", []wire.CardActionKind{wire.CardActionCreatePR}, nil},
		{"pr-flow-merge-project", "Implemented", "Awaiting merge", []wire.CardActionKind{wire.CardActionMerge, wire.CardActionCreatePR}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.scenario, func(t *testing.T) {
			t.Parallel()
			r := start(t, tc.scenario)
			var row wire.EpicListRow
			if !testutil.Poll(30*time.Second, 100*time.Millisecond, func() bool {
				row = r.prFlowRow(t)
				return row.StatusWords == tc.words && row.StatePill == tc.pill
			}) {
				t.Fatalf("row = %+v, want status words %q and pill %q", row, tc.words, tc.pill)
			}
			// The card may carry more buttons (open epic, open PR); these come in this order.
			got := kinds(row.Actions)
			if pos := slices.Index(got, tc.actions[0]); pos < 0 || !slices.Equal(got[pos:min(pos+len(tc.actions), len(got))], tc.actions) {
				t.Errorf("actions = %v, want %v in a row", got, tc.actions)
			}
			if tc.after != nil {
				tc.after(t, r, row)
			}
			r.Stop(t)
		})
	}
}

// checkActivityList: seven entries, newest first, the fix entries linking tickets 11 and 10.
func checkActivityList(t *testing.T, r *running, row wire.EpicListRow) {
	t.Helper()
	var d wire.EpicDetail
	if err := json.Unmarshal([]byte(r.Gx(t, "server", "epics", "detail", row.Address, "--json")), &d); err != nil {
		t.Fatal(err)
	}
	if len(d.PRActivity) != 7 {
		t.Fatalf("activity = %+v, want 7 entries", d.PRActivity)
	}
	if d.PRActivity[0].Text != "Paused: go-test failed 3 times" || d.PRActivity[6].Text != "Draft PR opened" {
		t.Errorf("activity = %+v, want newest first from the pause to the draft PR", d.PRActivity)
	}
	var fixes []string
	for _, e := range d.PRActivity {
		if e.Text == "Fix pushed" {
			fixes = append(fixes, e.Ticket)
		}
	}
	if len(fixes) != 2 || !strings.HasSuffix(fixes[0], "/11") || !strings.HasSuffix(fixes[1], "/10") {
		t.Errorf("fix entries link %v, want tickets 11 then 10", fixes)
	}
}

// checkMergeAfterCleaning: cleaning Trunk and pressing Merge stamps merged_at.
func checkMergeAfterCleaning(t *testing.T, r *running, row wire.EpicListRow) {
	t.Helper()
	if !row.MergedAt.IsZero() {
		t.Fatalf("merged_at = %v before the merge", row.MergedAt)
	}
	if err := os.Remove(filepath.Join(r.Repo(), "same.txt")); err != nil {
		t.Fatal(err)
	}
	// The watcher also retries the Trunk update on its own tick, so the press
	// may find the epic already merged; either way merged_at ends up stamped.
	cmd := exec.Command(fixturetest.GxBin, "server", "epics", "merge", row.Address)
	cmd.Env = r.Env
	if out, err := cmd.CombinedOutput(); err != nil && !strings.Contains(string(out), "already-merged") {
		t.Fatalf("merge: %v\n%s", err, out)
	}
	if !testutil.Poll(30*time.Second, 100*time.Millisecond, func() bool { return !r.prFlowRow(t).MergedAt.IsZero() }) {
		t.Fatalf("merged_at still empty: %+v", r.prFlowRow(t))
	}
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
