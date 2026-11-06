package spec

import (
	"path/filepath"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil/agentfake"
	"github.com/elentok/gx/tickets"
)

// AgentsFailuresScenario is one Claude session per failure the Agents page
// shows above the composer. It is not part of AgentsScenario because the short
// stall threshold would stall that scenario's Working session too.
func AgentsFailuresScenario() scenario {
	return scenario{
		Chat:           true,
		StallThreshold: "3s",
		Sessions: []sessionSpec{
			{Project: project, Prompt: agentfake.ChatSlowMarker + " a stuck turn", Failure: wire.FailureKindStalled},
			{Project: project, Prompt: "hello", Read: true, Failure: wire.FailureKindAuth,
				Control: &agentfake.Control{Action: agentfake.ActionAPIError, Error: "authentication_error", Status: 401, Message: "Invalid API key"}},
			{Project: project, Prompt: "hello", Read: true, Failure: wire.FailureKindRateLimit,
				Control: &agentfake.Control{Action: agentfake.ActionAPIError, Error: "rate_limit", Status: 429, Message: "You've hit your limit · resets at 3pm"}},
			{Project: project, Prompt: "hello", Read: true, Failure: wire.FailureKindBlocked,
				Control: &agentfake.Control{Action: agentfake.ActionBlock}},
		},
	}
}

// QueueAgentsScenario lists one PTY Queue agent and one Design agent. The
// ticket's worker idles in a long turn until the hold file is removed.
func QueueAgentsScenario() scenario {
	const mapAddr = "design-epic"
	return scenario{
		Epics: []epic{
			{project, "queue-epic", []ticket{{"01", "held", servertest.TicketOpts{}}}},
			{project, mapAddr, nil},
		},
		DesignSteps: map[string]string{project + ":" + mapAddr: string(tickets.StepResolveTickets)},
		Queue:       []string{project + ":queue-epic"},
		Hold:        true,
		Chat:        true,
		Sessions: []sessionSpec{
			{Project: project, Prompt: "hello", Link: project + ":" + mapAddr, Read: true},
		},
	}
}

// DefaultHarnessScenario is the Settings and Agents fixture for the Default
// harness. The global default is Claude. gx overrides to Codex; demo and
// dotfiles inherit. demo's cap of one agent keeps the second epic's ticket
// waiting behind the held worker of the first.
func DefaultHarnessScenario() scenario {
	const codexOverride = `,"agents":{"default-harness":"codex"}`
	return scenario{
		Epics: []epic{
			{project, "running-epic", []ticket{{"01", "running", servertest.TicketOpts{}}}},
			{project, "waiting-epic", []ticket{{"01", "waiting", servertest.TicketOpts{}}}},
			{"dotfiles", "dotfiles-epic", nil},
			{"gx", "gx-epic", nil},
		},
		ProjectExtra: map[string]string{project: `,"max-agents":1`, "gx": codexOverride},
		OwnRepos:     []string{"dotfiles", "gx"},
		Queue:        []string{project + ":running-epic", project + ":waiting-epic"},
		Hold:         true,
		Chat:         true,
		Sessions: []sessionSpec{
			{Project: project, Prompt: "hello", Read: true},
			{Project: "gx", Harness: "codex"},
			{Project: "dotfiles", Prompt: "hello", Read: true,
				Control:    &agentfake.Control{Action: agentfake.ActionModel, Model: SwitchedModel},
				ThenPrompt: "and again"},
		},
	}
}

// CodexQueueScenario fills demo's two agent slots with held workers (one Codex,
// one Claude) and leaves three tickets waiting. A queue item's agent covers its
// whole root, so each ticket has an epic of its own.
func CodexQueueScenario() scenario {
	addr := func(name string) string { return project + ":" + name }
	one := func(opts servertest.TicketOpts) []ticket { return []ticket{{"01", "work", opts}} }
	return scenario{
		Codex: true,
		Epics: []epic{
			{project, "codex-running", one(servertest.TicketOpts{})},
			{project, "claude-running", one(servertest.TicketOpts{})},
			{project, "follows-default", one(servertest.TicketOpts{})},
			{project, "pinned", one(servertest.TicketOpts{Agent: "codex"})},
			{project, "ran-on-claude", one(servertest.TicketOpts{})},
		},
		ProjectExtra: map[string]string{project: `,"max-agents":2`},
		Hold:         true,
		RanOnClaude:  []string{addr("ran-on-claude/01")},
		Queue: []string{
			addr("codex-running"), addr("claude-running"),
			addr("follows-default"), addr("pinned"), addr("ran-on-claude"),
		},
		QueueAgents: map[string]string{addr("codex-running"): "codex", addr("claude-running"): "claude"},
	}
}

// CodexDesignScenario opens a design session on a map while the global Default
// harness is Codex, so the launch resolves to Codex.
func CodexDesignScenario() scenario {
	const mapAddr = "design-epic"
	return scenario{
		Codex:        true,
		Epics:        []epic{{project, mapAddr, nil}},
		DesignSteps:  map[string]string{project + ":" + mapAddr: string(tickets.StepResolveTickets)},
		ProjectExtra: map[string]string{project: `,"agents":{"default-harness":"codex"}`},
		Design:       []string{agentfake.DesignTurnRound, agentfake.DesignTurnRunEnd},
		OpenDesigns:  []string{project + ":" + mapAddr},
	}
}

// PromoteScenario is AgentsScenario plus two Codex sessions with one message
// each (a promote keeps its session busy, so each width takes one), a held Queue
// agent and two design sessions: every kind of session the Promote to Design
// button shows on or stays off.
func PromoteScenario() scenario {
	sc := AgentsScenario()
	codexWithMessage := sessionSpec{Project: project, Harness: "codex", Prompt: "hello", Read: true}
	sc.Sessions = append(sc.Sessions, codexWithMessage, codexWithMessage)
	addQueueAgent(&sc)
	addDesignAgents(&sc)
	return sc
}

// SwitchedModel is the model the switched session's later replies report; it
// differs from the model its tier launched with.
const SwitchedModel = "claude-haiku-5-5"

// AgentsScenario is the Agents page fixture: eight sessions in three
// projects, one per row of the mock, and a worktree in demo.
func AgentsScenario() scenario {
	const mapAddr = "design-epic"
	return scenario{
		Codex:    true,
		Projects: []string{"other"},
		Epics: []epic{
			{project, mapAddr, nil},
			{"site", mapAddr, nil},
		},
		DesignSteps: map[string]string{
			project + ":" + mapAddr: string(tickets.StepResolveTickets),
			"site:" + mapAddr:       string(tickets.StepResolveTickets),
		},
		OwnRepos: []string{"other", "site"},
		Chat:     true,
		GitSetup: func(git Git, repo string) error {
			return git(repo, "worktree", "add", "-b", "feature", filepath.Join(repo, ".worktrees", "feature"))
		},
		Sessions: []sessionSpec{
			{Project: project, Prompt: "hello", Read: true},
			{Project: project, Prompt: agentfake.ChatSlowMarker + " a long turn"},
			{Project: project, Prompt: "hello", Link: project + ":" + mapAddr, Read: true, Actionable: "needs a decision"},
			{Project: project, Harness: "codex"},
			{Project: "other", Prompt: "hello"},
			{Project: "other", Actionable: "needs a decision"},
			{Project: "site", Prompt: "hello", Link: "site:" + mapAddr, Read: true},
			{Project: "site", Prompt: "hello", Crash: true, Read: true},
		},
	}
}
