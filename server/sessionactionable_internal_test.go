package server

import (
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/elentok/gx/testutil/agentfake"
)

// The fake agent and the server agree on the run-end text and the design
// prompt prefix only by convention. This fails when either side drifts.
func TestRunEndLine_MatchesAgentfakeDesignRunEnd(t *testing.T) {
	got, ok := runEndNext("Thanks, that settles it.\n\n"+agentfake.DesignRunEnd, "gx:epic/03", "claude", false)
	if !ok {
		t.Fatalf("runEndLine does not match agentfake.DesignRunEnd %q", agentfake.DesignRunEnd)
	}
	if got.Reason != agentfake.DesignRunEnd {
		t.Errorf("Reason = %q, want %q", got.Reason, agentfake.DesignRunEnd)
	}
}

func TestDesignContinueAction_UsesAgentfakeDesignPromptPrefix(t *testing.T) {
	action := designContinueAction("gx:epic/03", "claude")
	if len(action) != 2 || !strings.HasPrefix(action[1], agentfake.DesignPromptPrefix) {
		t.Errorf("action = %q, want second entry to start with %q", action, agentfake.DesignPromptPrefix)
	}
}

// A Codex session's run-end line is read from its rollout and raises a
// Continue that resets with /new.
func TestLastAssistantText_ReadsACodexRollout(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "sessions", "2026", "10", "25", "rollout-2026-10-25T10-00-00-conv.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	line := `{"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"text":` + strconv.Quote(agentfake.DesignRunEnd) + `}]}}` + "\n"
	if err := os.WriteFile(path, []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}
	s := &Server{log: slog.New(slog.DiscardHandler), cfg: Config{Env: []string{"CODEX_HOME=" + home}}}
	meta := sessionMeta{Harness: "codex", ConversationID: "conv"}

	text, ok := s.lastAssistantText("s1", meta)
	if !ok {
		t.Fatal("lastAssistantText found no codex rollout")
	}
	a, matched := designActionable(text, "gx:epic/03", meta.Harness, false)
	if !matched || len(a.Action) != 2 || a.Action[0] != "/new" {
		t.Errorf("actionable = %+v, matched %v, want a Continue starting with /new", a, matched)
	}
}

func TestDesignContinueAction_ResetsEachHarnessItsOwnWay(t *testing.T) {
	for agent, want := range map[string]string{"claude": "/clear", "codex": "/new"} {
		if got := designContinueAction("gx:epic/03", agent)[0]; got != want {
			t.Errorf("%s reset = %q, want %q", agent, got, want)
		}
	}
}
