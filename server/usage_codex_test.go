package server

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/testutil/runnerfake"
)

// codexUsageRig is a usageRig with a Codex home the server reads rollouts from.
type codexUsageRig struct {
	*usageRig
	codexHome string
}

func newCodexUsageRig(t *testing.T) *codexUsageRig {
	t.Helper()
	return &codexUsageRig{usageRig: newUsageRig(t), codexHome: t.TempDir()}
}

func (r *codexUsageRig) server() *Server {
	r.t.Helper()
	s, err := New(Config{Runner: runnerfake.NewRunner(), StateDir: r.stateDir, TicketStore: r.store, Env: []string{"HOME=" + r.t.TempDir(), "CODEX_HOME=" + r.codexHome}})
	if err != nil {
		r.t.Fatal(err)
	}
	return s
}

// writeCodexAgent writes a Codex agent dir whose rollout holds one token_count
// line with the given windows ("" for a null one).
func (r *codexUsageRig) writeCodexAgent(agent string, seen time.Time, primary, secondary string) {
	r.t.Helper()
	dir := filepath.Join(nativerunner.AgentsRoot(r.stateDir, "proj"), agent)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		r.t.Fatal(err)
	}
	meta, err := json.Marshal(nativerunner.Meta{Runner: "pty", Harness: "codex", SessionID: "sess-" + agent, Cwd: "/repo/" + agent})
	if err != nil {
		r.t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, nativerunner.MetaFile), meta, 0o644); err != nil {
		r.t.Fatal(err)
	}
	null := func(w string) string {
		if w == "" {
			return "null"
		}
		return w
	}
	rollout := fmt.Sprintf(`{"type":"session_meta","payload":{"id":"sess-%[1]s","cwd":"/repo/%[1]s"}}
{"timestamp":%[2]q,"type":"event_msg","payload":{"type":"token_count","rate_limits":{"primary":%[3]s,"secondary":%[4]s}}}
`, agent, seen.UTC().Format(time.RFC3339), null(primary), null(secondary))
	path := filepath.Join(r.codexHome, "sessions", "2026", "10", "30", "rollout-2026-10-30T10-00-00-sess-"+agent+".jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		r.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(rollout), 0o644); err != nil {
		r.t.Fatal(err)
	}
}

func codexWindowJSON(pct float64, minutes int, resets time.Time) string {
	return fmt.Sprintf(`{"used_percent":%v,"window_minutes":%d,"resets_at":%d}`, pct, minutes, resets.Unix())
}

func TestUsageGate_CodexWindowsGateCodexOnly(t *testing.T) {
	t.Parallel()
	now := time.Unix(1_000_000, 0)
	resets := now.Add(time.Hour)
	for _, tc := range []struct {
		name      string
		pct       float64
		minutes   int
		want      string // "" means not gated
		nullOther bool
	}{
		{"weekly at 98", 98, 10080, "Codex usage: 98% of the weekly limit, resets " + resets.UTC().Format(time.RFC3339), true},
		{"5-hour at 99", 99, 300, "Codex usage: 99% of the 5-hour limit, resets " + resets.UTC().Format(time.RFC3339), true},
		{"other window by its days", 98, 2880, "Codex usage: 98% of the 2-day limit, resets " + resets.UTC().Format(time.RFC3339), true},
		{"97 does not gate", 97, 10080, "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := newCodexUsageRig(t)
			s := r.server()
			r.writeCodexAgent("a", now, codexWindowJSON(tc.pct, tc.minutes, resets), "")
			s.scanUsage()
			got, held := s.usageGate(ralphloop.AgentCodex, now)
			if got != tc.want || held != (tc.want != "") {
				t.Fatalf("codex gate = %q, %v; want %q", got, held, tc.want)
			}
			if v, held := s.usageGate(ralphloop.AgentClaude, now); held {
				t.Errorf("a Codex sample gated Claude: %q", v)
			}
		})
	}
}

// A live Codex agent's meta holds gx's own start id; the rollout is named by the
// id the SessionStart hook saved in codex-session.json.
func TestUsageGate_CodexRolloutIsFoundByTheHookSessionID(t *testing.T) {
	t.Parallel()
	now := time.Unix(1_000_000, 0)
	r := newCodexUsageRig(t)
	s := r.server()
	r.writeCodexAgent("a", now, codexWindowJSON(99, 10080, now.Add(time.Hour)), "")
	dir := filepath.Join(nativerunner.AgentsRoot(r.stateDir, "proj"), "a")
	meta, err := json.Marshal(nativerunner.Meta{Runner: "pty", Harness: "codex", SessionID: "gx-start-id", Cwd: "/repo/a"})
	if err != nil {
		t.Fatal(err)
	}
	hook := `{"session_id":"sess-a","cwd":"/repo/a"}`
	for name, data := range map[string]string{nativerunner.MetaFile: string(meta), harness.CodexSessionFile: hook} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	s.scanUsage()
	if _, held := s.usageGate(ralphloop.AgentCodex, now); !held {
		t.Fatal("codex gate not held: the rollout was not found by the hook's session id")
	}
}

