package schedules

import (
	"fmt"
	"strings"
)

// Prompt is the person's prompt followed by the note a Scheduled run needs:
// nobody watches it, and nothing reaches chat unless the agent runs gx notify.
func Prompt(name, prompt string) string {
	var b strings.Builder
	b.WriteString(strings.TrimSpace(prompt))
	b.WriteString("\n\n---\n")
	fmt.Fprintf(&b, "This run was started by the Schedule %q with no one watching.\n", name)
	b.WriteString("`gx notify \"<message>\"` sends to Telegram and Slack only, and nothing is sent unless you run it.\n")
	fmt.Fprintf(&b, "Start the message with \"%s:\" and keep it short.\n", name)
	b.WriteString("Do not ask questions with `gx tickets ask` unless you cannot continue.\n")
	return b.String()
}
