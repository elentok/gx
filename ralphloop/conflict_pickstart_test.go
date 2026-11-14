package ralphloop

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/git/history"
	"github.com/elentok/gx/ralphloop/internal/fakes"
	"github.com/elentok/gx/ralphloop/merge"
	"github.com/elentok/gx/ralphloop/worktree"
)

// TestReattachedConflict_StampsFromRecordedPrePickHead covers a reattach whose
// merge lock has no owner HEAD: the stamp must still cover the whole pick
// range, from the HEAD a merge marker recorded.
func TestReattachedConflict_StampsFromRecordedPrePickHead(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		lockHead  string
		marker    *merge.MergeMarker
		wantStart string
	}{
		{name: "lock owner head", lockHead: "lockhead", wantStart: "lockhead"},
		{name: "ownerless lock falls back to the marker", marker: &merge.MergeMarker{Epic: "main", Ticket: "03", PrePickHead: "markerhead"}, wantStart: "markerhead"},
		{name: "nothing recorded stamps HEAD only", wantStart: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			d, _, _ := fakes.FakeDeps()
			d.MergeLockStateDir = t.TempDir()
			p := conflictParams(t, t.TempDir())
			label := worktree.ConflictLabel(p.Ticket.Identifier)
			issuesDir := filepath.Join(p.ScratchDir, "main", "issues")
			if err := os.MkdirAll(issuesDir, 0755); err != nil {
				t.Fatalf("MkdirAll: %v", err)
			}
			parent := "---\nid: \"03\"\nstatus: claimed\ntype: implement\n---\n# C\n"
			if err := os.WriteFile(filepath.Join(issuesDir, "03-c.md"), []byte(parent), 0644); err != nil {
				t.Fatalf("WriteFile: %v", err)
			}
			child := "---\nid: \"03a\"\nstatus: claimed\ntype: conflict-resolution\nparent: \"03\"\n---\n# Conflict resolution for 03\n"
			if err := os.WriteFile(filepath.Join(issuesDir, "03a-conflict-resolution.md"), []byte(child), 0644); err != nil {
				t.Fatalf("WriteFile: %v", err)
			}

			lockDir, err := mergeLockDirFor(d, p.ScratchDir, p.FeatureBranch)
			if err != nil {
				t.Fatal(err)
			}
			if err := merge.AcquireMergeLockAt(lockDir, p.FeatureBranch, "03", tc.lockHead); err != nil {
				t.Fatal(err)
			}
			if tc.marker != nil {
				if err := merge.WriteMergeMarker(lockDir, *tc.marker); err != nil {
					t.Fatal(err)
				}
			}

			inProgress := true
			d.CherryPickInProgress = func(string) (bool, error) { return inProgress, nil }
			gotStart, stamped := "", false
			d.StampTrailerRange = func(_, start string, _ ...history.Trailer) error {
				gotStart, stamped = start, true
				return nil
			}
			fakes.HostLiveAgent(t, d, label)
			fakes.FakeRunner(d).SetState(label, agentrunner.StateWorking, "")
			fakes.OnRunnerWait(d, func(s agentrunner.Session) {
				if s.Label == label {
					inProgress = false
					fakes.FakeRunner(d).SetState(label, agentrunner.StateIdle, "")
				}
			})

			if _, _, err := cherryPickWithConflictResolution(d, p, "base", "branch", "", ""); err != nil {
				t.Fatalf("cherryPickWithConflictResolution: %v", err)
			}
			if stamped != (tc.wantStart != "") || gotStart != tc.wantStart {
				t.Errorf("StampTrailerRange start = %q (called %v), want %q", gotStart, stamped, tc.wantStart)
			}
		})
	}
}
