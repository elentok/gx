package spec

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/elentok/gx/server/wire"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil/agentfake"
	"github.com/elentok/gx/tickets"
)

// SlowProbeDelay is longer than any flow waits for a command list (flow 149
// waits less).
const SlowProbeDelay = "6s"

// FeatureSkill is the skill only the `feature` worktree of demo holds.
const FeatureSkill = "feature-only-skill"

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

// commandStepsSession is an idle Claude session whose chat holds the /cost
// output and the /model pill, so a flow can read a command's result.
func commandStepsSession() sessionSpec {
	cost := make([]string, 14)
	for i := range cost {
		cost[i] = fmt.Sprintf("\x1b[1mcost line %d\x1b[0m: \x1b[32m$0.%02d\x1b[0m", i+1, i)
	}
	costOutput, empty := strings.Join(cost, "\n"), ""
	return sessionSpec{Project: project, Prompt: "hello", Read: true, Steps: []commandStep{
		{Prompt: "/cost", Output: &costOutput},
		{Prompt: "/model", Output: &empty},
		{Prompt: "/todos", Output: &empty},
		{Prompt: "/compact", Turn: true},
	}}
}

// leanCommandSessions is the order the command flows rely on: four idle Claude
// sessions (claudeIdle 0..3, the last holds command output; flow 181 sends to
// three of them, and each sent command keeps its session busy), an idle Codex
// one, a Working one and an Exited one. The design sessions follow
// (addDesignAgents).
func leanCommandSessions() []sessionSpec {
	return []sessionSpec{
		{Project: project, Prompt: "hello", Read: true},
		{Project: project, Prompt: "hello", Read: true},
		{Project: project, Prompt: "hello", Read: true},
		commandStepsSession(),
		{Project: project, Harness: "codex"},
		{Project: project, Prompt: agentfake.ChatSlowMarker + " a long turn"},
		{Project: project, Prompt: "hello", Crash: true, Read: true},
	}
}

// withCommandFakes gives sc the command-list fakes and an extra skill in the
// feature worktree of demo. delay and fail shape the list probes.
func withCommandFakes(sc *scenario, delay string, fail bool) {
	sc.Commands = &agentfake.ProbeConfig{Entries: commandEntries(), Delay: delay, Fail: fail}
	base := sc.GitSetup
	sc.GitSetup = func(git Git, repo string) error {
		if base != nil {
			if err := base(git, repo); err != nil {
				return err
			}
		}
		feature := filepath.Join(repo, ".worktrees", "feature")
		if base == nil {
			if err := git(repo, "worktree", "add", "-b", "feature", feature); err != nil {
				return err
			}
		}
		layer, err := json.Marshal(agentfake.ProbeConfig{Entries: append(commandEntries(),
			agentfake.ProbeEntry{Name: FeatureSkill, Description: "Only in the feature worktree"})})
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(feature, agentfake.CommandsCwdFile), layer, 0o644)
	}
}

// CommandsScenario is the lean scenario of the command flows: seven sessions and
// the two design sessions, with no Blocked, Rate limited or Queue sessions.
func CommandsScenario(delay string, fail bool) scenario {
	sc := scenario{
		Codex:       true,
		Chat:        true,
		DesignSteps: map[string]string{},
		Sessions:    leanCommandSessions(),
	}
	withCommandFakes(&sc, delay, fail)
	addDesignAgents(&sc)
	return sc
}

// BlockedScenario extends agents with the command-list fakes, three sessions
// that carry command entries and every Blocked state the key and permission
// flows read.
func BlockedScenario() scenario {
	sc := AgentsScenario()
	// A key the fake ignores (flow 158) fails after this wait, not after the 45s default.
	sc.PromptGrace = "8s"
	withCommandFakes(&sc, "", false)
	sc.Sessions = append(sc.Sessions,
		commandStepsSession(),
		sessionSpec{Project: project, Harness: "codex", Steps: []commandStep{{Prompt: "/status"}}},
		sessionSpec{Project: "site", Prompt: "hello", Crash: true, Read: true},
	)
	sc.Sessions = append(sc.Sessions, blockedCommandSessions()...)
	sc.Sessions = append(sc.Sessions, stateCommandSessions()...)
	addQueueAgent(&sc)
	addDesignAgents(&sc)
	return sc
}

