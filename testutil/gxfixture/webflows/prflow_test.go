package webflows

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil"
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
			r.Stop(t)
		})
	}
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
