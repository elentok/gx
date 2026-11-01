package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil/agentfake"
	"github.com/elentok/gx/tickets"
)

// slowProbeDelay is longer than any flow waits for a command list (flow 149
// waits less).
const slowProbeDelay = "6s"

// featureSkill is the skill only the `feature` worktree of demo holds.
const featureSkill = "feature-only-skill"

// commandEntries is the list both fakes answer with. fakeclaude lists all of
// it; fakecodex lists it as skills, so review-skill shows up twice there. A
// skill with user-invocable false is absent: Claude leaves it out, so the fake
// does not list it.
func commandEntries() []agentfake.ProbeEntry {
	return []agentfake.ProbeEntry{
		{Name: "clear", Description: "Clear the conversation", ArgumentHint: "[name]", Aliases: []string{"reset", "new"}, Builtin: true}, // oldword:ok /clear's real alias in Claude
		{Name: "cost", Description: "Show the session cost", Builtin: true},
		{Name: "model", Description: "Switch model", ArgumentHint: "[model]", Builtin: true},
		{Name: "compact", Description: "Compact the conversation", ArgumentHint: "[instructions]", Builtin: true},
		{Name: "review-skill", Description: "Review a change", ArgumentHint: "<pr>"},
		{Name: "review-skill", Description: "Review a change (second copy)"},
		{Name: "html-skill", Description: "Turn a <template> into a page"},
		// A skill whose author set disable-model-invocation: Claude lists it, and
		// only a person can run it.
		{Name: "deploy-notes", Description: "Write deploy notes (people only)"},
		{Name: "acme:lint", Description: "Lint the project (plugin)"},
		{Name: "acme-srv:review_pr (MCP)", Description: "Review a PR"},
	}
}

// commandsScenario extends agents with the command-list fakes, three sessions
// that carry command entries, and an extra skill in the feature worktree. delay
// and fail shape the list probes.
func commandsScenario(delay string, fail bool) scenario {
	sc := agentsScenario()
	sc.commands = &agentfake.ProbeConfig{Entries: commandEntries(), Delay: delay, Fail: fail}
	base := sc.gitSetup
	sc.gitSetup = func(f *fixture, repo string) error {
		if err := base(f, repo); err != nil {
			return err
		}
		feature := filepath.Join(repo, ".worktrees", "feature")
		layer, err := json.Marshal(agentfake.ProbeConfig{Entries: append(commandEntries(),
			agentfake.ProbeEntry{Name: featureSkill, Description: "Only in the feature worktree"})})
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(feature, agentfake.CommandsCwdFile), layer, 0o644)
	}
	cost := make([]string, 14)
	for i := range cost {
		cost[i] = fmt.Sprintf("\x1b[1mcost line %d\x1b[0m: \x1b[32m$0.%02d\x1b[0m", i+1, i)
	}
	costOutput, empty := strings.Join(cost, "\n"), ""
	sc.sessions = append(sc.sessions,
		sessionSpec{project: project, prompt: "hello", read: true, steps: []commandStep{
			{prompt: "/cost", output: &costOutput},
			{prompt: "/model", output: &empty},
			{prompt: "/todos", output: &empty},
			{prompt: "/compact", turn: true},
		}},
		sessionSpec{project: project, harness: "codex", steps: []commandStep{{prompt: "/status"}}},
		sessionSpec{project: "site", prompt: "hello", crash: true, read: true},
	)
	sc.sessions = append(sc.sessions, blockedCommandSessions()...)
	sc.sessions = append(sc.sessions, stateCommandSessions()...)
	addQueueAndDesignAgents(&sc)
	return sc
}

