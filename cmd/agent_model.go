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
		Short: "print the model (and Codex effort) configured for the current agent's tier",
		Long: "Print the model configured for a tier (light, worker or strongest) of the current agent.\n" +
			"Claude prints the model alone. Codex prints \"model effort\" on one line, or the model\n" +
			"alone when its effort is empty.\n\n" +
			"The agent is found in this order:\n" +
			"  1. " + nativerunner.AgentSessionEnv + ": the harness of that gx server session\n" +
			"  2. " + server.AgentEnv + ": set by ralph-loop to the run's agent\n" +
			"  3. claude\n\n" +
			"The value comes from the config agents.* section with the current directory's project\n" +
			"overrides on top, and falls back to the built-in default. It is printed as written,\n" +
			"without validation. An empty model prints nothing, even when an effort is set.",
		Args:      cobra.ExactArgs(1),
		ValidArgs: tierNames(),
		RunE: func(c *cobra.Command, args []string) error {
			return runAgentModel(d, c.OutOrStdout(), args[0])
		},
	}
}

func tierNames() []string {
	names := make([]string, len(ralphloop.AllTiers))
	for i, t := range ralphloop.AllTiers {
		names[i] = string(t)
	}
	return names
}

func runAgentModel(d deps, out io.Writer, tierName string) error {
	tier := ralphloop.Tier(tierName)
	if !tier.Valid() {
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
	agent := currentAgent(d)
	model, effort := ralphloop.ResolveTier(cfg.Agents, agent, tier)
	switch {
	case model == "":
	case effort == "" || agent != ralphloop.AgentCodex:
		fmt.Fprintln(out, model)
	default:
		fmt.Fprintln(out, model, effort)
	}
	return nil
}

// currentAgent is the agent the caller runs under: its gx server session's
// harness, else GX_AGENT, else Claude. A session the server cannot tell us
// about falls through to the next source.
func currentAgent(d deps) ralphloop.AgentKind {
	if id := d.getenv(nativerunner.AgentSessionEnv); id != "" {
		if h, ok := sessionHarness(d, id); ok {
			return ralphloop.AgentFor(h)
		}
	}
	return ralphloop.AgentFor(d.getenv(server.AgentEnv))
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
