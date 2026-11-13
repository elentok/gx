package rigs

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/agentfake"
)

// RepairToken is the state token of a ticket paused for repair.
const RepairToken = "0:repair"

// DesignsFixture is a server with the web mux up, for tests that open design
// and fix-agent sessions.
type DesignsFixture struct {
	H    *servertest.Harness
	Ctx  context.Context
	Evs  <-chan wire.Event
	Repo string
	Base string
	Ck   *http.Cookie
}

// StartDesignsIn starts a server on a store the caller has already seeded, in
// project proj whose repo is the trunk checkout.
func StartDesignsIn(t *testing.T, store string, opts ...func(*server.Config)) DesignsFixture {
	t.Helper()
	repo := testutil.TempRepo(t)
	servertest.SetProjectRepoNoAutoMerge(t, store, "proj", repo)
	opts = append([]func(*server.Config){servertest.WithTestHolder(), servertest.WithChatHarness(string(agentfake.HarnessKind)), servertest.WithDefaultHarness(string(agentfake.HarnessKind)),
		func(c *server.Config) { c.WebAddr = "127.0.0.1:0" }}, opts...)
	h := servertest.StartWithStore(t, store, opts...)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	evs, err := h.Client.Events(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	hs, err := h.Client.Handshake(ctx)
	if err != nil || hs.WebURL == "" {
		t.Fatalf("handshake web_url = %q (err %v)", hs.WebURL, err)
	}
	return DesignsFixture{H: h, Ctx: ctx, Evs: evs, Repo: repo, Base: hs.WebURL, Ck: servertest.Login(t, h.StateDir, hs.WebURL)}
}

// Open opens (or creates, when isNew) the design session of epic.
func (f DesignsFixture) Open(t *testing.T, epic string, isNew bool) wire.SessionResult {
	t.Helper()
	res, err := f.H.Client.Sessions.OpenDesign(f.Ctx, wire.DesignOpenRequest{Epic: epic, New: isNew})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// Live lists the sessions that have not exited.
func (f DesignsFixture) Live(t *testing.T) []wire.SessionInfo {
	t.Helper()
	snap, err := f.H.Client.Snapshot(f.Ctx)
	if err != nil {
		t.Fatal(err)
	}
	var out []wire.SessionInfo
	for _, s := range snap.Sessions {
		if s.Status.State != wire.SessionExited {
			out = append(out, s)
		}
	}
	return out
}

// StartDesigns starts a server with a map epic "map" and a plain epic "plain"
// in project proj, whose repo is the trunk checkout, with the web mux up.
func StartDesigns(t *testing.T, opts ...func(*server.Config)) DesignsFixture {
	t.Helper()
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "map", "01", "first", "")
	servertest.WriteTicket(t, store, "proj", "plain", "01", "first", "")
	if err := os.WriteFile(filepath.Join(store, "proj", "map", "ticket.md"), []byte("---\nkind: map\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return StartDesignsIn(t, store, opts...)
}

// PromptFixture is a design session on a fake program with an input box,
// Idle, whose transcript lives under the config dir.
type PromptFixture struct {
	DesignsFixture
	ID, Conv, Transcript string
	// CodexHome is the fake Codex's CODEX_HOME, set by the Codex sessions.
	CodexHome string
}

// StartPromptSession starts a PromptFixture. grace is how long the server
// waits for a typed prompt's turn to start; a test that must act inside that
// turn passes a longer one.
func StartPromptSession(t *testing.T, grace time.Duration) PromptFixture {
	t.Helper()
	return StartPromptSessionWith(t, grace)
}

// StartPromptSessionWith is StartPromptSession with more server options, such
// as a chat destination or the alerts.
func StartPromptSessionWith(t *testing.T, grace time.Duration, opts ...func(*server.Config)) PromptFixture {
	t.Helper()
	return StartPromptSessionAs(t, grace, wire.CreateSessionRequest{Ticket: "proj:map"}, opts...)
}

// StartPromptSessionAs is StartPromptSessionWith with the session made from
// create, such as a Fix agent. Its project, dir and harness are the fixture's.
func StartPromptSessionAs(t *testing.T, grace time.Duration, create wire.CreateSessionRequest, opts ...func(*server.Config)) PromptFixture {
	t.Helper()
	cfg := t.TempDir()
	f := StartDesigns(t, append([]func(*server.Config){func(c *server.Config) { c.PromptGrace = grace }, servertest.WithEnv(agentfake.ProgramInputEnv+"=1", "CLAUDE_CONFIG_DIR="+cfg)}, opts...)...)
	create.Project, create.Dir, create.Harness = "proj", t.TempDir(), string(agentfake.HarnessKind)
	res, err := f.H.Client.Sessions.Create(f.Ctx, create)
	if err != nil || res.Session == nil {
		t.Fatalf("create = %+v, %v", res, err)
	}
	id := res.Session.ID
	conv, _ := servertest.ReadMeta(t, f.H, id)["conversation_id"].(string)
	servertest.WaitForHolder(t, filepath.Join(f.H.SessionsDir, id))
	servertest.Report(t, conv, agentfake.Control{State: "idle"})
	servertest.WaitForSession(f.Ctx, t, f.H, f.Evs, id, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionIdle })
	return PromptFixture{DesignsFixture: f, ID: id, Conv: conv, Transcript: filepath.Join(cfg, "projects", "-repo", conv+".jsonl")}
}

// Prompt sends text to the session as a prompt.
func (f PromptFixture) Prompt(t *testing.T, text string) wire.SessionResult {
	t.Helper()
	res, err := f.H.Client.Sessions.Prompt(f.Ctx, wire.SessionPromptRequest{ID: f.ID, Text: text})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// Fix starts the Fix agent for the ticket at address.
func (f DesignsFixture) Fix(t *testing.T, address string) wire.FixAgentResult {
	t.Helper()
	res, err := f.H.Client.Sessions.FixAgent(f.Ctx, wire.FixAgentRequest{Address: address})
	if err != nil {
		t.Fatal(err)
	}
	return res
}
