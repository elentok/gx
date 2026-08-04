package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

const (
	agentClaude = "claude"
	agentCodex  = "codex"

	claudeSessionEnvVar = "CLAUDE_CODE_SESSION_ID"
	codexThreadEnvVar   = "CODEX_THREAD_ID"
)

func newAgentCmd(d deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "agent",
		Short: "agent-neutral inspection of the running coding-agent session",
	}
	cmd.AddCommand(newAgentContextWindowCmd(d))
	return cmd
}

func newAgentContextWindowCmd(d deps) *cobra.Command {
	var agent, session, cwd string

	cmd := &cobra.Command{
		Use:   "context-window",
		Short: "print the active Claude or Codex session's current context token count",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
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
func detectAgentCandidates(d deps, agent, session string) ([]agentCandidate, error) {
	if agent != "" {
		if agent != agentClaude && agent != agentCodex {
			return nil, fmt.Errorf(`invalid --agent %q: want "claude" or "codex"`, agent)
		}
		sessionID := session
		if sessionID == "" {
			sessionID = d.getenv(envVarFor(agent))
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
	if id := d.getenv(claudeSessionEnvVar); id != "" {
		candidates = append(candidates, agentCandidate{agentClaude, id})
	}
	if id := d.getenv(codexThreadEnvVar); id != "" {
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
func runAgentContextWindow(d deps, agent, session, cwdOverride string) (int, error) {
	cwd := cwdOverride
	if cwd == "" {
		var err error
		cwd, err = d.getwd()
		if err != nil {
			return 0, err
		}
	}

	candidates, err := detectAgentCandidates(d, agent, session)
	if err != nil {
		return 0, err
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

func readClaudeContextWindow(d deps, cwd, sessionID string) (int, error) {
	occupancy, ok, err := d.readClaudeOccupancy(cwd, sessionID)
	if err != nil {
		return 0, fmt.Errorf("reading claude session %s: %w", sessionID, err)
	}
	if !ok {
		return 0, fmt.Errorf("no claude transcript found for session %s in %s", sessionID, cwd)
	}
	return occupancy, nil
}

func readCodexContextWindow(d deps, cwd, sessionID string) (int, error) {
	verified, err := d.verifyCodexSession(cwd, sessionID)
	if err != nil {
		return 0, fmt.Errorf("verifying codex session %s: %w", sessionID, err)
	}
	if !verified {
		return 0, fmt.Errorf("no codex session %s found for %s", sessionID, cwd)
	}
	tokens, ok, err := d.readCodexContext(cwd, sessionID)
	if err != nil {
		return 0, fmt.Errorf("reading codex session %s: %w", sessionID, err)
	}
	if !ok {
		return 0, fmt.Errorf("codex session %s has no context usage yet", sessionID)
	}
	return tokens, nil
}
