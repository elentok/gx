package merge_test

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/notifykind"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
)

// A merge-blocked epic root is an Epic paused notification: it pushes to the
// epic page only when the Push list has epic-paused. No pause kind is added.
func TestRootPause_MergeBlockedPushesOnlyWithEpicPausedChecked(t *testing.T) {
	t.Parallel()
	one := map[string]servertest.TicketOpts{"01": {}}
	cases := []struct {
		name string
		kind events.Kind
		f    func(t *testing.T, opts ...func(*server.Config)) *schedFixture
	}{
		{"merge-error", events.MergeError, func(t *testing.T, opts ...func(*server.Config)) *schedFixture {
			return newSchedFixture(t, one, func(f *schedFixture, p *servertest.Turn, _ string) {
				commitWork(t, p, "01")
				// A stale index lock fails the merge without any local change.
				testutil.WriteFile(t, filepath.Join(f.Repo, ".git"), "index.lock", "")
			}, opts...)
		}},
		{"dirty-trunk", events.DirtyTrunk, func(t *testing.T, opts ...func(*server.Config)) *schedFixture {
			return newSchedFixture(t, one, func(f *schedFixture, p *servertest.Turn, _ string) {
				commitFiles(t, p, "clash.txt")
				testutil.WriteFile(t, f.Repo, "clash.txt", "local")
			}, opts...)
		}},
		{"trunk-missing", events.TrunkMissing, func(t *testing.T, opts ...func(*server.Config)) *schedFixture {
			f := newSchedFixture(t, one, func(f *schedFixture, p *servertest.Turn, _ string) {
				commitWork(t, p, "01")
				f.Git(t, "branch", "-D", "develop")
			}, opts...)
			f.Git(t, "branch", "develop")
			servertest.SetProjectTrunk(t, f.Store, "proj", f.Repo, "develop")
			return f
		}},
		{"needs-rebase", events.NeedsRebase, func(t *testing.T, opts ...func(*server.Config)) *schedFixture {
			return newSchedFixture(t, atMergeCap(one), func(f *schedFixture, p *servertest.Turn, _ string) {
				commitWork(t, p, "01")
				testutil.WriteFile(t, f.Repo, "moved.txt", "moved")
				testutil.CommitAll(t, f.Repo, "main moves on")
			}, opts...)
		}},
	}
	epicPushes := func(f *schedFixture) []servertest.Push {
		var out []servertest.Push
		for _, p := range f.H.Pushes.Sent() {
			if strings.HasPrefix(p.Tag, "ticket:") || strings.HasPrefix(p.Tag, "epic:") {
				out = append(out, p)
			}
		}
		return out
	}
	for _, tc := range cases {
		t.Run(tc.name+"/unchecked", func(t *testing.T) {
			t.Parallel()
			f := tc.f(t, servertest.WithChannel("push", true, []string{notifykind.PausedTicket}))
			ev := f.PauseRoot(t)
			if ev.Kind != string(tc.kind) {
				t.Fatalf("pause kind = %q (%s), want %q", ev.Kind, ev.Message, tc.kind)
			}
			// Negative wait: a wrong push would be sent right after the event.
			servertest.Poll(300*time.Millisecond, func() bool { return len(epicPushes(f)) > 0 })
			if got := epicPushes(f); len(got) > 0 {
				t.Errorf("root pause sent a push without epic-paused: %+v", got)
			}
		})
		t.Run(tc.name+"/checked", func(t *testing.T) {
			t.Parallel()
			f := tc.f(t, servertest.WithChannel("push", true, []string{notifykind.EpicPaused}))
			ev := f.PauseRoot(t)
			if ev.Kind != string(tc.kind) {
				t.Fatalf("pause kind = %q (%s), want %q", ev.Kind, ev.Message, tc.kind)
			}
			if !servertest.Poll(5*time.Second, func() bool { return len(epicPushes(f)) > 0 }) {
				t.Fatal("no push for the root pause")
			}
			got := epicPushes(f)
			id := "proj:" + servertest.SchedEpic
			if len(got) != 1 || got[0].Tag != "epic:"+id || got[0].Path != "/e/"+id || got[0].Urgent {
				t.Errorf("pushes = %+v, want one normal push tagged epic:%s", got, id)
			}
		})
	}
}
