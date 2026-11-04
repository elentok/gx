package ralphloop

import (
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

func TestSkillPrompt_MatchesTheHarnessSkillCommand(t *testing.T) {
	t.Parallel()
	for _, agent := range []AgentKind{AgentClaude, AgentCodex, "fake"} {
		for _, addr := range []string{"", "gx:epic/01"} {
			want := harness.SkillCommand(string(agent), "gx-implement", addr)
			if got := skillPrompt(agent, "gx-implement", addr); got != want {
				t.Errorf("skillPrompt(%q, %q) = %q, want %q", agent, addr, got, want)
			}
		}
	}
	if got := skillPrompt(AgentCodex, "gx-implement", "gx:epic/01"); got != "$gx-implement gx:epic/01" {
		t.Errorf("codex prompt = %q", got)
	}
}

// launchSettings is the one --settings claude gets: the guard hook, the lean
// context keys plus normal editor mode.
const launchSettings = `{"disableClaudeAiConnectors":true,"editorMode":"normal","hooks":{"PreToolUse":[{"hooks":[{"command":"gx agent guard-bash","type":"command"}],"matcher":"Bash"}]},"syncClaudeAiSkills":false}`

// notARepo is a cwd git cannot read, so agentArgs adds no git roots.
const notARepo = "/nonexistent/not-a-repo"

var codexTestRoots = []string{"scratch", filepath.Join("scratch", "epic")}

func TestAgentArgs_ModelAndEffort(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		agent  AgentKind
		model  string
		effort string
		want   []string
	}{
		{
			name:  "claude both empty unchanged",
			agent: AgentClaude,
			want:  []string{"--permission-mode", "auto", "--settings", launchSettings, "--disallowedTools", loopDisallowedTools, "--append-system-prompt-file", harness.GXSystemPromptFile()},
		},
		{
			name:   "claude both set",
			agent:  AgentClaude,
			model:  "sonnet",
			effort: "medium",
			want:   []string{"--permission-mode", "auto", "--settings", launchSettings, "--disallowedTools", loopDisallowedTools, "--append-system-prompt-file", harness.GXSystemPromptFile(), "--model", "sonnet", "--effort", "medium"},
		},
		{
			name:  "claude model only",
			agent: AgentClaude,
			model: "sonnet",
			want:  []string{"--permission-mode", "auto", "--settings", launchSettings, "--disallowedTools", loopDisallowedTools, "--append-system-prompt-file", harness.GXSystemPromptFile(), "--model", "sonnet"},
		},
		{
			name:   "claude effort only",
			agent:  AgentClaude,
			effort: "medium",
			want:   []string{"--permission-mode", "auto", "--settings", launchSettings, "--disallowedTools", loopDisallowedTools, "--append-system-prompt-file", harness.GXSystemPromptFile(), "--effort", "medium"},
		},
		{
			name:  "codex both empty unchanged",
			agent: AgentCodex,
			want:  harness.CodexArgs(harness.CodexArgsInput{WritableRoots: codexTestRoots}),
		},
		{
			name:   "codex both set",
			agent:  AgentCodex,
			model:  "gpt-5.6-sol",
			effort: "medium",
			want:   harness.CodexArgs(harness.CodexArgsInput{Model: "gpt-5.6-sol", Effort: "medium", WritableRoots: codexTestRoots}),
		},
		{
			name:  "codex model only",
			agent: AgentCodex,
			model: "gpt-5.6-sol",
			want:  harness.CodexArgs(harness.CodexArgsInput{Model: "gpt-5.6-sol", WritableRoots: codexTestRoots}),
		},
		{
			name:   "codex effort only",
			agent:  AgentCodex,
			effort: "medium",
			want:   harness.CodexArgs(harness.CodexArgsInput{Effort: "medium", WritableRoots: codexTestRoots}),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := agentArgs(tc.agent, notARepo, "scratch", "epic", tc.model, tc.effort)
			if !slices.Equal(got, tc.want) {
				t.Errorf("agentArgs() = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestAllDone_RepairIsNotComplete: a paused (repair) ticket used to count as
// terminal and end the run, and now must not — the run pauses on it instead.
func TestAllDone_RepairIsNotComplete(t *testing.T) {
	t.Parallel()
	epic := tickets.Epic{Tickets: []tickets.Ticket{
		{Number: 1, Status: "done"},
		{Number: 2, Status: "paused", PauseReason: schema.PauseIterationError},
	}}
	if allDone(epic) {
		t.Errorf("allDone() = true, want false while ticket 2 paused for repair")
	}
}

func TestAllDone_OpenTicketIsNotComplete(t *testing.T) {
	t.Parallel()
	epic := tickets.Epic{Tickets: []tickets.Ticket{
		{Number: 1, Status: "done"},
		{Number: 2, Status: "open"},
	}}
	if allDone(epic) {
		t.Errorf("allDone() = true, want false while ticket 2 is still open")
	}
}

func TestAllDone_EveryTicketDone(t *testing.T) {
	t.Parallel()
	epic := tickets.Epic{Tickets: []tickets.Ticket{
		{Number: 1, Status: "done"},
		{Number: 2, Status: "done"},
	}}
	if !allDone(epic) {
		t.Errorf("allDone() = false, want true when every ticket is done")
	}
}

// TestAllDone_WaitingForChildrenNotDone: a done ticket whose fork subtree
// (Parent) is still open renders as waiting-for-children, which must not count
// as terminal — otherwise an epic would be reported complete with unfinished
// forked work still inside it.
func TestAllDone_WaitingForChildrenNotDone(t *testing.T) {
	t.Parallel()
	parent := "1"
	epic := tickets.Epic{Tickets: []tickets.Ticket{
		{Number: 1, Identifier: "01", Status: "done"},
		{Number: 1, Identifier: "01a", Status: "open", Parent: &parent},
	}}
	if allDone(epic) {
		t.Errorf("allDone() = true, want false: ticket 01 is waiting-for-children")
	}
}

// TestGate_WakePaused_ShortensPauseWait: WakePaused cuts a pause wait short
// instead of it running out a long pause-poll-interval wait.
func TestGate_WakePaused_ShortensPauseWait(t *testing.T) {
	t.Parallel()
	gate := NewGate()
	waiting := make(chan struct{})
	woke := make(chan struct{})

	go func() {
		timer := time.After(time.Hour)
		pauseWake := gate.PauseWake()
		close(waiting)
		select {
		case <-timer:
		case <-pauseWake:
		}
		close(woke)
	}()

	<-waiting
	gate.WakePaused()

	select {
	case <-woke:
	case <-time.After(5 * time.Second):
		t.Fatal("WakePaused() did not interrupt an in-progress pause wait")
	}
}
