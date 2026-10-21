package harness

import "slices"

// GXSystemPrompt is the text gx appends to the system prompt of every Claude
// loop (ticket) agent, on launch and on resume. skills/gx-local-tracker.md
// carries the same text between its gx-system-prompt markers; a test keeps
// the two equal. The text is one line: herdr refuses launch arguments that
// contain a newline (invalid_agent_argument).
const GXSystemPrompt = "Never use the AskUserQuestion tool. To ask a person, run `gx tickets ask`. Do these steps in order. 1. Commit what is green. Work already done is not discarded while the question waits. 2. Run `gx tickets ask <addr> --body -` with the body on stdin. The body has two sections. `## Question`: a one-line plain-text summary first (no markdown, at most 120 characters), then optional detail. A person should be able to answer in one pass without reading the diff. `## Handoff`: required and non-empty. Name the files you touched, the decisions you made and why, and what you do with each answer. 3. End your turn. The agent keeps its worktree, pane and slot while it waits. After `gx tickets ask`, end your turn. The person's reply reaches you as a new message, in this session or in a fresh one that starts from your handoff. If the reply leaves anything unclear, or you need more, run `gx tickets ask` again. Never end a turn with a question in plain text and no `ask` call. No one is told, and the ticket stalls."

// WithGXSystemPrompt appends --append-system-prompt GXSystemPrompt to claude
// args unless args already carry the flag. Only loop launches call it: the
// Agents tab and gx-design sessions go through Launch without it.
func WithGXSystemPrompt(args []string) []string {
	if slices.Contains(args, "--append-system-prompt") {
		return args
	}
	return append(args, "--append-system-prompt", GXSystemPrompt)
}
