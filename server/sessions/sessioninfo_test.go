package sessions_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/ralphloop/agent"
	"github.com/elentok/gx/server/sessions"
	"github.com/elentok/gx/server/sessions/chat"
	sessprompt "github.com/elentok/gx/server/sessions/prompt"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil/agentfake"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/ticketlog"
	"github.com/elentok/gx/transcript"
)

// The fake agent and the server agree on the run-end text and the design
// prompt prefix only by convention. This fails when either side drifts.
func TestRunEndLine_MatchesAgentfakeDesignRunEnd(t *testing.T) {
	t.Parallel()
	got, ok := chat.RunEndNext("Thanks, that settles it.\n\n"+agentfake.DesignRunEnd, designRef, "claude", false)
	if !ok {
		t.Fatalf("runEndLine does not match agentfake.DesignRunEnd %q", agentfake.DesignRunEnd)
	}
	if got.Reason != agentfake.DesignRunEnd {
		t.Errorf("Reason = %q, want %q", got.Reason, agentfake.DesignRunEnd)
	}
}

var designRef = chat.FlowRef{Flow: tickets.FlowDesign, Key: "gx:epic/03"}

func TestContinueAction_UsesAgentfakeDesignPromptPrefix(t *testing.T) {
	t.Parallel()
	action := chat.ContinueAction(designRef, "claude")
	if len(action) != 2 || !strings.HasPrefix(action[1], agentfake.DesignPromptPrefix) {
		t.Errorf("action = %q, want second entry to start with %q", action, agentfake.DesignPromptPrefix)
	}
}

// infoSource is the chat Source of a bare Manager: only the Codex home is real.
type infoSource struct{ codexHome string }

func (s infoSource) SessionDir(string) (string, chat.Meta, wire.SessionResult, error) {
	return "", chat.Meta{}, wire.SessionResult{}, errors.New("unused")
}
func (s infoSource) ReadableSession(string) (string, chat.Meta, wire.SessionResult, error) {
	return "", chat.Meta{}, wire.SessionResult{}, errors.New("unused")
}
func (s infoSource) ReadMeta(string) (chat.Meta, error) { return chat.Meta{}, errors.New("unused") }
func (s infoSource) CheckProjectDir(string, string) (*wire.SessionResult, error) {
	return nil, errors.New("unused")
}
func (s infoSource) HeldAsks(string) []wire.AskHookRequest { return nil }
func (s infoSource) AskReleased(string, string) bool       { return false }
func (s infoSource) StepFinal(chat.FlowRef) bool           { return false }
func (s infoSource) NoteFailure(string)                    {}
func (s infoSource) NoteCards(string)                      {}
func (s infoSource) Stopped() <-chan struct{}              { return nil }
func (s infoSource) HasChatReader(string) bool             { return false }
func (s infoSource) CodexHome() (string, error)            { return s.codexHome, nil }
func (s infoSource) Publish(string, string)                {}
func (s infoSource) TicketAsk(string) (ticketlog.Question, string, bool) {
	return ticketlog.Question{}, "", false
}

// infoManager is a Manager with the session chat and prompt that New builds.
func infoManager(env []string, codexHome string) *sessions.Manager {
	log := slog.New(slog.DiscardHandler)
	return sessions.NewManager(sessions.ManagerDeps{
		Chat:     chat.New(chat.Deps{Config: chat.Config{Env: env}, Source: infoSource{codexHome}, Log: log}),
		Prompter: sessprompt.New(sessprompt.Deps{Log: log}),
	})
}

func modelTestServer(t *testing.T, cwd string, lines ...string) *sessions.Manager {
	t.Helper()
	mg, _ := modelTestManager(t, cwd, lines...)
	return mg
}

