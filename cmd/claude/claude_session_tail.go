package claude

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/elentok/gx/transcript"
)

const defaultSessionTailTurns = 3

func newClaudeSessionTailCmd(d Deps) *cobra.Command {
	var turns int

	cmd := &cobra.Command{
		Use:   "session-tail <session-id>",
		Short: "print the last text turns of a Claude Code transcript",
		Long: `Print the last text turns of a Claude Code transcript, one block per turn,
prefixed with "person:" or "agent:". Tool calls and JSONL metadata are left out.`,
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return runClaudeSessionTail(d, args[0], turns)
		},
	}
	cmd.Flags().IntVarP(&turns, "turns", "n", defaultSessionTailTurns, "number of turns to print")
	return cmd
}

// runClaudeSessionTail prints the last n person prompts and agent text
// replies of sessionID's transcript, oldest first.
func runClaudeSessionTail(d Deps, sessionID string, n int) error {
	path, err := findSessionTranscript(d, sessionID)
	if err != nil {
		return err
	}
	msgs, _, err := transcript.ReadChat(path, "")
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}

	var turns []transcript.ChatMessage
	for _, m := range msgs {
		if m.Kind == transcript.ChatKindText || m.Kind == transcript.ChatKindPrompt {
			turns = append(turns, m)
		}
	}
	if n >= 0 && len(turns) > n {
		turns = turns[len(turns)-n:]
	}
	for _, m := range turns {
		fmt.Fprintf(d.Stdout, "%s: %s\n", m.Role, m.Text)
	}
	return nil
}
