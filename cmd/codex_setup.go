package cmd

import (
	"github.com/spf13/cobra"

	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/codexsetup"
	"github.com/elentok/gx/sessionlookup"
)

func newCodexSetupCmd(d deps) *cobra.Command {
	var yes bool
	var codexHome string

	cmd := &cobra.Command{
		Use:   "setup",
		Short: "install Codex approvals for git commands",
		Long: `Install the Codex approvals gx relies on.

Writes $CODEX_HOME/rules/allow-git-commands.rules (git add/commit/diff/status/log,
git cherry-pick). Shows the changes first and asks before writing.`,
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if codexHome == "" {
				codexHome = sessionlookup.RootsFrom(d.env).CodexHome
			}
			return codexsetup.Run(codexsetup.Options{
				CodexHome:   codexHome,
				Yes:         yes,
				Interactive: cmdbase.IsTerminal(d.stdinIsTerminal),
				In:          d.stdin,
				Out:         d.stdout,
			})
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "write without asking")
	cmd.Flags().StringVar(&codexHome, "codex-home", "", "Codex home directory (default $CODEX_HOME or ~/.codex)")
	return cmd
}