// stateCommandSessions are the plain sessions in the remaining states: Blocked
// on an error (Codex), Rate limited, and a second Working session, so a flow can send
// keys to one while the first stays untouched.
func stateCommandSessions() []sessionSpec {
	return []sessionSpec{
		// Only Codex reaches "Agent hit an error": Claude's status has no error kind.
		{project: project, harness: "codex", failure: server.FailureKindBlocked,
			control: &agentfake.Control{Action: agentfake.ActionTaskError, Message: agentfake.CodexModelError, Error: "model_not_found"}},
		{project: project, prompt: "hello", read: true, failure: server.FailureKindRateLimit,
			control: &agentfake.Control{Action: agentfake.ActionAPIError, Error: "rate_limit", Status: 429, Message: "You've hit your limit · resets at 3pm"}},
		{project: project, prompt: agentfake.ChatSlowMarker + " a second long turn"},
	}
}

// addQueueAndDesignAgents adds a held Queue agent, a Claude design session with
// auto-advance on, and a Codex design session. The Codex one lives in gx, whose
// project config defaults to Codex, so no seeded Claude session changes harness.
func addQueueAndDesignAgents(sc *scenario) {
	const queueEpic, autoMap, codexMap = "queue-epic", "design-auto", "design-codex"
	step := string(tickets.StepResolveTickets)
	sc.epics = append(sc.epics,
		epic{project, queueEpic, []ticket{{"01", "held", servertest.TicketOpts{}}}},
		epic{project, autoMap, nil},
		epic{"gx", codexMap, nil},
	)
	sc.designSteps[project+":"+autoMap] = step
	sc.designSteps["gx:"+codexMap] = step
	sc.ownRepos = append(sc.ownRepos, "gx")
	sc.projectExtra = map[string]string{"gx": `,"agents":{"default-harness":"codex"}`}
	sc.queue = []string{project + ":" + queueEpic}
	sc.hold = true
	sc.openDesigns = []string{project + ":" + autoMap, "gx:" + codexMap}
}

// blockedCommandSessions are the sessions Blocked at a dialog or a permission
// prompt, one per way gx reads (or fails to read) what they wait on. Each ends
// as a Blocked failure.
func blockedCommandSessions() []sessionSpec {
	block := func(c agentfake.Control) *agentfake.Control {
		c.Action = agentfake.ActionBlock
		return &c
	}
	// dontAsk is a Bash prompt whose first option is not a plain Yes.
	dontAsk := agentfake.BashPermissionDialog("rg --files")
	dontAsk[4] = "❯ 1. Yes, and don't ask again for rg commands"
	blocked := func(s sessionSpec) sessionSpec {
		s.failure = server.FailureKindBlocked
		return s
	}
	codexBlock := func(c agentfake.Control) sessionSpec {
		return blocked(sessionSpec{project: project, harness: "codex", control: &c})
	}
	return []sessionSpec{
		// A dialog /model opened.
		blocked(sessionSpec{project: project, prompt: "hello", read: true, steps: []commandStep{
			{prompt: "/model", control: &agentfake.Control{Action: agentfake.ActionDialog}, blocks: true},
		}}),
		blocked(sessionSpec{project: project, prompt: "hello", read: true,
			control: block(agentfake.Control{Dialog: agentfake.BashPermissionDialog("rg readable-command")})}),
		blocked(sessionSpec{project: project, prompt: "hello", read: true,
			control: block(agentfake.Control{Dialog: []string{"▒▒▒▒ nothing gx can read ▒▒▒▒"}})}),
		blocked(sessionSpec{project: project, prompt: "hello", read: true, control: block(agentfake.Control{Dialog: dontAsk})}),
		// Enter leaves the dialog open, so Allow once never lands.
		blocked(sessionSpec{project: project, prompt: "hello", read: true,
			control: &agentfake.Control{Action: agentfake.ActionIgnoreAllow},
			steps:   []commandStep{{prompt: agentfake.PermissionPromptPrefix + "rg ignores-allow", blocks: true}}}),
		codexBlock(agentfake.Control{Action: agentfake.ActionPermission, Command: "rg codex-hook"}),
		codexBlock(agentfake.Control{Action: agentfake.ActionPermissionScreen, Command: "rg codex-screen"}),
		codexBlock(agentfake.Control{Action: agentfake.ActionPermissionScreen, Dialog: []string{"▒▒▒▒ nothing gx can read ▒▒▒▒"}}),
	}
}
