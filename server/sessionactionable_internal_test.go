package server

import (
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/elentok/gx/testutil/agentfake"
	"github.com/elentok/gx/transcript"
)

// The fake agent and the server agree on the run-end text and the design
// prompt prefix only by convention. This fails when either side drifts.
func TestRunEndLine_MatchesAgentfakeDesignRunEnd(t *testing.T) {
	t.Parallel()
	got, ok := runEndNext("Thanks, that settles it.\n\n"+agentfake.DesignRunEnd, "gx:epic/03", "claude", false)
	if !ok {
		t.Fatalf("runEndLine does not match agentfake.DesignRunEnd %q", agentfake.DesignRunEnd)
	}
	if got.Reason != agentfake.DesignRunEnd {
		t.Errorf("Reason = %q, want %q", got.Reason, agentfake.DesignRunEnd)
	}
}

func TestDesignContinueAction_UsesAgentfakeDesignPromptPrefix(t *testing.T) {
	t.Parallel()
	action := designContinueAction("gx:epic/03", "claude")
	if len(action) != 2 || !strings.HasPrefix(action[1], agentfake.DesignPromptPrefix) {
		t.Errorf("action = %q, want second entry to start with %q", action, agentfake.DesignPromptPrefix)
	}
}

// A Codex session's run-end line is read from its rollout and raises a
// Continue that resets with /new.
func TestLastAssistantText_ReadsACodexRollout(t *testing.T) {
	t.Parallel()
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

func modelTestServer(t *testing.T, cwd string, lines ...string) *Server {
	t.Helper()
	config := t.TempDir()
	path := filepath.Join(config, "projects", transcript.Slugify(cwd), "conv.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return &Server{log: slog.New(slog.DiscardHandler), cfg: Config{Env: []string{"CLAUDE_CONFIG_DIR=" + config}}}
}

func replyWithModel(model string) string {
	return `{"type":"assistant","message":{"model":"` + model + `","content":[{"type":"text","text":"hi"}]}}`
}

func TestSessionInfoModel_ReportedBeatsSavedAndNewestReplyWins(t *testing.T) {
	t.Parallel()
	s := modelTestServer(t, "/work/proj", replyWithModel("claude-opus-5-5"), replyWithModel("claude-sonnet-5-5"))
	meta := sessionMeta{Harness: "claude", Dir: "/work/proj", ConversationID: "conv", Tier: "worker", Model: "sonnet", Effort: "high"}
	info := s.sessionInfo(meta)
	if info.Model != "claude-sonnet-5-5" || info.Tier != "worker" || info.Effort != "high" {
		t.Errorf("tier/model/effort = %q/%q/%q, want worker/claude-sonnet-5-5/high", info.Tier, info.Model, info.Effort)
	}
}

func TestSessionInfoModel_TierAloneUntilFirstReply(t *testing.T) {
	t.Parallel()
	s := modelTestServer(t, "/work/proj", `{"type":"user","message":{"content":"hi"}}`)
	info := s.sessionInfo(sessionMeta{Harness: "claude", Dir: "/work/proj", ConversationID: "conv", Tier: "light"})
	if info.Tier != "light" || info.Model != "" {
		t.Errorf("tier/model = %q/%q, want light and no model", info.Tier, info.Model)
	}
}

func TestSessionInfoModel_OldMetaShowsTheReportedModelOnceItReplies(t *testing.T) {
	t.Parallel()
	meta := sessionMeta{Harness: "claude", Dir: "/work/proj", ConversationID: "conv"}
	replied := modelTestServer(t, "/work/proj", replyWithModel("claude-sonnet-5-5")).sessionInfo(meta)
	if replied.Model != "claude-sonnet-5-5" || replied.Tier != "" || replied.Effort != "" {
		t.Errorf("tier/model/effort = %q/%q/%q, want no tier or effort and claude-sonnet-5-5", replied.Tier, replied.Model, replied.Effort)
	}
	quiet := modelTestServer(t, "/work/proj", `{"type":"user","message":{"content":"hi"}}`).sessionInfo(meta)
	if quiet.Model != "" {
		t.Errorf("model = %q before the first reply, want empty", quiet.Model)
	}
}

func TestSessionInfoModel_AppendedReplyChangesTheCachedModel(t *testing.T) {
	t.Parallel()
	s := modelTestServer(t, "/work/proj", replyWithModel("claude-opus-5-5"))
	meta := sessionMeta{Harness: "claude", Dir: "/work/proj", ConversationID: "conv"}
	if got := s.sessionInfo(meta).Model; got != "claude-opus-5-5" {
		t.Fatalf("model = %q, want claude-opus-5-5", got)
	}
	path, ok := s.claudeTranscript("s1", meta)
	if !ok {
		t.Fatal("no transcript")
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(replyWithModel("claude-sonnet-5-5") + "\n"); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if got := s.sessionInfo(meta).Model; got != "claude-sonnet-5-5" {
		t.Errorf("model = %q after an appended reply, want claude-sonnet-5-5", got)
	}
}

func TestSessionInfoModel_CodexKeepsSavedValues(t *testing.T) {
	t.Parallel()
	s := modelTestServer(t, "/work/proj", replyWithModel("claude-opus-5-5"))
	info := s.sessionInfo(sessionMeta{Harness: "codex", Dir: "/work/proj", ConversationID: "conv", Tier: "worker", Model: "gpt-5", Effort: "low"})
	if info.Model != "gpt-5" || info.Effort != "low" {
		t.Errorf("model/effort = %q/%q, want gpt-5/low", info.Model, info.Effort)
	}
}

func TestDesignContinueAction_ResetsEachHarnessItsOwnWay(t *testing.T) {
	t.Parallel()
	for agent, want := range map[string]string{"claude": "/clear", "codex": "/new"} {
		if got := designContinueAction("gx:epic/03", agent)[0]; got != want {
			t.Errorf("%s reset = %q, want %q", agent, got, want)
		}
	}
}
