package harness

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/elentok/gx/agentrunner"
)

// GXSystemPrompt is the text gx appends to the system prompt of every Claude
// loop (ticket) agent, on launch and on resume. skills/gx-local-tracker.md
// carries the same text between its gx-system-prompt markers; a test keeps
// the two equal. The text is one line: herdr refuses launch arguments that
// contain a newline (invalid_agent_argument).
const GXSystemPrompt = "Never use the AskUserQuestion tool. To ask a person, run `gx tickets ask`. Do these steps in order. 1. Commit what is green. Work already done is not discarded while the question waits. 2. Run `gx tickets ask <addr> --body -` with the body on stdin. The body has two sections. `## Question`: a one-line plain-text summary first (no markdown, at most 120 characters), then optional detail. A person should be able to answer in one pass without reading the diff. When the answer is yes or no, or a pick from 2 to 6 choices, end the detail with a fenced ```gx-question block (YAML: `type: yes-no` with optional `yes:` and `no:` consequences, or `type: options` with `options:` entries of `id` and `label`; optional `recommended:`), so the person gets buttons instead of a text box. `## Handoff`: required and non-empty. Name the files you touched, the decisions you made and why, and what you do with each answer. 3. End your turn. The agent keeps its worktree, pane and slot while it waits. After `gx tickets ask`, end your turn. The person's reply reaches you as a new message, in this session or in a fresh one that starts from your handoff. If the reply leaves anything unclear, or you need more, run `gx tickets ask` again. Never end a turn with a question in plain text and no `ask` call. No one is told, and the ticket stalls."

// GXSystemPromptFor is the prompt for one harness: Claude's text as is, Codex's
// with its own name for the question tool.
func GXSystemPromptFor(kind agentrunner.Kind) string {
	if kind == Codex {
		return strings.Replace(GXSystemPrompt, "AskUserQuestion", CodexQuestionTool, 1)
	}
	return GXSystemPrompt
}

// tomlString renders s as a TOML basic string: backslash, double quote and
// control characters escaped, everything else (non-ASCII too) kept as is.
func tomlString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '\\' || r == '"':
			b.WriteByte('\\')
			b.WriteRune(r)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\t':
			b.WriteString(`\t`)
		case r == '\r':
			b.WriteString(`\r`)
		case r < 0x20 || r == 0x7f:
			fmt.Fprintf(&b, `\u%04X`, r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// WithGXSystemPrompt appends --append-system-prompt GXSystemPrompt to claude
// args unless args already carry the flag. Only loop launches call it: the
// Agents tab and gx-design sessions go through Launch without it.
func WithGXSystemPrompt(args []string) []string {
	if slices.Contains(args, "--append-system-prompt") {
		return args
	}
	return append(args, "--append-system-prompt", GXSystemPrompt)
}

// GXSystemPromptFile writes GXSystemPrompt to a file in the user cache dir and
// returns its path, for claude's --append-system-prompt-file. A herdr launch
// types the whole command line into a shell, so a file keeps that line short.
// It returns "" when the file cannot be written; callers then pass the text
// inline.
func GXSystemPromptFile() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	dir = filepath.Join(dir, "gx")
	path := filepath.Join(dir, "system-prompt.md")
	if cur, err := os.ReadFile(path); err == nil && string(cur) == GXSystemPrompt {
		return path
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return ""
	}
	tmp, err := os.CreateTemp(dir, "system-prompt-*.tmp")
	if err != nil {
		return ""
	}
	_, werr := tmp.WriteString(GXSystemPrompt)
	cerr := tmp.Close()
	if werr != nil || cerr != nil || os.Rename(tmp.Name(), path) != nil {
		os.Remove(tmp.Name())
		return ""
	}
	return path
}
