package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/elentok/gx/testutil/agentfake"
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
		{Name: "clear", Description: "Clear the conversation", ArgumentHint: "[name]", Aliases: []string{"reset", "new"}, Builtin: true},
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
	return sc
}
