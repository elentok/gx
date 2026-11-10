package sessions_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil/agentfake"
)

var (
	gxBinOnce sync.Once
	gxBinDir  string // dir holding the built "gx"
	gxBinErr  error
)

// gxBin builds the real gx once per package run: the hook command the fake
// runs is `gx agent ask-question`. TestMain calls removeGxBin after the run.
func gxBin(t *testing.T) string {
	t.Helper()
	gxBinOnce.Do(func() {
		if gxBinDir, gxBinErr = os.MkdirTemp("", "gxbin-"); gxBinErr != nil {
			return
		}
		if out, err := exec.Command("go", "build", "-o", filepath.Join(gxBinDir, "gx"), "github.com/elentok/gx").CombinedOutput(); err != nil {
			gxBinErr = &buildError{err, out}
		}
	})
	if gxBinErr != nil {
		t.Fatalf("build gx: %v", gxBinErr)
	}
	return gxBinDir
}

type buildError struct {
	err error
	out []byte
}

func (e *buildError) Error() string { return e.err.Error() + "\n" + string(e.out) }

func removeGxBin() {
	if gxBinDir != "" {
		os.RemoveAll(gxBinDir)
	}
}

// askFixture is a plain Claude session whose "claude" is the TUI fake and whose
// PATH holds the real gx, so the AskUserQuestion hook the launch installs
// reaches the test server.
type askFixture struct {
	h    *servertest.Harness
	ctx  context.Context
	evs  <-chan wire.Event
	id   string
	conv string
	home string
}

