package ralphloop

import (
	"cmp"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/config"
	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/tickets"
)

// ticketTrailerKey is the commit-message trailer mergeCherryPick stamps onto
// every merged commit (see Deps.AppendTrailer) and verify's last-resort
// fallback searches for (Deps.TrailerCommitExists).
const ticketTrailerKey = "Ralph-Loop-Ticket"

// TicketTrailerKey is ticketTrailerKey for the repair verbs.
const TicketTrailerKey = ticketTrailerKey

// tokensTrailerKey and elapsedTrailerKey are the commit-message trailers
// mergeCherryPick stamps alongside ticketTrailerKey whenever the merging
// session's own metrics are available (see writeMergedMetrics) — the same
// actual_context_window/elapsed_time values written to the ticket's
// frontmatter, surfaced on the merged commit too.
const (
	tokensTrailerKey  = "Ralph-Loop-Tokens"
	elapsedTrailerKey = "Ralph-Loop-Elapsed"
	costTrailerKey    = "Ralph-Loop-Cost"
)

// ticketTrailerValue builds ticketTrailerKey's value, scoped to epicName: a
// bare ticket identifier repeats across every epic (each restarts numbering
// from 01), and TrailerCommitExists searches the whole repo history, not just
// commits under this epic. An unscoped value can match a same-numbered
// ticket from a completely unrelated epic merged long ago, misreporting a
// genuinely unmerged ticket as already present.
func ticketTrailerValue(epicName, identifier string) string {
	return epicName + "/" + identifier
}

// defaultMaxParallel is how many iterations run concurrently when no
// MaxParallel is configured.
const defaultMaxParallel = 2

// defaultSmartZone is the context-token ceiling used when none is configured.
// It backstops the gx-implement skill's own proactive 90K split threshold, so
// a session that misses that threshold still gets paused before it runs away.
const defaultSmartZone = SmartZoneCeiling

// SmartZoneCeiling is the one context-token ceiling for every model; `gx agent
// context-window --fits` shares it so chaining decisions match the pause point.
const SmartZoneCeiling = 130_000

// defaultWorkerSkill is the skill each iteration invokes when none is
// configured.
const defaultWorkerSkill = "gx-implement"

// codeReviewSkill is the skill launched directly for type: code-review
// tickets instead of the worker skill (see launchSkill). It's launched as its
// own fresh /command rather than entered from inside a gx-implement session
// because gx-code-review sets disable-model-invocation: true, which blocks an
// in-session Skill-tool hand-off but not the harness starting a session with
// the command already resolved.
const codeReviewSkill = "gx-code-review"

// verifySkill is the skill launched for type: verify tickets.
const verifySkill = "gx-verify-epic"