// modelTestManager also returns the path of the transcript it wrote.
func modelTestManager(t *testing.T, cwd string, lines ...string) (*sessions.Manager, string) {
	t.Helper()
	config := t.TempDir()
	path := filepath.Join(config, "projects", transcript.Slugify(cwd), "conv.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return infoManager([]string{"CLAUDE_CONFIG_DIR=" + config}, ""), path
}

func replyWithModel(model string) string {
	return `{"type":"assistant","message":{"model":"` + model + `","content":[{"type":"text","text":"hi"}]}}`
}

func replyWithUsage(model string, tokens int) string {
	return fmt.Sprintf(`{"type":"assistant","message":{"model":%q,"usage":{"input_tokens":%d}}}`, model, tokens)
}

const syntheticReply = `{"type":"assistant","message":{"model":"<synthetic>","usage":{"input_tokens":0,"output_tokens":0}}}`

var contextMeta = sessions.Meta{Harness: "claude", Dir: "/work/proj", ConversationID: "conv"}

func TestSessionInfoContext_ClaudeReplyGivesCountAndLevel(t *testing.T) {
	t.Parallel()
	info := modelTestServer(t, "/work/proj", replyWithUsage("claude-opus-5-5", 80000)).SessionInfo(contextMeta)
	if info.ContextTokens == nil || *info.ContextTokens != 80000 || info.ContextLevel != agent.LevelYellow {
		t.Errorf("context = %v/%q, want 80000/yellow", info.ContextTokens, info.ContextLevel)
	}
}

func TestSessionInfoUsage_ClaudeGivesContextSplitTotalsCostAndCompactions(t *testing.T) {
	t.Parallel()
	first := `{"type":"assistant","timestamp":"2026-01-01T00:00:00Z","message":{"model":"claude-sonnet-5-5","usage":{"input_tokens":10,"output_tokens":5,"cache_read_input_tokens":20,"cache_creation_input_tokens":30}}}`
	boundary := `{"type":"system","subtype":"compact_boundary","compactMetadata":{"trigger":"manual"}}`
	last := `{"type":"assistant","timestamp":"2026-01-01T00:01:00Z","message":{"model":"claude-sonnet-5-5","usage":{"input_tokens":40,"output_tokens":6,"cache_read_input_tokens":50,"cache_creation_input_tokens":60}}}`
	info := modelTestServer(t, "/work/proj", first, boundary, last).SessionInfo(contextMeta)
	if info.Usage == nil {
		t.Fatal("usage = nil")
	}
	if got := *info.Usage; got.InputTokens != 40 || got.CacheReadTokens != 50 || got.CacheCreationTokens != 60 || got.TotalTokens != 221 || got.EstimatedCost == nil || *got.EstimatedCost <= 0 || got.Compactions != 1 {
		t.Errorf("usage = %+v, want the last context split, 221 total tokens, a cost and one compaction", got)
	}
}

func TestSessionInfoContext_NoReadingCarriesNeither(t *testing.T) {
	t.Parallel()
	cases := map[string]sessions.Meta{
		"no assistant message": contextMeta,
		"codex":                {Harness: "codex", Dir: "/work/proj", ConversationID: "conv"},
		"missing transcript":   {Harness: "claude", Dir: "/work/proj", ConversationID: "other"},
	}
	for name, meta := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			info := modelTestServer(t, "/work/proj", `{"type":"user","message":{"content":"hi"}}`).SessionInfo(meta)
			if info.ContextTokens != nil || info.ContextLevel != "" {
				t.Errorf("context = %v/%q, want none", info.ContextTokens, info.ContextLevel)
			}
			b, _ := json.Marshal(info)
			if strings.Contains(string(b), "context_") {
				t.Errorf("json %s carries a context field", b)
			}
		})
	}
}

func TestSessionInfoContext_CompactKeepsTheEarlierCount(t *testing.T) {
	t.Parallel()
	boundary := `{"type":"system","subtype":"compact_boundary","compactMetadata":{"trigger":"manual"}}`
	info := modelTestServer(t, "/work/proj", replyWithUsage("claude-opus-5-5", 120000), boundary).SessionInfo(contextMeta)
	if info.ContextTokens == nil || *info.ContextTokens != 120000 || info.ContextLevel != agent.LevelOrange {
		t.Errorf("context = %v/%q, want 120000/orange", info.ContextTokens, info.ContextLevel)
	}
}

