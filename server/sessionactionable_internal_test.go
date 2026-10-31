package server

import (
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/grillround"
	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/testutil/agentfake"
	"github.com/elentok/gx/tickets"
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

// A Codex turn with several agent messages keeps a round raised in an earlier one.
func TestLastAssistantText_CodexTurnKeepsARoundFromAnEarlierMessage(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	path := filepath.Join(home, "sessions", "2026", "10", "25", "rollout-2026-10-25T10-00-00-conv.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	msg := func(role, text string) string {
		return `{"type":"response_item","payload":{"type":"message","role":"` + role + `","content":[{"text":` + strconv.Quote(text) + `}]}}` + "\n"
	}
	rollout := msg("assistant", "old turn") + msg("user", "go") + msg("assistant", "❓ **Q1** - **Alpha**: one two") + msg("assistant", "Waiting for you.")
	if err := os.WriteFile(path, []byte(rollout), 0o644); err != nil {
		t.Fatal(err)
	}
	s := &Server{log: slog.New(slog.DiscardHandler), cfg: Config{Env: []string{"CODEX_HOME=" + home}}}

	text, ok := s.lastAssistantText("s1", sessionMeta{Harness: "codex", ConversationID: "conv"})
	if !ok || !grillround.HasMarker(text) || strings.Contains(text, "old turn") {
		t.Errorf("text = %q, ok %v; want the last turn's round", text, ok)
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
	if info.Model != "gpt-5" || info.Effort != "low" || info.ModelSwitched {
		t.Errorf("model/effort/switched = %q/%q/%v, want gpt-5/low/false", info.Model, info.Effort, info.ModelSwitched)
	}
}

func TestSessionInfoModel_CodexReadsTheNewestRolloutTurn(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	dir := filepath.Join(home, "sessions", "2026", "10", "30")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	rollout := `{"type":"turn_context","payload":{"model":"gpt-6.1-sol","effort":"low"}}
{"type":"turn_context","payload":{"model":"gpt-6.1-terra","effort":"high"}}
`
	if err := os.WriteFile(filepath.Join(dir, "rollout-conv.jsonl"), []byte(rollout), 0o644); err != nil {
		t.Fatal(err)
	}
	s := &Server{log: slog.New(slog.DiscardHandler), cfg: Config{Env: []string{"CODEX_HOME=" + home}}}
	info := s.sessionInfo(sessionMeta{Harness: "codex", ConversationID: "conv", Tier: "worker", Model: "gpt-6.1-sol", Effort: "low"})
	if info.Model != "gpt-6.1-terra" || info.Effort != "high" || !info.ModelSwitched {
		t.Errorf("model/effort/switched = %q/%q/%v, want gpt-6.1-terra/high/true", info.Model, info.Effort, info.ModelSwitched)
	}
}

func TestSessionInfoModel_ModelSwitchedNote(t *testing.T) {
	t.Parallel()
	meta := func(model string) sessionMeta {
		return sessionMeta{Harness: "claude", Dir: "/work/proj", ConversationID: "conv", Tier: "worker", Model: model}
	}
	for name, tc := range map[string]struct {
		launched string
		lines    []string
		want     bool
	}{
		"reply differs from the alias": {"sonnet", []string{replyWithModel("claude-opus-5-5")}, true},
		"reply contains the alias":     {"sonnet", []string{replyWithModel("claude-Sonnet-5-5")}, false},
		"no explicit launch model":     {"", []string{replyWithModel("claude-opus-5-5")}, false},
		"before the first reply":       {"sonnet", []string{`{"type":"user","message":{"content":"hi"}}`}, false},
	} {
		s := modelTestServer(t, "/work/proj", tc.lines...)
		if got := s.sessionInfo(meta(tc.launched)).ModelSwitched; got != tc.want {
			t.Errorf("%s: ModelSwitched = %v, want %v", name, got, tc.want)
		}
	}
}

func TestDesignContinueAction_ResetsEachHarnessItsOwnWay(t *testing.T) {
	t.Parallel()
	for agent, want := range map[string][]string{
		"claude": {"/clear", "/gx-design epic"},
		"codex":  {"/new", "$gx-design epic"},
		"fake":   {"/clear", "/gx-design epic"},
	} {
		if got := designContinueAction("gx:epic/03", agent); !slices.Equal(got, want) {
			t.Errorf("%s action = %q, want %q", agent, got, want)
		}
	}
}

// The run-end line raises the session's own harness command.
func TestChatNext_NamesTheSessionsHarnessCommand(t *testing.T) {
	t.Parallel()
	msgs := []transcript.ChatMessage{{Role: transcript.ChatRoleAgent, Kind: transcript.ChatKindText, Text: agentfake.DesignRunEnd}}
	for kind, want := range map[agentrunner.Kind][]string{
		harness.Claude: {"/clear", "/gx-design epic"},
		harness.Codex:  {"/new", "$gx-design epic"},
	} {
		next := chatNext(msgs, kind, "gx:epic/03", false)
		if next == nil || !slices.Equal(next.Action.Action, want) {
			t.Errorf("%s next = %+v, want action %q", kind, next, want)
		}
	}
}

func TestFixAgentPrompt_TypesTheSessionsPrefix(t *testing.T) {
	t.Parallel()
	var tk tickets.Ticket
	for agent, want := range map[string]string{
		"claude": "/gx-investigate gx:e/01 is in error",
		"codex":  "$gx-investigate gx:e/01 is in error",
	} {
		if got := fixAgentPrompt(agent, "gx:e/01", tk); got != want {
			t.Errorf("%s prompt = %q, want %q", agent, got, want)
		}
	}
}
