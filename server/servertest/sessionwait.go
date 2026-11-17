package servertest

import (
	"cmp"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/nativerunner/holder"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil/agentfake"
)

// Report sends the fake program of conversation conv a report control.
func Report(t *testing.T, conv string, c agentfake.Control) {
	t.Helper()
	c.Action = agentfake.ActionReport
	if err := agentfake.SendProgramControl(conv, c); err != nil {
		t.Fatal(err)
	}
}

// WaitForHolder dials the holder in dir until it answers.
func WaitForHolder(t *testing.T, dir string) *holder.HolderClient {
	t.Helper()
	var c *holder.HolderClient
	var err error
	if !Poll(30*time.Second, func() bool {
		c, err = holder.DialHolder(dir)
		return err == nil
	}) {
		t.Fatalf("no holder in %s: %v", dir, err)
	}
	return c
}

// SessionState describes what a timed-out wait on a session can see: the live
// iteration list and, for each session dir (only id's when id is not empty),
// its meta.json and whether the holder answers. For WaitState's read.
func (h *Harness) SessionState(id string) string {
	var b strings.Builder
	if its, err := h.Client.Reads.Iterations(context.Background()); err != nil {
		fmt.Fprintf(&b, "iterations: error %v\n", err)
	} else {
		fmt.Fprintf(&b, "iterations (%d): %+v\n", len(its), its)
	}
	entries, _ := os.ReadDir(h.SessionsDir)
	for _, e := range entries {
		if !e.IsDir() || (id != "" && e.Name() != id) {
			continue
		}
		dir := filepath.Join(h.SessionsDir, e.Name())
		meta, err := os.ReadFile(filepath.Join(dir, "meta.json"))
		fmt.Fprintf(&b, "session %s meta (err %v): %s\n", e.Name(), err, strings.TrimSpace(string(meta)))
		if c, err := holder.DialHolder(dir); err != nil {
			fmt.Fprintf(&b, "session %s holder: %v\n", e.Name(), err)
		} else {
			fmt.Fprintf(&b, "session %s holder: answers (holder pid %d, child pid %d)\n", e.Name(), c.HolderPID(), c.ChildPID())
			if w, err := c.Watched(); err != nil {
				fmt.Fprintf(&b, "session %s watch: error %v\n", e.Name(), err)
			} else {
				fmt.Fprintf(&b, "session %s watch: %+v\n", e.Name(), w)
			}
		}
	}
	return b.String()
}

// WaitForSession waits until the snapshot's session id satisfies ok. It reads
// the snapshot before each wait: a change that landed together with the one
// the caller just waited for sends no further event.
func WaitForSession(ctx context.Context, t *testing.T, h *Harness, evs <-chan wire.Event, id string, ok func(wire.SessionInfo) bool) wire.SessionInfo {
	t.Helper()
	s, failure := waitForSession(ctx, h, evs, id, ok)
	if failure != "" {
		t.Fatal(failure)
	}
	return s
}

// waitForSession is WaitForSession returning the failure text instead of
// failing: the text names the last entry read for the session and the server
// log tail, which is all a timeout leaves of a cause nobody has found.
func waitForSession(ctx context.Context, h *Harness, evs <-chan wire.Event, id string, ok func(wire.SessionInfo) bool) (wire.SessionInfo, string) {
	last := "(no entry for the session in the snapshot)"
	giveUp := func(why string) (wire.SessionInfo, string) {
		return wire.SessionInfo{}, fmt.Sprintf("%s\nlast session entry read: %s\nserver log tail:\n%s", why, last, h.LogTail(30))
	}
	for {
		snap, err := h.Client.Snapshot(ctx)
		if err != nil {
			return giveUp(fmt.Sprintf("snapshot for session %q: %v", id, err))
		}
		for _, s := range snap.Sessions {
			if s.ID != id {
				continue
			}
			last = fmt.Sprintf("status=%+v actionable=%+v auto-advance=%+v", s.Status, s.Actionable, s.AutoAdvance)
			if ok(s) {
				return s, ""
			}
		}
		for typed := false; !typed; {
			select {
			case ev, open := <-evs:
				if !open {
					return giveUp(fmt.Sprintf("event stream ended before %s %q", wire.EventSessionChanged, id))
				}
				typed = ev.Type == wire.EventSessionChanged && ev.Address == id
			case <-ctx.Done():
				return giveUp(fmt.Sprintf("no %s event for %q: %v", wire.EventSessionChanged, id, ctx.Err()))
			}
		}
	}
}

// Submitted is what the fake program of conversation conv has received.
func Submitted(t *testing.T, conv string) string {
	t.Helper()
	data, _ := os.ReadFile(agentfake.ProgramSubmittedPath(conv))
	return string(data)
}

// WaitForSubmitted waits until Submitted equals want.
func WaitForSubmitted(t *testing.T, conv, want string) {
	t.Helper()
	if !Poll(10*time.Second, func() bool { return Submitted(t, conv) == want }) {
		t.Fatalf("submitted = %q, want %q", Submitted(t, conv), want)
	}
}

// StartFakeSession creates a fake-harness session and waits for its first
// report, returning its id and conversation id.
func StartFakeSession(t *testing.T, opts ...func(*server.Config)) (*Harness, context.Context, <-chan wire.Event, string, string) {
	t.Helper()
	return StartFakeSessionWith(t, wire.CreateSessionRequest{}, opts...)
}

// StartFakeSessionWith is StartFakeSession for req in project proj.
func StartFakeSessionWith(t *testing.T, req wire.CreateSessionRequest, opts ...func(*server.Config)) (*Harness, context.Context, <-chan wire.Event, string, string) {
	t.Helper()
	h := StartWithStore(t, t.TempDir(), append([]func(*server.Config){WithTestHolder()}, opts...)...)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	evs, err := h.Client.Events(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	req.Project, req.Dir, req.Harness = "proj", t.TempDir(), cmp.Or(req.Harness, string(agentfake.HarnessKind))
	res, err := h.Client.Sessions.Create(ctx, req)
	if err != nil || res.Session == nil {
		t.Fatalf("create = %+v, %v", res, err)
	}
	id := res.Session.ID
	conv, _ := ReadMeta(t, h, id)["conversation_id"].(string)
	return h, ctx, evs, id, conv
}

// StopFakeSession stops session id and waits for it to be Exited.
func StopFakeSession(ctx context.Context, t *testing.T, h *Harness, evs <-chan wire.Event, id string) {
	t.Helper()
	WaitForHolder(t, filepath.Join(h.SessionsDir, id))
	if res, err := h.Client.Sessions.Stop(ctx, id); err != nil || res.Refused {
		t.Fatalf("stop = %+v, %v", res, err)
	}
	WaitForSession(ctx, t, h, evs, id, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionExited })
}
