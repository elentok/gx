package agent

import (
	"errors"
	"fmt"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/codexsession"
	"github.com/elentok/gx/config"
	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/nativerunner/holder"
	"github.com/elentok/gx/sessionlookup"
	"github.com/elentok/gx/transcript"
	"github.com/elentok/gx/ui/nav"
	"github.com/spf13/cobra"
)

const (
	agentClaude = "claude"
	agentCodex  = "codex"

	claudeSessionEnvVar = "CLAUDE_CODE_SESSION_ID"
	codexThreadEnvVar   = "CODEX_THREAD_ID"
)

// Deps is the runtime context of every command in this package.
type Deps struct {
	cmdbase.Env
	// OpenTab opens the TUI on a tab; root passes its own runner so this
	// package does not import the TUI.
	OpenTab             func(nav.TabID) error
	ReadClaudeOccupancy func(cwd, sessionID string) (occupancy int, ok bool, err error)
	VerifyCodexSession  func(cwd, sessionID string) (ok bool, err error)
	ReadCodexContext    func(cwd, sessionID string) (tokens int, ok bool, err error)
	// ReadAutoAdvance reads a gx session's auto-advance switch from the server.
	ReadAutoAdvance func(sessionID string) (bool, error)
	// ReadSessionHarness reads a gx session's harness name from the server.
	ReadSessionHarness func(sessionID string) (string, error)
}

// DefaultDeps wires the production hooks. Env and OpenTab are the caller's.
func DefaultDeps(dirs config.Dirs, roots sessionlookup.Roots) Deps {
	return Deps{
		ReadClaudeOccupancy: func(cwd, sessionID string) (int, bool, error) {
			return transcript.LastAssistantOccupancyIn(roots.Home, cwd, sessionID)
		},
		VerifyCodexSession: func(cwd, sessionID string) (bool, error) {
			return codexsession.VerifyIdentityIn(roots.CodexHome, cwd, sessionID)
		},
		ReadCodexContext: func(cwd, sessionID string) (int, bool, error) {
			return codexsession.LastContextTokensIn(roots.CodexHome, cwd, sessionID)
		},
		ReadAutoAdvance:    func(sessionID string) (bool, error) { return readServerAutoAdvance(dirs, sessionID) },
		ReadSessionHarness: func(sessionID string) (string, error) { return readServerHarness(dirs, sessionID) },
	}
}

// New builds `gx agent`.
func New(d Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "agent",
		Short: "agent-neutral inspection of the running coding-agent session",
	}
	cmd.AddCommand(newAgentContextWindowCmd(d), newAgentActionableCmd(d), newAgentLinkCmd(d), newAgentTranscriptCmd(d), newAgentStepCmd(d), newAgentGuardBashCmd(), newAgentGuardQuestionCmd(), newAgentPermissionRequestCmd(), newAgentModelCmd(d))
	return cmd
}

// NewHolder builds the hidden `gx agent-holder`.
func NewHolder() *cobra.Command {
	var dir, kind string
	var opts holder.HolderOptions
	cmd := &cobra.Command{
		Use:   "agent-holder --session-id <uuid> [--harness <name>] [--gx-session <id>] [--cols N --rows N] [--trust] --dir <agent-dir> -- <cmd...>",
		Short: "run a pty agent and record its output (started by the pty runner and the server)",
		// Internal helper; the pty runner launches it under setsid.
		Hidden: true,
		Args:   cobra.MinimumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return holder.RunHarnessHolder(dir, agentrunner.Kind(kind), opts, args)
		},
	}
	cmd.Flags().IntVar(&opts.Cols, "cols", 0, "the pty's width (default the loop's)")
	cmd.Flags().IntVar(&opts.Rows, "rows", 0, "the pty's height (default the loop's)")
	// The id also sits in the holder's cmdline, which the liveness check requires.
	cmd.Flags().StringVar(&opts.SessionID, "session-id", "", "the agent's session id")
	cmd.Flags().StringVar(&kind, "harness", string(harness.Claude), "the harness that watches the agent")
	cmd.Flags().StringVar(&opts.GXSession, "gx-session", "", "the gx session id, set as "+holder.AgentSessionEnv+" in the agent's env")
	cmd.Flags().StringVar(&dir, "dir", "", "the agent directory")
	cmd.Flags().BoolVar(&opts.Trust, "trust", false, "answer the harness's trust dialog")
	_ = cmd.MarkFlagRequired("session-id")
	_ = cmd.MarkFlagRequired("dir")
	return cmd
}

func newAgentContextWindowCmd(d Deps) *cobra.Command {
	var agent, session, cwd, fits string

	cmd := &cobra.Command{
		Use:   "context-window",
		Short: "print the active Claude or Codex session's current context token count",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			if fits != "" {
				return runAgentFits(d, c.OutOrStdout(), agent, session, cwd, fits)
			}
			tokens, err := runAgentContextWindow(d, agent, session, cwd)
			if err != nil {
				return err
			}
			fmt.Fprintln(c.OutOrStdout(), tokens)
			return nil
		},
	}
	cmd.Flags().StringVar(&agent, "agent", "", `override auto-detection: "claude" or "codex"`)
	cmd.Flags().StringVar(&session, "session", "", "override the session id (requires --agent; skips env-var detection)")
	cmd.Flags().StringVar(&cwd, "cwd", "", "override the working directory used for identity matching")
	cmd.Flags().StringVar(&fits, "fits", "", "design step to chain into: exit 0 if the context has room for it, 1 if not")
	return cmd
}

