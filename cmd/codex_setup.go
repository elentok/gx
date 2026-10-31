package cmd

import (
	"github.com/spf13/cobra"

	"github.com/elentok/gx/codexsetup"
	"github.com/elentok/gx/sessionlookup"
)

func newCodexSetupCmd(d deps) *cobra.Command {
	var yes bool
	var codexHome, goplsServer string

	cmd := &cobra.Command{
		Use:   "setup",
		Short: "install Codex approvals for git commands and the gopls MCP tools",
		Long: `Install the Codex approvals gx relies on.

Writes $CODEX_HOME/rules/allow-git-commands.rules (git add/commit/diff/status/log,
git cherry-pick) and auto-approves every tool of the gopls or gopls-mcp MCP
server in $CODEX_HOME/config.toml. Shows the changes first and asks before
writing.`,
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if codexHome == "" {
				codexHome = sessionlookup.RootsFrom(d.env).CodexHome
			}
			return codexsetup.Run(codexsetup.Options{
				CodexHome:   codexHome,
				GoplsServer: goplsServer,
				Yes:         yes,
				Interactive: isTerminal(d),
				In:          d.stdin,
				Out:         d.stdout,
			})
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "write without asking")
	cmd.Flags().StringVar(&codexHome, "codex-home", "", "Codex home directory (default $CODEX_HOME or ~/.codex)")
	cmd.Flags().StringVar(&goplsServer, "gopls-server", "", "MCP server name to approve (default gopls-mcp, else gopls)")
	return cmd
}
