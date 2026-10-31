package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
)

// guardBashExitCode is the exit code a Claude Code PreToolUse hook uses to
// block the tool call and feed stderr back to the agent.
const guardBashExitCode = 2

var (
	sedInPlace   = regexp.MustCompile(`\bsed\b[^|;&\n]*\s(-i|-[a-zA-Z]*i[a-zA-Z]*|--in-place)(\s|=|$)`)
	heredoc      = regexp.MustCompile(`<<-?\s*['"]?[A-Za-z_]`)
	fileRedirect = regexp.MustCompile(`(^|[^<>&0-9])>>?\s*[^\s>&]`)
	devNull      = regexp.MustCompile(`>>?\s*/dev/null`)
	scriptHeredc = regexp.MustCompile(`\b(python[0-9.]*|perl|ruby|node)\b[^\n]*<<`)
	heredocStart = regexp.MustCompile(`<<-?\s*['"]?([A-Za-z_][A-Za-z0-9_]*)`)
)

// withoutHeredocBodies drops the lines a heredoc feeds to stdin, keeping the
// command lines. A ">" in a body (a ticket's prose, say) is not a redirect.
func withoutHeredocBodies(command string) string {
	var kept []string
	delim := ""
	for _, line := range strings.Split(command, "\n") {
		if delim != "" {
			if strings.TrimSpace(line) == delim {
				delim = ""
			}
			continue
		}
		kept = append(kept, line)
		if m := heredocStart.FindStringSubmatch(line); m != nil {
			delim = m[1]
		}
	}
	return strings.Join(kept, "\n")
}

// withoutQuotedText empties quoted strings, keeping the quotes. A ">" inside
// one (a commit trailer ending in `<noreply@x.com>"`) is not a redirect, while
// `> "file"` still reads as one. A quote that never closes (an apostrophe in
// prose) makes the pairing unreliable, so the command comes back unstripped and
// the redirect check fails closed.
func withoutQuotedText(command string) string {
	var out strings.Builder
	var open rune
	for _, r := range command {
		switch {
		case open == 0 && (r == '"' || r == '\''):
			open = r
			out.WriteRune(r)
		case open != 0 && r == open:
			open = 0
			out.WriteRune(r)
		case open == 0:
			out.WriteRune(r)
		}
	}
	if open != 0 {
		return command
	}
	return out.String()
}

// guardBashCommand returns why command must not run, or "" when it may.
// It blocks the ways agents edit files that bypass Edit's staleness check:
// sed -i, heredocs redirected into a file, and interpreter heredoc scripts.
func guardBashCommand(command string) string {
	return guardBashReason(command, "the Edit or Write tool")
}

// guardBashReason is guardBashCommand with the harness's edit tool named in the hint.
func guardBashReason(command, editTool string) string {
	hint := "Use " + editTool + " instead: it fails loudly on a stale read, a shell edit does not."
	switch {
	case sedInPlace.MatchString(command):
		return "sed -i edits files behind Edit's back. " + hint
	case scriptHeredc.MatchString(command):
		return "A heredoc script that rewrites files is invisible when it hangs. " + hint
	case heredoc.MatchString(command) && fileRedirect.MatchString(withoutQuotedText(devNull.ReplaceAllString(withoutHeredocBodies(command), ""))):
		return "A heredoc written into a file skips imports and indentation checks. " + hint
	}
	return ""
}

func newAgentGuardBashCmd() *cobra.Command {
	var harnessName string
	cmd := &cobra.Command{
		Use:   "guard-bash",
		Short: "PreToolUse hook: reject shell file edits (sed -i, heredoc writes) in ralph-loop agent sessions",
		Args:  cobra.NoArgs,
		// Quiet so a rejection prints only the reason the agent should read.
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(c *cobra.Command, _ []string) error {
			editTool := "the Edit or Write tool"
			if harnessName == agentCodex {
				editTool = "apply_patch"
			}
			raw, err := io.ReadAll(c.InOrStdin())
			if err != nil {
				return err
			}
			var in struct {
				ToolInput struct {
					Command string `json:"command"`
				} `json:"tool_input"`
			}
			// A payload we can't read must not block the agent.
			if json.Unmarshal(raw, &in) != nil {
				return nil
			}
			reason := guardBashReason(strings.TrimSpace(in.ToolInput.Command), editTool)
			if reason == "" {
				return nil
			}
			fmt.Fprintln(c.ErrOrStderr(), reason)
			return &ExitError{Code: guardBashExitCode}
		},
	}
	cmd.Flags().StringVar(&harnessName, "harness", agentClaude, `the harness running the hook: "claude" or "codex"`)
	return cmd
}

// newAgentGuardQuestionCmd is the Codex PreToolUse hook for request_user_input.
// A hook cannot tell Plan mode from any other, so it always blocks.
func newAgentGuardQuestionCmd() *cobra.Command {
	return &cobra.Command{
		Use:           "guard-question",
		Short:         "PreToolUse hook: reject Codex's request_user_input in favour of gx tickets ask",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(c *cobra.Command, _ []string) error {
			fmt.Fprintln(c.ErrOrStderr(), "Do not use request_user_input. To ask a person, run `gx tickets ask <addr> --body -` (a `## Question` and a `## Handoff` section on stdin), then end your turn.")
			return &ExitError{Code: guardBashExitCode}
		},
	}
}
