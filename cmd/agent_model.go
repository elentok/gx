package cmd

import (
	"context"
	"fmt"
	"io"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/tickets"
	"github.com/spf13/cobra"
)

func newAgentModelCmd(d deps) *cobra.Command {
	return &cobra.Command{
		Use:   "model <light|worker|strongest>",
		Short: "print the model configured for the current agent's tier",
		Long: "Print the model configured for a tier (light, worker or strongest) of the current agent.\n\n" +
			"The agent is found in this order:\n" +
			"  1. " + nativerunner.AgentSessionEnv + ": the harness of that gx server session\n" +
			"  2. " + server.AgentEnv + ": set by ralph-loop to the run's agent\n" +
			"  3. claude\n\n" +
			"The value comes from the config agents.* section with the current directory's project\n" +
			"overrides on top, and falls back to the built-in default. It is printed as written,\n" +
			"without validation. An empty model prints nothing.",
		Args:      cobra.ExactArgs(1),
		ValidArgs: []string{string(ralphloop.TierLight), string(ralphloop.TierWorker), string(ralphloop.TierStrongest)},
		RunE: func(c *cobra.Command, args []string) error {
			return runAgentModel(d, c.OutOrStdout(), args[0])
		},
	}
}

func runAgentModel(d deps, out io.Writer, tierName string) error {
	tier := ralphloop.Tier(tierName)
	switch tier {
	case ralphloop.TierLight, ralphloop.TierWorker, ralphloop.TierStrongest:
	default:
		return fmt.Errorf("unknown tier %q (want light, worker or strongest)", tierName)
	}
	cfg, err := d.loadConfig()
	if err != nil {
		return err
	}
	if cwd, err := d.getwd(); err == nil {
		if projectDir, err := tickets.RootIn(cfg.TicketStore.Path, cwd); err == nil {
			if pf, err := config.ReadProjectFile(projectDir); err == nil {
				cfg = cfg.WithProject(pf)
			}
		}
	}
	model, _ := ralphloop.ResolveTier(cfg.Agents, currentAgent(d), tier)
	if model != "" {
		fmt.Fprintln(out, model)
	}
	return nil
}

// currentAgent is the agent the caller runs under: its gx server session's
// harness, else GX_AGENT, else Claude. A session the server cannot tell us
// about falls through to the next source.
func currentAgent(d deps) ralphloop.AgentKind {
	if id := d.getenv(nativerunner.AgentSessionEnv); id != "" {
		if h, ok := sessionHarness(d, id); ok {
			if h == string(ralphloop.AgentCodex) {
				return ralphloop.AgentCodex
			}
			return ralphloop.AgentClaude
		}
	}
	if d.getenv(server.AgentEnv) == string(ralphloop.AgentCodex) {
		return ralphloop.AgentCodex
	}
	return ralphloop.AgentClaude
}

func sessionHarness(d deps, id string) (string, bool) {
	cl, err := d.dial()()
	if err != nil {
		return "", false
	}
	sessions, err := cl.Sessions(context.Background())
	if err != nil {
		return "", false
	}
	for _, s := range sessions {
		if s.ID == id {
			return s.Harness, true
		}
	}
	return "", false
}
