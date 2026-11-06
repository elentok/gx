package designs_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/ralphloop/notify"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/servertest/rigs"
)

const readySeedMark = "design-ready:seeded"

// readyRig is a server whose chat goes to a recording Slack hook.
type readyRig struct {
	f       designsFixture
	hook    *rigs.SlackHook
	gate    string
	started int
}

func startReadyRig(t *testing.T, step, gate string) *readyRig {
	t.Helper()
	hook := rigs.NewSlackHook(t)
	if gate == "" {
		gate = filepath.Join(t.TempDir(), "gate.json")
	}
	f := startDesignsAt(t, step, chatTo(hook, gate))
	return newReadyRig(t, f, hook, gate)
}

func chatTo(hook *rigs.SlackHook, gate string) func(*server.Config) {
	return func(c *server.Config) {
		c.Chat = notify.ServerChatConfig{SlackWebhookURL: hook.URL, GateStatePath: gate, FlushInterval: servertest.FlushInterval}
	}
}

// newReadyRig waits for the first scan to settle the seed mark, so a test
// that moves the step afterwards moves it while the server runs.
func newReadyRig(t *testing.T, f designsFixture, hook *rigs.SlackHook, gate string) *readyRig {
	t.Helper()
	started := hook.WaitStarted(t)
	waitFor(t, func() bool { return gateHasMark(gate, readySeedMark) })
	return &readyRig{f: f, hook: hook, gate: gate, started: started}
}

func gateHasMark(gate, mark string) bool {
	state, err := notify.LoadNotificationState(gate)
	return err == nil && slices.Contains(state.Marks, mark)
}

// quiet waits out the chat flush and returns what the hook got after the start notice.
func (r *readyRig) quiet() []string {
	time.Sleep(5 * servertest.FlushInterval) // negative wait: no event marks a message that must not come
	return r.hook.PostsFrom(r.started)
}

func (r *readyRig) rescan(t *testing.T) {
	t.Helper()
	if err := r.f.h.Client.Tickets.Changed(r.f.ctx, "proj:map"); err != nil {
		t.Fatal(err)
	}
}

func (r *readyRig) setMuted(t *testing.T, muted bool) {
	t.Helper()
	err := notify.UpdateNotificationState(r.gate, func(s *notify.NotificationState) {
		ts := s.Transports["slack"]
		ts.Muted, ts.Reason = muted, "manual-disable"
		s.Transports["slack"] = ts
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestDesignReady_ReachingBranchSendsOneReadyWithEpicCountAndLink(t *testing.T) {
	t.Parallel()
	r := startReadyRig(t, "create-tickets", "")
	r.f.setDesignStepBranch(t)
	posts := r.hook.PostsSince(r.started)
	if len(posts) != 1 {
		t.Fatalf("posts = %q, want one Ready", posts)
	}
	for _, want := range []string{"📦 Ready map-impl", "The design is finished. 1 tickets are ready to queue.", r.f.base + "/#/e/proj:map-impl"} {
		if !strings.Contains(posts[0], want) {
			t.Errorf("post lacks %q:\n%s", want, posts[0])
		}
	}
	// A repeated Branch and a rescan add nothing.
	r.f.setDesignStepBranch(t)
	r.rescan(t)
	if got := r.quiet(); len(got) != 1 {
		t.Errorf("posts = %q, want still one", got)
	}
}

func TestDesignReady_ARescanWithNoStepChangeLeavesTheStateFileUntouched(t *testing.T) {
	t.Parallel()
	r := startReadyRig(t, "create-tickets", "")
	r.f.setDesignStepBranch(t)
	if got := r.hook.PostsSince(r.started); len(got) != 1 {
		t.Fatalf("posts = %q, want one Ready", got)
	}
	old := time.Now().Add(-time.Hour).Truncate(time.Second)
	if err := os.Chtimes(r.gate, old, old); err != nil {
		t.Fatal(err)
	}
	r.rescan(t)
	r.rescan(t)
	r.quiet()
	info, err := os.Stat(r.gate)
	if err != nil {
		t.Fatal(err)
	}
	if !info.ModTime().Equal(old) {
		t.Errorf("state file mtime = %v, want untouched %v", info.ModTime(), old)
	}
}

func TestDesignReady_ARestartNeverResends(t *testing.T) {
	t.Parallel()
	r := startReadyRig(t, "create-tickets", "")
	r.f.setDesignStepBranch(t)
	if got := r.hook.PostsSince(r.started); len(got) != 1 {
		t.Fatalf("posts = %q, want one Ready", got)
	}
	// The same store and gate state under a new server: its designSteps starts empty.
	hook2 := rigs.NewSlackHook(t)
	f2 := startDesignsIn(t, r.f.h.TicketStore, chatTo(hook2, r.gate))
	r2 := newReadyRig(t, f2, hook2, r.gate)
	if got := r2.quiet(); len(got) != 0 {
		t.Errorf("posts after restart = %q, want none", got)
	}
}

func TestDesignReady_ADesignAlreadyAtBranchBeforeTheFirstScanSendsNothing(t *testing.T) {
	t.Parallel()
	r := startReadyRig(t, "branch", "")
	r.rescan(t)
	if got := r.quiet(); len(got) != 0 {
		t.Errorf("posts = %q, want none for a Design that was at Branch already", got)
	}
	if !gateHasMark(r.gate, "design-ready:proj/map-impl") {
		t.Errorf("the seed scan should mark the Design as sent")
	}
}

func TestDesignReady_TheGlobalMuteStopsItAndReenablingLetsALaterOneSend(t *testing.T) {
	t.Parallel()
	r := startReadyRig(t, "create-tickets", "")
	r.setMuted(t, true)
	r.f.setDesignStepBranch(t)
	if got := r.quiet(); countReady(got) != 0 {
		t.Fatalf("posts = %q, want no Ready while muted", got)
	}
	r.setMuted(t, false)
	// A later Design: a second map reaching Branch.
	servertest.WriteTicket(t, r.f.h.TicketStore, "proj", "second", "01", "first", "")
	servertest.WriteTicket(t, r.f.h.TicketStore, "proj", "second-impl", "01", "first", "")
	writeMapStep(t, r.f.h.TicketStore, "second", "branch")
	if err := r.f.h.Client.Tickets.Changed(r.f.ctx, "proj:second"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return countReady(r.hook.PostsFrom(r.started)) == 1 })
	if got := r.hook.PostsFrom(r.started); !strings.Contains(strings.Join(got, "\n"), "Ready second-impl") {
		t.Errorf("posts = %q, want Ready second-impl", got)
	}
}

// writeMapStep makes epic a map at step; rename, not a truncating write, so a
// rescan never reads it half-written.
func writeMapStep(t *testing.T, store, epic, step string) {
	t.Helper()
	path := filepath.Join(store, "proj", epic, "ticket.md")
	if err := os.WriteFile(path+".tmp", []byte("---\nkind: map\ndesign_step: "+step+"\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(path+".tmp", path); err != nil {
		t.Fatal(err)
	}
}

func countReady(posts []string) int {
	n := 0
	for _, p := range posts {
		n += strings.Count(p, "📦 Ready")
	}
	return n
}
