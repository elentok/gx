// Package agent holds the identity of the coding agent behind an iteration,
// its model tiers and its launch arguments. It is a leaf: it imports no other
// ralphloop package.
package agent

import (
	"fmt"
	"path/filepath"
	"slices"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/config"
	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/tickets"
)

// DefaultSmartZone is the context-token ceiling used when none is configured.
// It backstops the gx-implement skill's own proactive 90K split threshold, so
// a session that misses that threshold still gets paused before it runs away.
const DefaultSmartZone = SmartZoneCeiling

// SmartZoneCeiling is the one context-token ceiling for every model; `gx agent
// context-window --fits` shares it so chaining decisions match the pause point.
const SmartZoneCeiling = 130_000

// DefaultWorkerSkill is the skill each iteration invokes when none is
// configured.
const DefaultWorkerSkill = "gx-implement"

// codeReviewSkill is the skill launched directly for type: code-review
// tickets instead of the worker skill (see LaunchSkill). It's launched as its
// own fresh /command rather than entered from inside a gx-implement session
// because gx-code-review sets disable-model-invocation: true, which blocks an
// in-session Skill-tool hand-off but not the harness starting a session with
// the command already resolved.
const codeReviewSkill = "gx-code-review"

// VerifySkill is the skill launched for type: verify tickets.
const VerifySkill = "gx-verify-epic"

// LaunchSkill is the skill an iteration on t starts under: workerSkill,
// unless t's type routes it to its own skill.
func LaunchSkill(t tickets.Ticket, workerSkill string) string {
	switch {
	case t.IsCodeReview():
		return codeReviewSkill
	case t.IsVerify():
		return VerifySkill
	}
	return workerSkill
}

// AgentKind identifies the coding agent that drives an iteration.
type AgentKind string

const (
	AgentClaude AgentKind = "claude"
	AgentCodex  AgentKind = "codex"
)

// AgentFor maps a harness name to its agent kind: "codex" is Codex, anything
// else reads as Claude.
func AgentFor(harnessName string) AgentKind {
	if harnessName == string(AgentCodex) {
		return AgentCodex
	}
	return AgentClaude
}

func (a AgentKind) valid() bool {
	return a == AgentClaude || a == AgentCodex
}

// ValidateAgentKind reports whether agent names a supported iteration agent.
func ValidateAgentKind(agent AgentKind) error {
	if agent == "" {
		return nil
	}
	if !agent.valid() {
		return fmt.Errorf("invalid agent %q: must be claude or codex", agent)
	}
	return nil
}

// SkillPrompt is the prompt that starts agent on skill for ticketAddr.
func SkillPrompt(agent AgentKind, skill, ticketAddr string) string {
	return harness.SkillCommand(string(agent), skill, ticketAddr)
}

// claudeGuardSettings adds a PreToolUse hook that rejects shell file edits
// (sed -i, heredoc writes), which agents keep using despite the gx-implement
// skill forbidding them. See `gx agent guard-bash`.
//
// It also keeps the loop agent's context lean: no claude.ai synced skills and
// no claude.ai connectors (Claude Docs). Together they cut about 5k tokens
// from every turn's prefix. Design and regular agents are launched elsewhere
// and keep both, because they use the Docs connector.
const claudeGuardSettings = `{"syncClaudeAiSkills":false,"disableClaudeAiConnectors":true,"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"gx agent guard-bash"}]}]}}`

// loopDisallowedTools is LoopDisallowedTools plus tools a loop agent never
// uses. Banning them also drops their names from the deferred-tool list.
const loopDisallowedTools = agentrunner.LoopDisallowedTools + ",CronCreate,CronDelete,CronList,RemoteTrigger,EnterWorktree,ExitWorktree"

// AgentArgs is the launch argument list for agent. cwd is the agent's
// worktree; extraDirs are further writable dirs for a Codex sandbox. Claude
// ignores both.
func AgentArgs(agent AgentKind, cwd, scratchDir, epicName, model, effort string, extraDirs ...string) []string {
	if agent == AgentCodex {
		extra := append([]string{scratchDir, filepath.Join(scratchDir, epicName)}, extraDirs...)
		return harness.CodexArgs(harness.CodexArgsInput{
			Model: model, Effort: effort,
			WritableRoots: harness.CodexWritableRoots(cwd, extra...),
		})
	}
	args := []string{
		"--permission-mode", "auto", "--settings", claudeGuardSettings,
		"--disallowedTools", loopDisallowedTools,
	}
	// herdr types the launch command into a shell, so keep it short: the
	// prompt goes in a file when one can be written.
	if f := harness.GXSystemPromptFile(); f != "" {
		args = append(args, "--append-system-prompt-file", f)
	} else {
		args = append(args, "--append-system-prompt", harness.GXSystemPrompt)
	}
	if model != "" {
		args = append(args, "--model", model)
	}
	if effort != "" {
		args = append(args, "--effort", effort)
	}
	// herdr launches claude from these args itself, so the editor mode goes in here.
	return harness.MergeClaudeSettings(args)
}

// Tier names one of the three model/effort pairs configured per agent.
type Tier string

const (
	TierLight     Tier = "light"
	TierWorker    Tier = "worker"
	TierStrongest Tier = "strongest"
)

// AllTiers lists every tier, in picker order.
var AllTiers = []Tier{TierLight, TierWorker, TierStrongest}

// Valid reports whether t is one of AllTiers.
func (t Tier) Valid() bool { return slices.Contains(AllTiers, t) }

// ResolveTier is the one place that turns (agent, tier) into the model and
// effort to pass. An empty result means "pass no flag".
func ResolveTier(agents config.AgentsConfig, agent AgentKind, tier Tier) (model, effort string) {
	ac := agents.Claude
	if agent == AgentCodex {
		ac = agents.Codex
	}
	switch tier {
	case TierLight:
		return ac.Light.Model, ac.Light.Effort
	case TierStrongest:
		return ac.Strongest.Model, ac.Strongest.Effort
	}
	return ac.Model, ac.Effort
}