func TestSessionInfoContext_SyntheticZeroLineKeepsTheRealCount(t *testing.T) {
	t.Parallel()
	info := modelTestServer(t, "/work/proj", replyWithUsage("claude-opus-5-5", 50000), syntheticReply).SessionInfo(contextMeta)
	if info.ContextTokens == nil || *info.ContextTokens != 50000 || info.ContextLevel != agent.LevelGreen {
		t.Errorf("context = %v/%q, want 50000/green", info.ContextTokens, info.ContextLevel)
	}
}

func TestSessionInfoContext_AppendedReplyChangesTheCachedCount(t *testing.T) {
	t.Parallel()
	s, path := modelTestManager(t, "/work/proj", replyWithUsage("claude-opus-5-5", 50000))
	if got := s.SessionInfo(contextMeta).ContextTokens; got == nil || *got != 50000 {
		t.Fatalf("tokens = %v, want 50000", got)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(replyWithUsage("claude-opus-5-5", 140000) + "\n"); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	info := s.SessionInfo(contextMeta)
	if info.ContextTokens == nil || *info.ContextTokens != 140000 || info.ContextLevel != agent.LevelRed {
		t.Errorf("context = %v/%q after an appended reply, want 140000/red", info.ContextTokens, info.ContextLevel)
	}
}

func TestSessionInfoModel_ReportedBeatsSavedAndNewestReplyWins(t *testing.T) {
	t.Parallel()
	s := modelTestServer(t, "/work/proj", replyWithModel("claude-opus-5-5"), replyWithModel("claude-sonnet-5-5"))
	meta := sessions.Meta{Harness: "claude", Dir: "/work/proj", ConversationID: "conv", Tier: "worker", Model: "sonnet", Effort: "high"}
	info := s.SessionInfo(meta)
	if info.Model != "claude-sonnet-5-5" || info.Tier != "worker" || info.Effort != "high" {
		t.Errorf("tier/model/effort = %q/%q/%q, want worker/claude-sonnet-5-5/high", info.Tier, info.Model, info.Effort)
	}
}

func TestSessionInfoModel_TierAloneUntilFirstReply(t *testing.T) {
	t.Parallel()
	s := modelTestServer(t, "/work/proj", `{"type":"user","message":{"content":"hi"}}`)
	info := s.SessionInfo(sessions.Meta{Harness: "claude", Dir: "/work/proj", ConversationID: "conv", Tier: "light"})
	if info.Tier != "light" || info.Model != "" {
		t.Errorf("tier/model = %q/%q, want light and no model", info.Tier, info.Model)
	}
}

func TestSessionInfoModel_OldMetaShowsTheReportedModelOnceItReplies(t *testing.T) {
	t.Parallel()
	meta := sessions.Meta{Harness: "claude", Dir: "/work/proj", ConversationID: "conv"}
	replied := modelTestServer(t, "/work/proj", replyWithModel("claude-sonnet-5-5")).SessionInfo(meta)
	if replied.Model != "claude-sonnet-5-5" || replied.Tier != "" || replied.Effort != "" {
		t.Errorf("tier/model/effort = %q/%q/%q, want no tier or effort and claude-sonnet-5-5", replied.Tier, replied.Model, replied.Effort)
	}
	quiet := modelTestServer(t, "/work/proj", `{"type":"user","message":{"content":"hi"}}`).SessionInfo(meta)
	if quiet.Model != "" {
		t.Errorf("model = %q before the first reply, want empty", quiet.Model)
	}
}

func TestSessionInfoModel_AppendedReplyChangesTheCachedModel(t *testing.T) {
	t.Parallel()
	s, path := modelTestManager(t, "/work/proj", replyWithModel("claude-opus-5-5"))
	meta := sessions.Meta{Harness: "claude", Dir: "/work/proj", ConversationID: "conv"}
	if got := s.SessionInfo(meta).Model; got != "claude-opus-5-5" {
		t.Fatalf("model = %q, want claude-opus-5-5", got)
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
	if got := s.SessionInfo(meta).Model; got != "claude-sonnet-5-5" {
		t.Errorf("model = %q after an appended reply, want claude-sonnet-5-5", got)
	}
}

func TestSessionInfoModel_CodexKeepsSavedValues(t *testing.T) {
	t.Parallel()
	s := modelTestServer(t, "/work/proj", replyWithModel("claude-opus-5-5"))
	info := s.SessionInfo(sessions.Meta{Harness: "codex", Dir: "/work/proj", ConversationID: "conv", Tier: "worker", Model: "gpt-5", Effort: "low"})
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
	s := infoManager(nil, home)
	info := s.SessionInfo(sessions.Meta{Harness: "codex", ConversationID: "conv", Tier: "worker", Model: "gpt-6.1-sol", Effort: "low"})
	if info.Model != "gpt-6.1-terra" || info.Effort != "high" || !info.ModelSwitched {
		t.Errorf("model/effort/switched = %q/%q/%v, want gpt-6.1-terra/high/true", info.Model, info.Effort, info.ModelSwitched)
	}
}

func TestSessionInfoContext_CodexRolloutGivesCountAndLevel(t *testing.T) {
	t.Parallel()
	tokenCount := func(tokens int) string {
		return fmt.Sprintf(`{"type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":%d}}}}`, tokens)
	}
	for name, tc := range map[string]struct {
		lines []string
		want  int
		total int64
	}{
		"newest token_count wins": {
			[]string{tokenCount(30000), `{"type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":80000},"total_token_usage":{"total_tokens":300000}}}}`},
			80000, 300000,
		},
		"zero reading is skipped":   {[]string{tokenCount(80000), tokenCount(0)}, 80000, 0},
		"no token_count gives none": {[]string{`{"type":"turn_context","payload":{"model":"gpt-6.1-sol"}}`}, 0, 0},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			dir := filepath.Join(home, "sessions", "2026", "10", "30")
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "rollout-conv.jsonl"), []byte(strings.Join(tc.lines, "\n")+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			info := infoManager(nil, home).SessionInfo(sessions.Meta{Harness: "codex", ConversationID: "conv"})
			if tc.want == 0 {
				if info.ContextTokens != nil || info.ContextLevel != "" {
					t.Errorf("context = %v/%q, want none", info.ContextTokens, info.ContextLevel)
				}
				return
			}
			if info.ContextTokens == nil || *info.ContextTokens != tc.want || info.ContextLevel != agent.LevelYellow {
				t.Errorf("context = %v/%q, want %d/yellow", info.ContextTokens, info.ContextLevel, tc.want)
			}
			if tc.total > 0 && (info.Usage == nil || info.Usage.TotalTokens != tc.total || info.Usage.EstimatedCost != nil) {
				t.Errorf("usage = %+v, want %d total tokens and no estimated cost", info.Usage, tc.total)
			}
		})
	}
}