func TestUsageGate_ClaudeSampleDoesNotGateCodex(t *testing.T) {
	t.Parallel()
	r := newCodexUsageRig(t)
	s := r.server()
	now := time.Unix(1_000_000, 0)
	r.write("a", usageRecord(now, 99, now.Add(time.Hour)))
	s.scanUsage()
	if v, held := s.usageGate(ralphloop.AgentCodex, now); held {
		t.Fatalf("a Claude sample gated Codex: %q", v)
	}
	if _, held := s.usageGate(ralphloop.AgentClaude, now); !held {
		t.Fatal("the Claude sample did not gate Claude")
	}
}

func TestUsageSample_CodexGateLiftsAtResetsAtWithNoNewSample(t *testing.T) {
	t.Parallel()
	r := newCodexUsageRig(t)
	s := r.server()
	resets := time.Unix(2_000_000, 0)
	r.writeCodexAgent("a", resets.Add(-2*time.Hour), codexWindowJSON(98, 300, resets), "")
	s.scanUsage()
	if _, held := s.usageGate(ralphloop.AgentCodex, resets.Add(-time.Second)); !held {
		t.Fatal("not gated before resets_at")
	}
	if _, held := s.usageGate(ralphloop.AgentCodex, resets); held {
		t.Fatal("still gated at resets_at")
	}
}

func TestUsageSample_CodexNewestSampleWins(t *testing.T) {
	t.Parallel()
	r := newCodexUsageRig(t)
	s := r.server()
	now := time.Unix(1_000_000, 0)
	resets := now.Add(time.Hour)
	r.writeCodexAgent("old", now.Add(-time.Hour), codexWindowJSON(99, 300, resets), "")
	r.writeCodexAgent("new", now, codexWindowJSON(40, 300, resets), codexWindowJSON(50, 10080, resets))
	s.scanUsage()
	if v, held := s.usageGate(ralphloop.AgentCodex, now); held {
		t.Fatalf("an older 99%% sample gated over a newer 40%%: %q", v)
	}
}

func TestUsageOverride_CodexLiftsOnlyCodexAndSurvivesRestart(t *testing.T) {
	t.Parallel()
	r := newCodexUsageRig(t)
	s := r.server()
	now := time.Unix(1_000_000, 0)
	resets := now.Add(time.Hour)
	r.write("claude", usageRecord(now, 99, resets))
	r.writeCodexAgent("codex", now, codexWindowJSON(99, 10080, resets), "")
	s.scanUsage()

	res, err := s.usageOverride(now, ralphloop.AgentCodex)
	if err != nil || res.Refused || len(res.Windows) != 1 || res.Windows[0] != "weekly" || !res.Until.Equal(resets) {
		t.Fatalf("codex override = %+v, %v", res, err)
	}
	if _, held := s.usageGate(ralphloop.AgentCodex, now); held {
		t.Error("Codex still gated after its override")
	}
	if _, held := s.usageGate(ralphloop.AgentClaude, now); !held {
		t.Error("the Codex override lifted Claude")
	}
	if _, _, ok := openUsageFiles(r.stateDir, usageSampleFileCodex, usageLiftFileCodex).limitWindow(now); ok {
		t.Error("the Codex override was lost across a restart")
	}

	res, err = s.usageOverride(now, ralphloop.AgentCodex)
	if err != nil || !res.Refused || res.Reason != ReasonNotGated {
		t.Errorf("second Codex override = %+v, %v; want refused %s", res, err, ReasonNotGated)
	}
}

func TestUsageOverride_RefusalNamesTheHarness(t *testing.T) {
	t.Parallel()
	s := newCodexUsageRig(t).server()
	now := time.Unix(1_000_000, 0)
	for agent, want := range map[ralphloop.AgentKind]string{
		ralphloop.AgentClaude: "no Claude usage window is at its limit",
		ralphloop.AgentCodex:  "no Codex usage window is at its limit",
	} {
		res, err := s.usageOverride(now, agent)
		if err != nil || !res.Refused || res.Reason != ReasonNotGated || res.Message != want {
			t.Errorf("%s override = %+v, %v; want refused %q", agent, res, err, want)
		}
	}
}
