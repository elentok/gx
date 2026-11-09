package prompt

import (
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/elentok/gx/grillround"
	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/server/projects"
	"github.com/elentok/gx/server/sessions"
	"github.com/elentok/gx/server/sessions/chat"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil/agentfake"
	"github.com/elentok/gx/tickets"
)

var designRef = chat.FlowRef{Flow: tickets.FlowDesign, Key: "gx:epic/03"}

// codexPrompt is a Prompt that reads codex rollouts under home.
func codexPrompt(home string) *Prompt {
	log := slog.New(slog.DiscardHandler)
	env := func() projects.Env { return projects.Env{Env: []string{"CODEX_HOME=" + home}} }
	return New(Deps{Log: log, Projects: projects.NewRegistry(env, log, nil, nil, nil, nil)})
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
	p := codexPrompt(home)
	meta := sessions.Meta{Harness: "codex", ConversationID: "conv"}

	text, ok := p.lastAssistantText("s1", meta)
	if !ok {
		t.Fatal("lastAssistantText found no codex rollout")
	}
	a, matched := designActionable(text, designRef, meta.Harness, false)
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
	p := codexPrompt(home)

	text, ok := p.lastAssistantText("s1", sessions.Meta{Harness: "codex", ConversationID: "conv"})
	if !ok || !grillround.HasMarker(text) || strings.Contains(text, "old turn") {
		t.Errorf("text = %q, ok %v; want the last turn's round", text, ok)
	}
}

func continueMeta(edit func(*sessions.Meta)) sessions.Meta {
	m := sessions.Meta{
		Harness:    string(harness.Claude),
		Status:     wire.SessionStatus{State: wire.SessionIdle},
		Actionable: wire.SessionActionable{Reason: "done", Action: []string{"/clear"}, Derived: true},
	}
	if edit != nil {
		edit(&m)
	}
	return m
}

func TestDecideActionable(t *testing.T) {
	t.Parallel()
	cont := wire.SessionActionable{Reason: "next", Action: []string{"/clear", "/gx-design x"}, Derived: true}
	policy := actionablePolicy{Live: true, MaxAutoSends: 5}
	tests := []struct {
		name    string
		cur     func(*sessions.Meta)
		matched bool
		policy  actionablePolicy
		want    actionableOutcome
		reason  string
	}{
		{"gx-raised kept", func(m *sessions.Meta) { m.Actionable = wire.SessionActionable{Reason: "gx", GxRaised: true} }, true, policy, actionableNone, "gx"},
		{"derived cleared when unmatched", nil, false, policy, actionableCleared, ""},
		{"agent-raised kept when unmatched", func(m *sessions.Meta) { m.Actionable.Derived = false }, false, policy, actionableNone, "done"},
		{"duplicate deduped", func(m *sessions.Meta) { m.Actionable = cont }, true, policy, actionableNone, "next"},
		{"not promptable untouched", func(m *sessions.Meta) { m.Status = wire.SessionStatus{State: wire.SessionWorking} }, true, policy, actionableNone, "done"},
		{"advance", nil, true, policy, actionableAdvance, "next"},
		{"raised when not live", nil, true, actionablePolicy{MaxAutoSends: 5}, actionableRaised, "next"},
		{"raised with the switch off", func(m *sessions.Meta) { m.AutoAdvanceOff = true }, true, policy, actionableRaised, "next"},
		{"advance on codex", func(m *sessions.Meta) { m.Harness = string(harness.Codex) }, true, policy, actionableAdvance, "next"},
		{"capped", func(m *sessions.Meta) { m.AutoSends = 5 }, true, policy, actionableCapped, "next"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			next, got := decideActionable(continueMeta(tt.cur), cont, tt.matched, tt.policy)
			if got != tt.want {
				t.Fatalf("outcome = %v, want %v", got, tt.want)
			}
			if next.Reason != tt.reason {
				t.Fatalf("next reason = %q, want %q", next.Reason, tt.reason)
			}
		})
	}
}

// Every refusal names its reason, and a Grilling round (an open question, the
// shape a prototype review takes) is never reported as advanceable.
func TestRefusalReason(t *testing.T) {
	t.Parallel()
	cont := wire.SessionActionable{Reason: "next", Action: []string{"/clear"}, Derived: true}
	grill := wire.SessionActionable{Reason: "q", Action: []string{"/clear"}, Derived: true, Grilling: true}
	tests := []struct {
		name string
		meta sessions.Meta
		a    wire.SessionActionable
		live bool
		want string
	}{
		{"open question", sessions.Meta{}, grill, true, "open-question"},
		{"switch off", sessions.Meta{AutoAdvanceOff: true}, cont, true, "switch-off"},
		{"no action", sessions.Meta{}, wire.SessionActionable{Reason: "r", Derived: true}, true, "no-action"},
		{"not live", sessions.Meta{}, cont, false, "not-live"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := refusalReason(tt.meta, tt.a, tt.live); got != tt.want {
				t.Errorf("refusalReason = %q, want %q", got, tt.want)
			}
		})
	}
}

// The decision line carries the session, ticket, decision and reason.
func TestLogAutoAdvance_WritesOneLinePerDecision(t *testing.T) {
	t.Parallel()
	var buf strings.Builder
	p := New(Deps{Log: slog.New(slog.NewTextHandler(&buf, nil))})
	p.logAutoAdvance("s1", "gx:epic/06", "refused", "open-question", "Q1 waits")
	out := buf.String()
	for _, want := range []string{"auto-advance decision", "session=s1", "ticket=gx:epic/06", "decision=refused", "reason=open-question"} {
		if !strings.Contains(out, want) {
			t.Errorf("log %q lacks %q", out, want)
		}
	}
	if n := strings.Count(out, "\n"); n != 1 {
		t.Errorf("log has %d lines, want 1", n)
	}
}

// A message that ends on a prose question is a session waiting on the person:
// the run-end line above it must not raise a Continue that auto-advance sends.
func TestDesignActionable_EndingOnAProseQuestionRaisesNoContinue(t *testing.T) {
	t.Parallel()
	text := agentfake.DesignRunEnd + "\n\nDo you agree with B?\n"
	if a, matched := designActionable(text, designRef, "claude", false); matched {
		t.Errorf("actionable = %+v, want none while a question waits", a)
	}
	if _, matched := designActionable(agentfake.DesignRunEnd+"\n", designRef, "claude", false); !matched {
		t.Error("a plain run end must still raise its Continue")
	}
}
