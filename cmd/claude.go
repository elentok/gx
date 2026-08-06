package cmd

import (
	"github.com/spf13/cobra"
)

func newClaudeCmd(d deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "claude",
		Short: "Claude Code integration (statusline, history)",
	}
	cmd.AddCommand(newClaudeStatuslineCmd(d))
	return cmd
}

func newClaudeStatuslineCmd(d deps) *cobra.Command {
	var silent, demo, install bool

	cmd := &cobra.Command{
		Use:   "statusline",
		Short: "render Claude Code's statusline from the hook JSON payload on stdin",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if install {
				return installClaudeStatusline(d)
			}
			return runClaudeStatusline(d, silent, demo)
		},
	}
	cmd.Flags().BoolVar(&silent, "silent", false, "omit missing/invalid fields instead of rendering an error segment")
	cmd.Flags().BoolVar(&demo, "demo", false, "print sample statusline output instead of reading the hook JSON from stdin")
	cmd.Flags().BoolVar(&install, "install", false, "install this command as Claude Code's statusLine in ~/.claude/settings.json")
	return cmd
}