// launchSkill is the skill an iteration on t starts under: workerSkill,
// unless t's type routes it to its own skill.
func launchSkill(t tickets.Ticket, workerSkill string) string {
	switch {
	case t.IsCodeReview():
		return codeReviewSkill
	case t.IsVerify():
		return verifySkill
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

// ticketAddress is the canonical "project:epic/NN" address of the ticket
// file at ticketPath (<root>/<epic>/issues/NN-<slug>.md), the only form
// iteration prompts hand to agents.
func ticketAddress(t tickets.Ticket) string {
	a, _ := tickets.AddressOfPath(t.Path, t.Identifier)
	return a.String()
}

// SkillPrompt and AgentArgs expose the launch shape to the server's runner.
func SkillPrompt(agent AgentKind, skill, ticketAddr string) string {
	return skillPrompt(agent, skill, ticketAddr)
}

// cwd is the agent's worktree; extraDirs are further writable dirs for a Codex
// sandbox. Claude ignores both.
func AgentArgs(agent AgentKind, cwd, scratchDir, epicName, model, effort string, extraDirs ...string) []string {
	return agentArgs(agent, cwd, scratchDir, epicName, model, effort, extraDirs...)
}

func skillPrompt(agent AgentKind, skill, ticketAddr string) string {
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

func agentArgs(agent AgentKind, cwd, scratchDir, epicName, model, effort string, extraDirs ...string) []string {
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

// outcome is one iteration's or one merge's result.
type outcome struct {
	ticket tickets.Ticket
	err    error
	// built is true when this outcome is a build that finished with commits
	// ready to merge (ahead > 0): iteration_status: finished is already
	// written.
	built bool
	// pausedOnChild is true when this outcome is a merge outcome whose
	// cherry-pick hit errConflictResolutionUnresolved: the ticket was paused
	// (repair) on a conflict-resolution child, not merged — it stays claimed on
	// disk. err is nil on this path (pausing already handled the failure).
	pausedOnChild bool
	// childReason names the conflict-resolution child and why it failed, on a
	// pausedOnChild outcome; the server pauses the parent with it.
	childReason string
	// mergeDeferred is set by mergeOne when the merge lock was held or a human's
	// conflict marker is pending.
	mergeDeferred bool
}

// scanDecisions builds one claimNext pass's scheduler-scan log line: epic's
// tickets, in file order, each labeled with why the scheduler did or didn't
// claim it this pass. claimed is the zero-value Ticket when this pass found
// nothing to claim.
func scanDecisions(epic tickets.Epic, scope RunScope, frontier []tickets.Ticket, claimed tickets.Ticket) []ScanDecision {
	inFrontier := make(map[string]bool, len(frontier))
	for _, t := range frontier {
		inFrontier[t.Path] = true
	}

	decisions := make([]ScanDecision, 0, len(epic.Tickets))
	for _, t := range epic.Tickets {
		isClaimed := claimed.Path != "" && t.Path == claimed.Path
		decisions = append(decisions, TicketVerdict(epic, scope, t, isClaimed, inFrontier[t.Path]))
	}
	return decisions
}

// TicketVerdict is the scheduler's single per-ticket eligibility decision:
// why t was or wasn't claimed this pass, with the reason a scan line reports.
// The first matching rule wins, so the order is the precedence.
func TicketVerdict(epic tickets.Epic, scope RunScope, t tickets.Ticket, claimed, inFrontier bool) ScanDecision {
	status := epic.RenderedStatus(t)
	d := ScanDecision{Ticket: t.Identifier, Status: status.Word()}
	switch {
	case claimed:
		d.Decision = DecisionClaimed
	case !scope.Contains(t, epic):
		d.Decision = DecisionOutOfScope
	case status.Terminal():
		d.Decision = DecisionDone
	case status == tickets.StatusError:
		d.Decision = DecisionError
		d.Reason = cmp.Or(t.ReadErr, t.GraphErr, t.BlockedByErr, "unrecognized status "+t.Status)
	case epic.IsPaused(t):
		d.Decision = DecisionStalled
	case inFrontier:
		d.Decision = DecisionFrontier
	case status == tickets.StatusBlocked:
		d.Decision = DecisionBlocked
		d.Reason = strings.Join(epic.UnresolvedBlockers(t), ", ")
	default:
		d.Decision = DecisionUnclaimed
	}
	return d
}

// AllDone is allDone for the server's runner.
func AllDone(e tickets.Epic) bool { return allDone(e) }

// allDone reports whether every ticket in e is done — the run's one exit
// condition. A stalled ticket (see tickets.Epic.IsPaused) is deliberately not an
// exit condition: the run pauses on it instead, so the agent's question is
// still answerable in its own pane.
func allDone(e tickets.Epic) bool {
	if len(e.Tickets) == 0 {
		return false
	}
	for _, t := range e.Tickets {
		if !e.RenderedStatus(t).Terminal() {
			return false
		}
	}
	return true
}

// loadNamedEpic loads scratchDir and returns the epic named name, or nil if
// no such epic exists.
func loadNamedEpic(scratchDir, name string) (*tickets.Epic, error) {
	epics, err := tickets.Load(scratchDir)
	if err != nil {
		return nil, err
	}
	for i := range epics {
		if epics[i].Name == name {
			return &epics[i], nil
		}
	}
	return nil, nil
}