// startAskFixture starts the fixture's plain Claude session; link, when set,
// then links it to a Design or Triage flow.
func startAskFixture(t *testing.T, link *wire.LinkSessionRequest, cfg ...func(*server.Config)) askFixture {
	t.Helper()
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "map", "01", "first", "")
	if err := os.WriteFile(filepath.Join(store, "proj", "map", "ticket.md"), []byte("---\nkind: map\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	bin, home := t.TempDir(), t.TempDir()
	script := "#!/bin/sh\nexport " + agentfake.TUIEnv + "=1\nexec '" + os.Args[0] + "' \"$@\"\n"
	if err := os.WriteFile(filepath.Join(bin, "claude"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	// gx finds the server at $XDG_STATE_HOME/gx; the harness keeps its state
	// elsewhere, so link it in once the harness is up.
	xdg, err := os.MkdirTemp("/tmp", "gxq")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(xdg) })
	h := servertest.StartWithStore(t, store,
		servertest.WithTestHolder(),
		servertest.WithAlerts(true, true),
		func(c *server.Config) {
			c.LaunchProofCaps.Grace = 6 * time.Second
			c.LaunchProofCaps.UnknownGrace = 600 * time.Millisecond
			c.LaunchProofCaps.Poll = 20 * time.Millisecond
			c.DialogFallbackDelay = 300 * time.Millisecond
			c.KeysSettleStep = 20 * time.Millisecond
			for _, f := range cfg {
				f(c)
			}
		},
		servertest.WithEnv("PATH="+bin+string(os.PathListSeparator)+gxBin(t)+string(os.PathListSeparator)+os.Getenv("PATH"),
			"HOME="+home, "CLAUDE_CONFIG_DIR="+filepath.Join(home, ".claude"), "XDG_STATE_HOME="+xdg),
	)
	if err := os.Symlink(h.StateDir, filepath.Join(xdg, "gx")); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	evs, err := h.Client.Events(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	res, err := h.Client.Sessions.Create(ctx, wire.CreateSessionRequest{Project: "proj", Dir: dir, Harness: "claude"})
	if err != nil || res.Session == nil {
		t.Fatalf("create = %+v, %v", res, err)
	}
	id := res.Session.ID
	conv, _ := servertest.ReadMeta(t, h, id)["conversation_id"].(string)
	servertest.WaitForSession(ctx, t, h, evs, id, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionIdle })
	if link != nil {
		req := *link
		req.ID = id
		if res, err := h.Client.Sessions.Link(ctx, req); err != nil || res.Refused {
			t.Fatalf("link = %+v, %v", res, err)
		}
	}
	return askFixture{h: h, ctx: ctx, evs: evs, id: id, conv: conv, home: home}
}

// ask makes the fake open an AskUserQuestion of three questions, the second a
// multi-select, through the hook.
func (f askFixture) ask(t *testing.T) (hookOut string) {
	t.Helper()
	hookOut = filepath.Join(t.TempDir(), "hook.out")
	opts := func(labels ...string) []agentfake.AskOption {
		var out []agentfake.AskOption
		for _, l := range labels {
			out = append(out, agentfake.AskOption{Label: l, Description: "about " + l})
		}
		return out
	}
	call := &agentfake.AskCall{ToolUseID: "toolu_ask", OutFile: hookOut, Questions: []agentfake.AskQuestion{
		{Header: "Color", Question: "Which color?", Options: opts("red", "blue")},
		{Header: "Sizes", Question: "Which sizes?", MultiSelect: true, Options: opts("S", "M", "L")},
		{Header: "Ship", Question: "Ship it?", Options: opts("yes", "no")},
	}}
	if err := agentfake.SendTUIControl(f.home, f.conv, agentfake.Control{Action: agentfake.ActionAskQuestion, Ask: call}); err != nil {
		t.Fatal(err)
	}
	// The server's shutdown waits on a request the hook still holds, so a test
	// that fails mid-hold must release it first. Runs before the harness stops.
	t.Cleanup(func() { f.esc() })
	return hookOut
}

// esc presses Esc on the fake, which sends the held hook SIGTERM.
func (f askFixture) esc() error {
	_, err := f.h.Client.Sessions.Keys(f.ctx, wire.SessionKeysRequest{ID: f.id, Key: "esc"})
	return err
}

func (f askFixture) info(t *testing.T) wire.SessionInfo {
	t.Helper()
	list, err := f.h.Client.Sessions.List(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range list {
		if s.ID == f.id {
			return s
		}
	}
	t.Fatalf("session %s not listed", f.id)
	return wire.SessionInfo{}
}

func (f askFixture) cards(t *testing.T) []wire.QuestionCard {
	t.Helper()
	chat, err := f.h.Client.Sessions.Chat(f.ctx, f.id)
	if err != nil {
		t.Fatal(err)
	}
	return chat.Questions
}

func (f askFixture) waitBlocked(t *testing.T) {
	t.Helper()
	servertest.WaitForSession(f.ctx, t, f.h, f.evs, f.id, func(s wire.SessionInfo) bool {
		return s.Status.State == wire.SessionBlocked && s.Status.Kind == "question"
	})
}

func (f askFixture) waitIdle(t *testing.T) {
	t.Helper()
	servertest.WaitForSession(f.ctx, t, f.h, f.evs, f.id, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionIdle })
}

func TestAskHook_HeldQuestionIsADialogCardAndBlocksTheSessionWhileHeld(t *testing.T) {
	t.Parallel()
	f := startAskFixture(t, nil)
	f.ask(t)
	f.waitBlocked(t)

	cards := f.cards(t)
	if len(cards) != 1 || cards[0].Source != wire.CardSourceClaudeDialog || len(cards[0].Questions) != 3 {
		t.Fatalf("cards = %+v, want one claude-dialog card of three questions", cards)
	}
	for i, want := range []struct {
		title, kind string
		options     []string
	}{{"Which color?", "options", []string{"red", "blue", "Other"}}, {"Which sizes?", "multi-select", []string{"S", "M", "L", "Other"}}, {"Ship it?", "options", []string{"yes", "no", "Other"}}} {
		q := cards[0].Questions[i]
		var labels []string
		for _, o := range q.Options {
			labels = append(labels, o.Label)
			// The card ends every question with the typed-answer row.
			if !o.Text && o.Description != "about "+o.Label {
				t.Errorf("question %d option %q description = %q", i, o.Label, o.Description)
			}
		}
		if q.Title != want.title || q.Kind != want.kind || !slices.Equal(labels, want.options) {
			t.Errorf("question %d = %q %s %v, want %+v", i, q.Title, q.Kind, labels, want)
		}
	}
	if info := f.info(t); info.Status.State != wire.SessionBlocked || info.Status.Kind != "question" {
		t.Errorf("listed status = %+v, want blocked question", info.Status)
	} else if info.Actionable.Kind != "question" {
		t.Errorf("actionable = %+v, want kind question", info.Actionable)
	}
	servertest.WaitFor(t, 10*time.Second, "the Blocked: question banner", func() bool {
		for _, b := range f.h.Banners.Sent() {
			if strings.HasPrefix(b.Body, "Blocked: question") {
				return true
			}
		}
		return false
	})
}

func TestAskHook_EscEndsTheHoldAndClearsTheCard(t *testing.T) {
	t.Parallel()
	f := startAskFixture(t, nil)
	f.ask(t)
	f.waitBlocked(t)
	if err := f.esc(); err != nil {
		t.Fatal(err)
	}
	f.waitIdle(t)
	if cards := f.cards(t); len(cards) != 0 {
		t.Errorf("cards after Esc = %+v, want none", cards)
	}
	if info := f.info(t); info.Actionable.Kind == "question" {
		t.Errorf("actionable after Esc = %+v, want it cleared", info.Actionable)
	}
}

func TestAskHook_DesignAndTriageSessionsListNoDialogCard(t *testing.T) {
	t.Parallel()
	for name, link := range map[string]wire.LinkSessionRequest{
		"design": {Ticket: "proj:map"},
		"triage": {Flow: "triage", FlowKey: "proj"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := startAskFixture(t, &link)
			f.ask(t)
			f.waitBlocked(t)
			for _, c := range f.cards(t) {
				if c.Source == wire.CardSourceClaudeDialog {
					t.Errorf("cards = %+v, want no claude-dialog card", f.cards(t))
				}
			}
		})
	}
}
