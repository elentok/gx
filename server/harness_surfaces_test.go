package server_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/agentfake"
	"github.com/elentok/gx/testutil/runnerfake"
)

// withFakeAgents puts the fake codex first on PATH, so a session on the codex
// harness runs it. Claude sessions are not run here: these tests read what a
// session was started as from its meta.
func withFakeAgents(t *testing.T) func(*server.Config) {
	t.Helper()
	bin, home := t.TempDir(), t.TempDir()
	if out, err := exec.Command("go", "build", "-o", filepath.Join(bin, "codex"), "github.com/elentok/gx/testutil/agentfake/cmd/fakecodex").CombinedOutput(); err != nil {
		t.Fatalf("build fakecodex: %v\n%s", err, out)
	}
	return servertest.WithEnv(
		"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"HOME="+home,
		"CODEX_HOME="+filepath.Join(home, ".codex"),
		agentfake.ProgramDirEnv+"="+agentfake.NewProgramDir(t),
	)
}

func seedMap(t *testing.T, store, epic string) {
	t.Helper()
	servertest.WriteTicket(t, store, "proj", epic, "01", "first", "")
	if err := os.WriteFile(filepath.Join(store, "proj", epic, "ticket.md"), []byte("---\nkind: map\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// A design session reads the default when it starts and keeps its harness
// after a later switch.
func TestDesignSession_StartsOnTheDefaultAtStartTime(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	seedMap(t, store, "map-a")
	seedMap(t, store, "map-b")
	path := filepath.Join(t.TempDir(), "config.json")
	f := startDesignsIn(t, store, servertest.WithDefaultHarness("codex"), servertest.WithChatHarness("codex"), servertest.WithConfigFile(path), withFakeAgents(t))
	servertest.WriteConfig(t, path, store, map[string]any{"agents": map[string]any{"default-harness": "codex"}})

	first := f.open(t, "proj:map-a", false)
	if first.Refused || first.Session == nil {
		t.Fatalf("open map-a = %+v", first)
	}
	meta := readMeta(t, f.h, first.Session.ID)
	if meta["harness"] != "codex" || meta["first_prompt"] != "$gx-design proj:map-a" {
		t.Fatalf("first design meta = %v, want codex and a $ skill prompt", meta)
	}

	switchDefaultHarness(t, f.h, path, store, "claude")
	second := f.open(t, "proj:map-b", false)
	if second.Refused || second.Session == nil {
		t.Fatalf("open map-b = %+v", second)
	}
	meta = readMeta(t, f.h, second.Session.ID)
	if meta["harness"] != "claude" || meta["first_prompt"] != "/gx-design proj:map-b" {
		t.Errorf("design after the switch meta = %v, want claude and a / skill prompt", meta)
	}
	if got := readMeta(t, f.h, first.Session.ID)["harness"]; got != "codex" {
		t.Errorf("running design session harness = %v after the switch, want codex", got)
	}
}

// The Fix agent (web route) and TUI Investigate start on a Codex default.
func TestFixAgent_StartsOnACodexDefault(t *testing.T) {
	t.Parallel()
	const wantPrompt = "$gx-investigate proj:loop/01 is paused for repair: Merge conflict"
	for _, route := range []string{"tui", "web"} {
		t.Run(route, func(t *testing.T) {
			t.Parallel()
			store := t.TempDir()
			servertest.WriteTicketWith(t, store, "proj", "loop", "01", "broken", servertest.TicketOpts{PauseReason: "merge-conflict"})
			f := startDesignsIn(t, store, servertest.WithDefaultHarness("codex"), servertest.WithChatHarness("codex"), withFakeAgents(t))
			if route == "tui" {
				if res := f.fix(t, "proj:loop/01"); res.Refused {
					t.Fatalf("fix = %+v", res)
				}
			} else {
				body, _ := json.Marshal(server.FixAgentRequest{Address: "proj:loop/01", State: repairToken})
				if code, out := webPostJSON(t, f.base+"/v1/tickets/fix-agent", string(body), f.ck, "", jsonPost(f.base)); code != 200 || reasonOf(t, out) != "" {
					t.Fatalf("web fix = %d %s", code, out)
				}
			}
			live := f.live(t)
			if len(live) != 1 {
				t.Fatalf("%d live sessions, want the Fix agent", len(live))
			}
			meta := readMeta(t, f.h, live[0].ID)
			if meta["harness"] != "codex" || meta["first_prompt"] != wantPrompt {
				t.Errorf("fix agent meta = %v, want codex and %q", meta, wantPrompt)
			}
		})
	}
}

// startSplitRunners starts a server whose Claude launches go to one fake (as the
// configured headless runner) and Codex launches to another (as pty), so a test
// sees which runner a ticket picked.
func startSplitRunners(t *testing.T, store string, extra ...func(*server.Config)) (h *servertest.Harness, claude, pty *runnerfake.Runner, path string) {
	t.Helper()
	servertest.SetProjectRepo(t, store, "proj", testutil.TempRepo(t))
	claude, pty = runnerfake.NewRunner(), runnerfake.NewRunner()
	cl, pt := namedFake{claude, "headless"}, namedFake{pty, "pty"}
	path = filepath.Join(t.TempDir(), "config.json")
	opts := append([]func(*server.Config){
		servertest.WithRunnerFor(cl, func(name, _ string) agentrunner.Runner {
			if name == "pty" {
				return pt
			}
			return cl
		}),
		servertest.WithConfigFile(path),
	}, extra...)
	h = servertest.StartWithStore(t, store, opts...)
	servertest.WriteConfig(t, path, store, nil)
	t.Cleanup(func() {
		for _, r := range []*runnerfake.Runner{claude, pty} {
			if len(r.Prompts("epic-a-iter-01")) > 0 {
				r.SetState("epic-a-iter-01", agentrunner.StateDone, "")
			}
		}
	})
	return h, claude, pty, path
}

// startRetryEnv starts a server on a Codex default whose ticket 01 (of type
// typ) is paused with an iteration error, with ticket 02 as the investigate
// ticket forked off it. earlierClaude gives 01 a Claude run that left a
// transcript. It returns the Investigate retry of 01.
func startRetryEnv(t *testing.T, typ string, earlierClaude bool, extra ...func(*server.Config)) server.QueueResult {
	t.Helper()
	store := t.TempDir()
	servertest.WriteTicketWith(t, store, "proj", "epic-a", "01", "first", servertest.TicketOpts{Type: typ})
	servertest.WriteTicketWith(t, store, "proj", "epic-a", "02", "probe", servertest.TicketOpts{Type: "investigate", Parent: "01"})
	servertest.SetProjectRepo(t, store, "proj", testutil.TempRepo(t))
	h := servertest.StartWithStore(t, store, append([]func(*server.Config){servertest.WithDefaultHarness("codex")}, extra...)...)
	registerTurn(h)
	if earlierClaude {
		servertest.SeedEarlierClaudeRun(t, h, store, "proj", "epic-a", "01", "sess-1", true)
	}
	if err := h.Server.PauseAs("proj:epic-a/01", events.IterationError, "boom"); err != nil {
		t.Fatal(err)
	}
	return retryAs(t, h, investigateAddr)
}

// A person's relaunch of a ticket that ran on Claude, after a switch to
// Codex, picks the Claude runner.
func TestRelaunch_EarlierClaudeRunPicksTheClaudeRunner(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "one", "")
	h, claude, pty, path := startSplitRunners(t, store)
	servertest.SeedEarlierClaudeRun(t, h, store, "proj", "epic-a", "01", "sess-1", true)
	switchDefaultHarness(t, h, path, store, "codex")
	if res, err := h.Client.TicketRelaunch(context.Background(), "proj:epic-a/01", false); err != nil || res.Refused {
		t.Fatalf("relaunch: %+v, %v", res, err)
	}
	if k := waitKind(t, claude, "epic-a-iter-01"); k != "claude" {
		t.Errorf("retry started as %q, want claude", k)
	}
	if k := pty.Kind("epic-a-iter-01"); k != "" {
		t.Errorf("pty runner started the retry as %q, want none", k)
	}
}

// An Investigate retry is held by the usage gate of the harness the resolver
// picks: Claude's for a ticket that ran on Claude, whatever the default.
func TestInvestigateRetry_EarlierClaudeRunChecksClaudeGate(t *testing.T) {
	t.Parallel()
	resets := time.Now().Add(time.Hour).Truncate(time.Second)
	claudeGated := nativerunner.UsageRecord{SeenAt: time.Now(), FiveHour: &nativerunner.UsageWindow{UsedPercentage: 98, ResetsAt: resets.Unix()}}
	codexGated := nativerunner.UsageRecord{SeenAt: time.Now(), SevenDay: &nativerunner.UsageWindow{UsedPercentage: 98, ResetsAt: resets.Unix(), Name: "weekly"}}

	t.Run("claude gate holds it", func(t *testing.T) {
		t.Parallel()
		if res := startRetryEnv(t, "implement", true, withUsageSample(t, claudeGated)); !res.Refused || res.Reason != server.ReasonUsageLimit || !strings.Contains(res.Message, "Claude usage: 98% of the 5-hour limit") {
			t.Errorf("retry = %+v, want a Claude usage-limit refusal", res)
		}
	})
	t.Run("codex gate does not", func(t *testing.T) {
		t.Parallel()
		if res := startRetryEnv(t, "implement", true, withCodexUsageSample(t, codexGated)); res.Refused {
			t.Errorf("retry = %+v, want it let through", res)
		}
	})
}

// A relaunch of a follows-default ticket, one-off and diagnose tickets
// included, with default codex checks Codex's usage gate and picks the PTY
// runner.
func TestRelaunch_FollowsCodexDefaultGateAndRunner(t *testing.T) {
	t.Parallel()
	resets := time.Now().Add(time.Hour).Truncate(time.Second)
	gated := nativerunner.UsageRecord{SeenAt: time.Now(), SevenDay: &nativerunner.UsageWindow{UsedPercentage: 98, ResetsAt: resets.Unix(), Name: "weekly"}}

	for _, typ := range []string{"implement", "diagnose"} {
		setup := func(t *testing.T, extra ...func(*server.Config)) (*servertest.Harness, *runnerfake.Runner, *runnerfake.Runner) {
			store := t.TempDir()
			servertest.WriteTicketWith(t, store, "proj", "epic-a", "01", "one", servertest.TicketOpts{Type: typ})
			h, claude, pty, _ := startSplitRunners(t, store, append([]func(*server.Config){servertest.WithDefaultHarness("codex")}, extra...)...)
			if res, err := h.Client.TicketRelaunch(context.Background(), "proj:epic-a/01", false); err != nil || res.Refused {
				t.Fatalf("relaunch: %+v, %v", res, err)
			}
			return h, claude, pty
		}
		t.Run(typ+" runner", func(t *testing.T) {
			t.Parallel()
			_, claude, pty := setup(t)
			if k := waitKind(t, pty, "epic-a-iter-01"); k != "codex" {
				t.Errorf("relaunch started as %q on pty, want codex", k)
			}
			if k := claude.Kind("epic-a-iter-01"); k != "" {
				t.Errorf("claude runner started the relaunch as %q, want none", k)
			}
		})
		t.Run(typ+" gate", func(t *testing.T) {
			t.Parallel()
			if res := startRetryEnv(t, typ, false, withCodexUsageSample(t, gated)); !res.Refused || res.Reason != server.ReasonUsageLimit || !strings.Contains(res.Message, "Codex usage: 98% of the weekly limit") {
				t.Errorf("retry = %+v, want a Codex usage-limit refusal", res)
			}
		})
	}
}