// agentCandidate is one provider that appears to have a session active, per
// the env vars gx knows how to read (or the --agent/--session overrides).
type agentCandidate struct {
	agent     string
	sessionID string
}

// detectAgentCandidates finds which provider(s) look active. With --agent
// set, only that provider is considered (its session id from --session or
// its own env var); otherwise every provider whose env var is non-empty is a
// candidate, so the caller can tell "none", "one", and "both" apart.
func detectAgentCandidates(d Deps, agent, session string) ([]agentCandidate, error) {
	if agent != "" {
		if agent != agentClaude && agent != agentCodex {
			return nil, fmt.Errorf(`invalid --agent %q: want "claude" or "codex"`, agent)
		}
		sessionID := session
		if sessionID == "" {
			sessionID = d.Getenv(envVarFor(agent))
		}
		if sessionID == "" {
			return nil, fmt.Errorf("no session id for --agent %s: pass --session or set %s", agent, envVarFor(agent))
		}
		return []agentCandidate{{agent, sessionID}}, nil
	}

	if session != "" {
		return nil, fmt.Errorf("--session requires --agent")
	}

	var candidates []agentCandidate
	if id := d.Getenv(claudeSessionEnvVar); id != "" {
		candidates = append(candidates, agentCandidate{agentClaude, id})
	}
	if id := d.Getenv(codexThreadEnvVar); id != "" {
		candidates = append(candidates, agentCandidate{agentCodex, id})
	}
	return candidates, nil
}

func envVarFor(agent string) string {
	if agent == agentCodex {
		return codexThreadEnvVar
	}
	return claudeSessionEnvVar
}

// runAgentContextWindow resolves the active agent session (auto-detected via
// env vars, or pinned via the --agent/--session/--cwd overrides), verifies
// it belongs to cwd, and returns its current context-occupancy token count.
func runAgentContextWindow(d Deps, agent, session, cwdOverride string) (int, error) {
	cwd := cwdOverride
	if cwd == "" {
		var err error
		cwd, err = d.Getwd()
		if err != nil {
			return 0, err
		}
	}

	candidates, err := detectAgentCandidates(d, agent, session)
	if err != nil {
		return 0, err
	}
	if len(candidates) > 1 {
		candidates, err = pickByGXSession(d, candidates)
		if err != nil {
			return 0, err
		}
	}
	switch len(candidates) {
	case 0:
		return 0, fmt.Errorf("no agent session detected: set %s or %s, or pass --agent/--session", claudeSessionEnvVar, codexThreadEnvVar)
	case 1:
		// fall through
	default:
		return 0, fmt.Errorf("ambiguous agent session: both %s and %s are set; pass --agent to disambiguate", claudeSessionEnvVar, codexThreadEnvVar)
	}

	c := candidates[0]
	switch c.agent {
	case agentClaude:
		return readClaudeContextWindow(d, cwd, c.sessionID)
	default:
		return readCodexContextWindow(d, cwd, c.sessionID)
	}
}

// errCodexNoUsage marks a verified Codex session with no token_count yet;
// only --fits maps it to 0 used tokens.
var errCodexNoUsage = errors.New("no context usage yet")

// pickByGXSession settles both-variables-set by the harness of the gx session
// the command runs in. Without GX_AGENT_SESSION it keeps the ambiguity.
func pickByGXSession(d Deps, candidates []agentCandidate) ([]agentCandidate, error) {
	id := d.Getenv(holder.AgentSessionEnv)
	if id == "" || d.ReadSessionHarness == nil {
		return candidates, nil
	}
	name, err := d.ReadSessionHarness(id)
	if err != nil {
		return nil, err
	}
	want := agentClaude
	if name == string(harness.Codex) {
		want = agentCodex
	}
	for _, c := range candidates {
		if c.agent == want {
			return []agentCandidate{c}, nil
		}
	}
	return candidates, nil
}

func readClaudeContextWindow(d Deps, cwd, sessionID string) (int, error) {
	occupancy, ok, err := d.ReadClaudeOccupancy(cwd, sessionID)
	if err != nil {
		return 0, fmt.Errorf("reading claude session %s: %w", sessionID, err)
	}
	if !ok {
		return 0, fmt.Errorf("no claude transcript found for session %s in %s", sessionID, cwd)
	}
	return occupancy, nil
}

func readCodexContextWindow(d Deps, cwd, sessionID string) (int, error) {
	verified, err := d.VerifyCodexSession(cwd, sessionID)
	if err != nil {
		return 0, fmt.Errorf("verifying codex session %s: %w", sessionID, err)
	}
	if !verified {
		return 0, fmt.Errorf("no codex session %s found for %s", sessionID, cwd)
	}
	tokens, ok, err := d.ReadCodexContext(cwd, sessionID)
	if err != nil {
		return 0, fmt.Errorf("reading codex session %s: %w", sessionID, err)
	}
	if !ok {
		return 0, fmt.Errorf("codex session %s has no context usage yet: %w", sessionID, errCodexNoUsage)
	}
	return tokens, nil
}
