package cmd

import (
	"bytes"
	"context"
	"io"
	"time"

	"github.com/spf13/cobra"

	"github.com/elentok/gx/ui/history"
)

func newClaudeCmd(d deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "claude",
		Short: "Claude Code integration (statusline, history, doctor)",
	}
	cmd.AddCommand(newClaudeStatuslineCmd(d))
	cmd.AddCommand(newClaudeHistoryCmd(d))
	cmd.AddCommand(newClaudeSessionPathCmd(d))
	cmd.AddCommand(newClaudeDoctorCmd(d))
	cmd.AddCommand(newClaudeReportLimitsCmd(d))
	return cmd
}

func newClaudeHistoryCmd(d deps) *cobra.Command {
	return &cobra.Command{
		Use:   "history",
		Short: "browse Claude Code session history across all projects",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			home, err := d.dirs().Home()
			if err != nil {
				return err
			}
			return history.Run(home)
		},
	}
}

// statuslineReportTimeout bounds dial plus request, so a slow server never
// holds up Claude Code's statusline.
const statuslineReportTimeout = 500 * time.Millisecond

// reportStatuslineLimits is best effort: the hook never starts the server and
// never retries, so every error is dropped.
func reportStatuslineLimits(d deps, payload []byte) {
	ctx, cancel := context.WithTimeout(context.Background(), statuslineReportTimeout)
	defer cancel()
	_, _, _ = reportLimits(ctx, d.dial(), payload)
}

func newClaudeStatuslineCmd(d deps) *cobra.Command {
	var silent, demo, install bool
	var agentDir string

	cmd := &cobra.Command{
		Use:   "statusline",
		Short: "render Claude Code's statusline from the hook JSON payload on stdin",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if install {
				return installClaudeStatusline(d)
			}
			if demo {
				return runClaudeStatusline(d, silent, demo)
			}
			var payload bytes.Buffer
			d.stdin = io.TeeReader(d.stdin, &payload)
			err := runClaudeStatusline(d, silent, demo)
			// Neither a failed record write nor a failed report changes the
			// line or the exit code.
			if agentDir != "" {
				_ = recordStatuslineUsage(agentDir, payload.Bytes(), time.Now())
			}
			reportStatuslineLimits(d, payload.Bytes())
			return err
		},
	}
	cmd.Flags().StringVar(&agentDir, "agent-dir", "", "also record the payload's rate_limits into this pty agent directory")
	cmd.Flags().BoolVar(&silent, "silent", false, "omit missing/invalid fields instead of rendering an error segment")
	cmd.Flags().BoolVar(&demo, "demo", false, "print sample statusline output instead of reading the hook JSON from stdin")
	cmd.Flags().BoolVar(&install, "install", false, "install this command as Claude Code's statusLine in ~/.claude/settings.json")
	return cmd
}