func TestSessionInfoContext_HarnessWithNoChatGivesNone(t *testing.T) {
	t.Parallel()
	meta := sessions.Meta{Harness: "shell", Dir: "/work/proj", ConversationID: "conv"}
	info := modelTestServer(t, "/work/proj", replyWithUsage("claude-opus-5-5", 80000)).SessionInfo(meta)
	if info.ContextTokens != nil || info.ContextLevel != "" {
		t.Errorf("context = %v/%q, want none", info.ContextTokens, info.ContextLevel)
	}
}

func TestSessionInfoModel_ModelSwitchedNote(t *testing.T) {
	t.Parallel()
	meta := func(model string) sessions.Meta {
		return sessions.Meta{Harness: "claude", Dir: "/work/proj", ConversationID: "conv", Tier: "worker", Model: model}
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
		if got := s.SessionInfo(meta(tc.launched)).ModelSwitched; got != tc.want {
			t.Errorf("%s: ModelSwitched = %v, want %v", name, got, tc.want)
		}
	}
}

func TestContinueAction_ResetsEachHarnessItsOwnWay(t *testing.T) {
	t.Parallel()
	for agent, want := range map[string][]string{
		"claude": {"/clear", "/gx-design epic"},
		"codex":  {"/new", "$gx-design epic"},
		"fake":   {"/clear", "/gx-design epic"},
	} {
		if got := chat.ContinueAction(designRef, agent); !slices.Equal(got, want) {
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
		next := chat.ChatNext(msgs, kind, designRef, false)
		if next == nil || !slices.Equal(next.Action.Action, want) {
			t.Errorf("%s next = %+v, want action %q", kind, next, want)
		}
	}
}
