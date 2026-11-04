package servertest

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/elentok/gx/conversations"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil/agentfake"
)

// SessionSnapshotOf is session id's entry in the snapshot.
func SessionSnapshotOf(t *testing.T, f DesignsFixture, id string) wire.SessionInfo {
	t.Helper()
	snap, err := f.H.Client.Snapshot(f.Ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range snap.Sessions {
		if s.ID == id {
			return s
		}
	}
	t.Fatalf("session %s not in the snapshot", id)
	return wire.SessionInfo{}
}

// SetMapStep writes the design step of the map "map" in project proj.
func SetMapStep(t *testing.T, store, step string) {
	t.Helper()
	body := "---\nkind: map\ndesign_step: " + step + "\n---\n"
	if err := os.WriteFile(filepath.Join(store, "proj", "map", "ticket.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// HandoffFixture is a design session launched by gx on a map at the chart
// step, Idle on a fake program with an input box.
type HandoffFixture struct {
	DesignsFixture
	Store, Cfg, ID, Conv string
}

func StartHandoff(t *testing.T, step string, opts ...func(*server.Config)) HandoffFixture {
	t.Helper()
	cfg := t.TempDir()
	store := t.TempDir()
	WriteTicket(t, store, "proj", "map", "01", "first", "")
	WriteTicket(t, store, "proj", "map-impl", "01", "first", "")
	SetMapStep(t, store, step)
	f := StartDesignsIn(t, store, append(opts, WithEnv(agentfake.ProgramInputEnv+"=1", "CLAUDE_CONFIG_DIR="+cfg))...)
	res := f.Open(t, "proj:map", false)
	if res.Refused || res.Session == nil {
		t.Fatalf("open = %+v", res)
	}
	id := res.Session.ID
	conv, _ := ReadMeta(t, f.H, id)["conversation_id"].(string)
	WaitForHolder(t, filepath.Join(f.H.SessionsDir, id))
	Report(t, conv, agentfake.Control{State: "idle"})
	WaitForSession(f.Ctx, t, f.H, f.Evs, id, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionIdle })
	// Idle can precede the launch prompt: a /clear typed meanwhile lands in the
	// same input box and is never submitted alone.
	WaitForSubmitted(t, conv, "\"/gx-design proj:map\"\n")
	return HandoffFixture{DesignsFixture: f, Store: store, Cfg: cfg, ID: id, Conv: conv}
}

func (f HandoffFixture) ContinueSession(t *testing.T) {
	t.Helper()
	res, err := f.H.Client.PromptSession(f.Ctx, wire.SessionPromptRequest{ID: f.ID, Continue: true})
	if err != nil || res.Refused {
		t.Fatalf("continue = %+v, %v", res, err)
	}
}

// NewConversation waits for the session to move off old and returns its meta.
func (f HandoffFixture) NewConversation(t *testing.T, old string) map[string]any {
	t.Helper()
	var meta map[string]any
	WaitFor(t, 10*time.Second, "a new conversation id", func() bool {
		meta = ReadMeta(t, f.H, f.ID)
		return meta["conversation_id"] != old
	})
	return meta
}

func (f HandoffFixture) WantRestartedOnWorker(t *testing.T) {
	t.Helper()
	meta := f.NewConversation(t, f.Conv)
	newConv, _ := meta["conversation_id"].(string)
	if meta["tier"] != "worker" || meta["launch_design"] != true || meta["ticket"] != "proj:map" {
		t.Errorf("meta tier=%v launch_design=%v ticket=%v, want worker, true, proj:map", meta["tier"], meta["launch_design"], meta["ticket"])
	}
	for _, c := range []string{f.Conv, newConv} {
		if _, err := conversations.Read(f.H.ConversationsDir, c); err != nil {
			t.Errorf("conversation %s not recorded: %v", c, err)
		}
	}
	WaitForHolder(t, filepath.Join(f.H.SessionsDir, f.ID))
	Report(t, newConv, agentfake.Control{State: "idle"})
	WaitForSubmitted(t, newConv, "\"/gx-design map\"\n")
	snap := SessionSnapshotOf(t, f.DesignsFixture, f.ID)
	if snap.Status.State == wire.SessionExited {
		t.Errorf("session = %+v, want it alive", snap)
	}
}
