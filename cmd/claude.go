package cmd

import (
	"github.com/spf13/cobra"

	"github.com/elentok/gx/ui/history"
)

func newClaudeCmd(d deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "claude",
		Short: "Claude Code integration (statusline, history)",
	}
	cmd.AddCommand(newClaudeStatuslineCmd(d))
	cmd.AddCommand(newClaudeHistoryCmd(d))
	return cmd
}

func newClaudeHistoryCmd(_ deps) *cobra.Command {
	return &cobra.Command{
		Use:   "history",
		Short: "browse Claude Code session history across all projects",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return history.Run()
		},
	}
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