// stateCommandSessions are the plain sessions in the remaining states: Blocked
// on an error (Codex), Rate limited, and a second Working session, so a flow can send
// keys to one while the first stays untouched.
func stateCommandSessions() []sessionSpec {
	return []sessionSpec{
		// Only Codex reaches "Agent hit an error": Claude's status has no error kind.
		{Project: project, Harness: "codex", Failure: wire.FailureKindBlocked,
			Control: &agentfake.Control{Action: agentfake.ActionTaskError, Message: agentfake.CodexModelError, Error: "model_not_found"}},
		{Project: project, Prompt: "hello", Read: true, Failure: wire.FailureKindRateLimit,
			Control: &agentfake.Control{Action: agentfake.ActionAPIError, Error: "rate_limit", Status: 429, Message: "You've hit your limit · resets at 3pm"}},
		{Project: project, Prompt: agentfake.ChatSlowMarker + " a second long turn"},
	}
}

// addQueueAgent adds a held Queue agent.
func addQueueAgent(sc *scenario) {
	const queueEpic = "queue-epic"
	sc.Epics = append(sc.Epics, epic{project, queueEpic, []ticket{{"01", "held", servertest.TicketOpts{}}}})
	sc.Queue = []string{project + ":" + queueEpic}
	sc.Hold = true
	sc.QueueAfterSessions = true
}

// addDesignAgents adds a Claude design session with auto-advance on, and a
// Codex design session. The Codex one lives in gx, whose project config
// defaults to Codex, so no seeded Claude session changes harness.
func addDesignAgents(sc *scenario) {
	const autoMap, codexMap = "design-auto", "design-codex"
	step := string(tickets.StepResolveTickets)
	sc.Epics = append(sc.Epics, epic{project, autoMap, nil}, epic{"gx", codexMap, nil})
	sc.DesignSteps[project+":"+autoMap] = step
	sc.DesignSteps["gx:"+codexMap] = step
	sc.OwnRepos = append(sc.OwnRepos, "gx")
	sc.ProjectExtra = map[string]string{"gx": `,"agents":{"default-harness":"codex"}`}
	sc.OpenDesigns = append(sc.OpenDesigns, project+":"+autoMap, "gx:"+codexMap)
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
		s.Failure = wire.FailureKindBlocked
		return s
	}
	codexBlock := func(c agentfake.Control) sessionSpec {
		return blocked(sessionSpec{Project: project, Harness: "codex", Control: &c})
	}
	return []sessionSpec{
		// A dialog /model opened.
		blocked(sessionSpec{Project: project, Prompt: "hello", Read: true, Steps: []commandStep{
			{Prompt: "/model", Control: &agentfake.Control{Action: agentfake.ActionDialog}, Blocks: true},
		}}),
		blocked(sessionSpec{Project: project, Prompt: "hello", Read: true,
			Control: block(agentfake.Control{Dialog: agentfake.BashPermissionDialog("rg readable-command")})}),
		blocked(sessionSpec{Project: project, Prompt: "hello", Read: true,
			Control: block(agentfake.Control{Dialog: []string{"▒▒▒▒ nothing gx can read ▒▒▒▒"}})}),
		blocked(sessionSpec{Project: project, Prompt: "hello", Read: true, Control: block(agentfake.Control{Dialog: dontAsk})}),
		// Enter leaves the dialog open, so Allow once never lands.
		blocked(sessionSpec{Project: project, Prompt: "hello", Read: true,
			Control: &agentfake.Control{Action: agentfake.ActionIgnoreAllow},
			Steps:   []commandStep{{Prompt: agentfake.PermissionPromptPrefix + "rg ignores-allow", Blocks: true}}}),
		codexBlock(agentfake.Control{Action: agentfake.ActionPermission, Command: "rg codex-hook"}),
		codexBlock(agentfake.Control{Action: agentfake.ActionPermissionScreen, Command: "rg codex-screen"}),
		codexBlock(agentfake.Control{Action: agentfake.ActionPermissionScreen, Dialog: []string{"▒▒▒▒ nothing gx can read ▒▒▒▒"}}),
	}
}
