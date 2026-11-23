package merge_test

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
)

// A merge-blocked epic root waits on a person's fix and then merges, so its
// pause is a chat notice only: no push, with or without Chat.
func TestRootPause_MergeBlockedSendsNoPush(t *testing.T) {
	t.Parallel()
	one := map[string]servertest.TicketOpts{"01": {}}
	cases := []struct {
		name string
		kind events.Kind
		f    func(t *testing.T) *schedFixture
	}{
		{"merge-error", events.MergeError, func(t *testing.T) *schedFixture {
			return newSchedFixture(t, one, func(f *schedFixture, p *servertest.Turn, _ string) {
				commitWork(t, p, "01")
				// A stale index lock fails the merge without any local change.
				testutil.WriteFile(t, filepath.Join(f.Repo, ".git"), "index.lock", "")
			})
		}},
		{"dirty-trunk", events.DirtyTrunk, func(t *testing.T) *schedFixture {
			return newSchedFixture(t, one, func(f *schedFixture, p *servertest.Turn, _ string) {
				commitFiles(t, p, "clash.txt")
				testutil.WriteFile(t, f.Repo, "clash.txt", "local")
			})
		}},
		{"trunk-missing", events.TrunkMissing, func(t *testing.T) *schedFixture {
			f := newSchedFixture(t, one, func(f *schedFixture, p *servertest.Turn, _ string) {
				commitWork(t, p, "01")
				f.Git(t, "branch", "-D", "develop")
			})
			f.Git(t, "branch", "develop")
			servertest.SetProjectTrunk(t, f.Store, "proj", f.Repo, "develop")
			return f
		}},
		{"needs-rebase", events.NeedsRebase, func(t *testing.T) *schedFixture {
			return newSchedFixture(t, atMergeCap(one), func(f *schedFixture, p *servertest.Turn, _ string) {
				commitWork(t, p, "01")
				testutil.WriteFile(t, f.Repo, "moved.txt", "moved")
				testutil.CommitAll(t, f.Repo, "main moves on")
			})
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := tc.f(t) // no Chat configured
			ev := f.PauseRoot(t)
			if ev.Kind != string(tc.kind) {
				t.Fatalf("pause kind = %q (%s), want %q", ev.Kind, ev.Message, tc.kind)
			}
			// Negative wait: a wrong push would be sent right after the event.
			servertest.Poll(300*time.Millisecond, func() bool { return len(f.H.Pushes.Sent()) > 0 })
			for _, p := range f.H.Pushes.Sent() {
				if strings.HasPrefix(p.Tag, "ticket:") || strings.HasPrefix(p.Tag, "epic:") {
					t.Errorf("root pause sent a push: %+v", p)
				}
			}
		})
	}
}
