package server_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/agentfake"
)

// refusalFixture is a server with a map to design and a ticket paused for
// repair, so each interactive launch path has something to start.
type refusalFixture struct {
	h     *servertest.Harness
	ctx   context.Context
	path  string
	store string
}

func startRefusal(t *testing.T) refusalFixture {
	t.Helper()
	store := t.TempDir()
	servertest.WriteTicketWith(t, store, "proj", "loop", "01", "broken", servertest.TicketOpts{PauseReason: "merge-conflict"})
	servertest.WriteTicket(t, store, "proj", "map", "01", "first", "")
	servertest.WriteTicket(t, store, "proj", "map-impl", "01", "first", "")
	if err := os.WriteFile(filepath.Join(store, "proj", "map", "ticket.md"), []byte("---\nkind: map\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	servertest.SetProjectRepoNoAutoMerge(t, store, "proj", testutil.TempRepo(t))
	path := filepath.Join(t.TempDir(), "config.json")
	servertest.WriteConfig(t, path, store, nil)
	h := servertest.StartWithStore(t, store, servertest.WithConfigFile(path))
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	return refusalFixture{h: h, ctx: ctx, path: path, store: store}
}

func (f refusalFixture) create(t *testing.T, req server.CreateSessionRequest) server.SessionResult {
	t.Helper()
	req.Project = "proj"
	if req.Dir == "" {
		req.Dir = t.TempDir()
	}
	res, err := f.h.Client.CreateSession(f.ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// Every interactive launch whose harness is not on PATH is refused before it
// starts anything, and the message names what chose the harness.
func TestHarnessMissing_EveryLaunchPathRefuses(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		from string
		run  func(t *testing.T, f refusalFixture) server.SessionResult
	}{
		{"project override", "the project default", func(t *testing.T, f refusalFixture) server.SessionResult {
			setHarness(t, f.h, server.SetDefaultHarnessRequest{Scope: "project", Project: "proj", Harness: "codex"})
			f.h.Programs.Missing("codex")
			return f.create(t, server.CreateSessionRequest{})
		}},
		{"default changed by reload", "the global default", func(t *testing.T, f refusalFixture) server.SessionResult {
			switchDefaultHarness(t, f.h, f.path, f.store, "codex")
			f.h.Programs.Missing("codex")
			return f.create(t, server.CreateSessionRequest{})
		}},
		{"agents new", "the request", func(t *testing.T, f refusalFixture) server.SessionResult {
			f.h.Programs.Missing("codex")
			return f.create(t, server.CreateSessionRequest{Harness: "codex"})
		}},
		{"design step", "the global default", func(t *testing.T, f refusalFixture) server.SessionResult {
			switchDefaultHarness(t, f.h, f.path, f.store, "codex")
			f.h.Programs.Missing("codex")
			res, err := f.h.Client.OpenDesign(f.ctx, server.DesignOpenRequest{Epic: "proj:map"})
			if err != nil {
				t.Fatal(err)
			}
			return res
		}},
		{"fix agent", "the global default", func(t *testing.T, f refusalFixture) server.SessionResult {
			switchDefaultHarness(t, f.h, f.path, f.store, "codex")
			f.h.Programs.Missing("codex")
			res, err := f.h.Client.FixAgent(f.ctx, server.FixAgentRequest{Address: "proj:loop/01"})
			if err != nil {
				t.Fatal(err)
			}
			return res.SessionResult
		}},
		{"investigate", "the global default", func(t *testing.T, f refusalFixture) server.SessionResult {
			switchDefaultHarness(t, f.h, f.path, f.store, "codex")
			f.h.Programs.Missing("codex")
			return f.create(t, server.CreateSessionRequest{Ticket: "proj:loop/01", Prompt: "investigate"})
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := startRefusal(t)
			res := tc.run(t, f)
			if !res.Refused || res.Reason != server.ReasonSessionHarnessMissing {
				t.Fatalf("result = %+v, want a harness-missing refusal", res)
			}
			for _, want := range []string{"codex is not on PATH", tc.from, "Install it or change the default harness"} {
				if !strings.Contains(res.Message, want) {
					t.Errorf("message = %q, want it to contain %q", res.Message, want)
				}
			}
			if list, err := f.h.Client.Sessions(f.ctx); err != nil || len(list) != 0 {
				t.Errorf("sessions = %v, %v; want none", list, err)
			}
		})
	}
}

// Restore of an Exited session whose harness has left PATH is refused and the
// session stays Exited.
func TestHarnessMissing_RestoreRefuses(t *testing.T) {
	t.Parallel()
	codexHome := t.TempDir()
	h, ctx, id, conv, dir := stoppedCodexSession(t, servertest.WithEnv("CODEX_HOME="+codexHome))
	writeCodexRollout(t, codexHome, conv, dir)
	h.Programs.Missing("codex")

	res, err := h.Client.ResumeSession(ctx, id)
	if err != nil || !res.Refused || res.Reason != server.ReasonSessionHarnessMissing {
		t.Fatalf("resume = %+v, %v; want a harness-missing refusal", res, err)
	}
	for _, want := range []string{"codex is not on PATH", "the session's harness", "Install it or change the default harness"} {
		if !strings.Contains(res.Message, want) {
			t.Errorf("message = %q, want it to contain %q", res.Message, want)
		}
	}
	list, err := h.Client.Sessions(ctx)
	if err != nil || len(list) != 1 || list[0].Status.State != server.SessionExited {
		t.Errorf("sessions = %+v, %v; want the one session still Exited", list, err)
	}
}

// A charting handoff whose harness is gone is refused before the old agent is
// stopped: the session keeps its conversation, run and run-end line, and the
// same Continue works once the harness is back.
func TestHarnessMissing_ChartingHandoffRefusesBeforeStoppingTheOldAgent(t *testing.T) {
	t.Parallel()
	f := startHandoff(t, "chart")
	setMapStep(t, f.store, "resolve-tickets")
	before := readMeta(t, f.h, f.id)
	beforeSnap := sessionSnapshotOf(t, f.designsFixture, f.id)
	f.h.Programs.Missing(os.Args[0])

	res, err := f.h.Client.PromptSession(f.ctx, server.SessionPromptRequest{ID: f.id, Continue: true})
	if err != nil || !res.Refused || res.Reason != server.ReasonSessionHarnessMissing {
		t.Fatalf("continue = %+v, %v; want a harness-missing refusal", res, err)
	}
	after := readMeta(t, f.h, f.id)
	for _, k := range []string{"conversation_id", "run", "tier"} {
		if !reflect.DeepEqual(before[k], after[k]) {
			t.Errorf("meta %s = %v, want %v kept", k, after[k], before[k])
		}
	}
	afterSnap := sessionSnapshotOf(t, f.designsFixture, f.id)
	if afterSnap.Status.State == server.SessionExited || !reflect.DeepEqual(beforeSnap.Actionable, afterSnap.Actionable) {
		t.Errorf("session = %+v, want it alive with actionable %+v kept", afterSnap, beforeSnap.Actionable)
	}

	f.h.Programs.Install(os.Args[0])
	f.continueSession(t)
	f.wantRestartedOnWorker(t)
}

// keptSession is a fake session relabelled as label, on a server whose
// default harness can be switched by a config reload.
type keptSession struct {
	h          *servertest.Harness
	ctx        context.Context
	evs        <-chan server.Event
	id, conv   string
	transcript string
	store, cfg string
}

func startKept(t *testing.T, label string) keptSession {
	t.Helper()
	store, claudeCfg, codexHome := t.TempDir(), t.TempDir(), t.TempDir()
	cfg := filepath.Join(t.TempDir(), "config.json")
	servertest.WriteConfig(t, cfg, store, nil)
	fake := string(agentfake.HarnessKind)
	h := servertest.StartWithStore(t, store, servertest.WithTestHolder(), servertest.WithConfigFile(cfg),
		servertest.WithChatHarness(fake), servertest.WithDefaultHarness(fake), servertest.WithChatPollInterval(chatPollTestInterval),
		servertest.WithEnv("CLAUDE_CONFIG_DIR="+claudeCfg, "CODEX_HOME="+codexHome))
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	evs, err := h.Client.Events(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	res, err := h.Client.CreateSession(ctx, server.CreateSessionRequest{Project: "proj", Dir: t.TempDir(), Harness: fake})
	if err != nil || res.Session == nil {
		t.Fatalf("create = %+v, %v", res, err)
	}
	id := res.Session.ID
	meta := readMeta(t, h, id)
	conv, _ := meta["conversation_id"].(string)
	path := filepath.Join(claudeCfg, "projects", "-repo", conv+".jsonl")
	if label == "codex" {
		meta["harness"] = "codex"
		data, err := json.Marshal(meta)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(h.SessionsDir, id, "meta.json"), data, 0o644); err != nil {
			t.Fatal(err)
		}
		path = filepath.Join(codexHome, "sessions", "2026", "01", "01", "rollout-2026-01-01T10-00-00-"+conv+".jsonl")
	}
	waitForHolder(t, filepath.Join(h.SessionsDir, id))
	return keptSession{h: h, ctx: ctx, evs: evs, id: id, conv: conv, transcript: path, store: store, cfg: cfg}
}

func (k keptSession) switchDefault(t *testing.T, to string) {
	t.Helper()
	switchDefaultHarness(t, k.h, k.cfg, k.store, to)
}

// A default switched to codex leaves a running Claude-format session read as
// that session's own kind by the chat, the chat watch, the turn settle and
// failure detection; a Codex session likewise when the default goes back.
func TestHarnessSwitch_RunningSessionKeepsItsKind(t *testing.T) {
	t.Parallel()
	t.Run("chat", func(t *testing.T) {
		t.Parallel()
		k := startKept(t, "claude")
		writeTranscript(t, k.transcript, reply)
		k.switchDefault(t, "codex")
		got, err := k.h.Client.SessionChat(k.ctx, k.id)
		if err != nil || got.Unsupported || got.Unreadable || len(got.Messages) != 1 || got.Messages[0].Text != "hi" {
			t.Errorf("chat = %+v, %v; want the Claude transcript's one message", got, err)
		}
	})
	t.Run("chat watch", func(t *testing.T) {
		t.Parallel()
		k := startKept(t, "claude")
		writeTranscript(t, k.transcript, reply)
		k.switchDefault(t, "codex")
		report(t, k.conv, agentfake.Control{State: "working"})
		waitSessionState(k.ctx, t, k.h, k.id, server.SessionWorking)
		if !chatEventWithin(k.evs, t, k.transcript, k.id, 5*time.Second, true) {
			t.Error("no session-chat-changed event while the transcript grew")
		}
	})
	t.Run("turn settle", func(t *testing.T) {
		t.Parallel()
		k := startKept(t, "claude")
		writeTranscript(t, k.transcript, reply)
		k.switchDefault(t, "codex")
		report(t, k.conv, agentfake.Control{State: "done"})
		waitSessionState(k.ctx, t, k.h, k.id, server.SessionIdle)
		if !chatEventWithin(k.evs, t, k.transcript, k.id, 5*time.Second, true) {
			t.Error("no session-chat-changed event after the transcript grew past the idle edge")
		}
	})
	t.Run("failure detection", func(t *testing.T) {
		t.Parallel()
		k := startKept(t, "claude")
		writeTranscript(t, k.transcript, apiError401)
		k.switchDefault(t, "codex")
		report(t, k.conv, agentfake.Control{State: "done"})
		if got := waitForFailure(k.ctx, t, k.h, k.id, "auth"); got.Label != "Claude login needed" {
			t.Errorf("failure = %+v, want the Claude login failure", got)
		}
	})
	t.Run("codex session chat", func(t *testing.T) {
		t.Parallel()
		k := startKept(t, "codex")
		writeTranscript(t, k.transcript,
			`{"ordinal":0,"type":"session_meta","payload":{"cli_version":"0.166.0"}}`,
			`{"ordinal":1,"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"hi"}]}}`,
			`{"ordinal":2,"type":"event_msg","payload":{"type":"task_complete"}}`,
		)
		k.switchDefault(t, "claude")
		got, err := k.h.Client.SessionChat(k.ctx, k.id)
		if err != nil || got.Unsupported || got.Unreadable || len(got.Messages) != 1 || got.Messages[0].Text != "hi" {
			t.Errorf("chat = %+v, %v; want the rollout's one message", got, err)
		}
	})
	t.Run("codex session chat watch", func(t *testing.T) {
		t.Parallel()
		k := startKept(t, "codex")
		writeTranscript(t, k.transcript, codexMessageLine)
		k.switchDefault(t, "claude")
		report(t, k.conv, agentfake.Control{State: "working"})
		waitSessionState(k.ctx, t, k.h, k.id, server.SessionWorking)
		if !codexEventWithin(k.evs, t, k.transcript, k.id, 5*time.Second) {
			t.Error("no session-chat-changed event while the rollout grew")
		}
	})
}

// The charting handoff of a running design session restarts on the session's
// own harness, whatever the default became since.
func TestHarnessSwitch_ChartingHandoffKeepsTheSessionsKind(t *testing.T) {
	t.Parallel()
	cfg := filepath.Join(t.TempDir(), "config.json")
	f := startHandoff(t, "chart", servertest.WithConfigFile(cfg))
	servertest.WriteConfig(t, cfg, f.store, nil)
	switchDefaultHarness(t, f.h, cfg, f.store, "codex")
	setMapStep(t, f.store, "resolve-tickets")
	f.continueSession(t)
	f.wantRestartedOnWorker(t)
	if got := readMeta(t, f.h, f.id)["harness"]; got != string(agentfake.HarnessKind) {
		t.Errorf("harness after the handoff = %v, want %s", got, agentfake.HarnessKind)
	}
}
