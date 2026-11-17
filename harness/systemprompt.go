package harness

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/elentok/gx/agentrunner"
)

// GXAskText is the ask part of the loop prompt, shared by Claude and Codex.
// One line: a launch argument that contains a newline is refused
// (invalid_agent_argument).
const GXAskText = "Never use the AskUserQuestion tool. To ask a person, run `gx tickets ask`. Do these steps in order. 1. Commit what is green. Work already done is not discarded while the question waits. 2. Run `gx tickets ask <addr> --body -` with the body on stdin. The body has two sections. `## Question`: a one-line plain-text summary first (no markdown, at most 120 characters), then optional detail. A person should be able to answer in one pass without reading the diff. When the answer is yes or no, or a pick from 2 to 6 choices, end the detail with a fenced ```gx-question block (YAML: `type: yes-no` with optional `yes:` and `no:` consequences, or `type: options` with `options:` entries of `id` and `label`; optional `recommended:`), so the person gets buttons instead of a text box. `## Handoff`: required and non-empty. Name the files you touched, the decisions you made and why, and what you do with each answer. 3. End your turn. The agent keeps its worktree, pane and slot while it waits. After `gx tickets ask`, end your turn. The person's reply reaches you as a new message, in this session or in a fresh one that starts from your handoff. If the reply leaves anything unclear, or you need more, run `gx tickets ask` again. Never end a turn with a question in plain text and no `ask` call. No one is told, and the ticket stalls."

// GXWaitSentence is Claude only: Claude gets a completion notice when a
// background task ends. Codex gets none, so the line would stall it.
const GXWaitSentence = "After you start a background task, wait for its completion notice. Do not poll the task and do not call `ListAgents` in a loop."

// GXSystemPrompt is the text gx appends to the system prompt of every Claude
// loop (ticket) agent, on launch and on resume. skills/gx-local-tracker.md
// carries the same text between its gx-system-prompt markers; a test keeps
// the two equal.
const GXSystemPrompt = GXAskText + " " + GXWaitSentence

// GXFilesSentence tells an agent how to show the person a file. One line, the
// same for Claude and Codex.
const GXFilesSentence = "To show the person a file you made (Markdown, HTML, a Mermaid diagram, a mock), never start your own HTTP server. Run `gx files url <path>` and send that link. gx hosts the file behind its web login. If the command refuses the path, save the file in the repo or a temp folder and run it again."

// GXLoopPrompt is what loop agents receive: the ask text, then the files sentence.
const GXLoopPrompt = GXSystemPrompt + " " + GXFilesSentence

// GXSystemPromptFor is the loop prompt for one harness: Claude's text as is,
// Codex's with its own name for the question tool in the ask part.
func GXSystemPromptFor(kind agentrunner.Kind) string {
	if kind == Codex {
		return strings.Replace(GXAskText, "AskUserQuestion", CodexQuestionTool, 1) + " " + GXFilesSentence
	}
	return GXLoopPrompt
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

// WithGXSystemPrompt appends --append-system-prompt GXLoopPrompt to claude
// args unless args already carry either prompt flag. Only loop launches call
// it; other sessions get WithGXFilesPrompt.
func WithGXSystemPrompt(args []string) []string {
	if hasPromptFlag(args) {
		return args
	}
	return append(args, "--append-system-prompt", GXLoopPrompt)
}

// GXNoteSentence is the one line that sends the agent to its launch note file.
func GXNoteSentence(noteFile string) string {
	return "Before the skill's first step, read `" + noteFile + "` and treat it as part of your launch prompt."
}

// GXLoopPromptWithNote is GXLoopPrompt plus GXNoteSentence.
func GXLoopPromptWithNote(noteFile string) string {
	return GXLoopPrompt + " " + GXNoteSentence(noteFile)
}

// WithPromptFile points claude args at file as the one system-prompt file,
// replacing whichever prompt flag they carry. Args with none get the flag.
func WithPromptFile(args []string, file string) []string {
	out := slices.Clone(args)
	for i, a := range out {
		switch a {
		case "--append-system-prompt-file":
			if i+1 < len(out) {
				out[i+1] = file
				return out
			}
		case "--append-system-prompt":
			if i+1 < len(out) {
				out[i], out[i+1] = "--append-system-prompt-file", file
				return out
			}
		}
	}
	return append(out, "--append-system-prompt-file", file)
}

func hasPromptFlag(args []string) bool {
	return slices.Contains(args, "--append-system-prompt") || slices.Contains(args, "--append-system-prompt-file")
}

// GXSystemPromptFile writes GXLoopPrompt to a file in the user cache dir and
// returns its path, for claude's --append-system-prompt-file. A launch that
// types the whole command line into a shell, so a file keeps that line short.
// It returns "" when the file cannot be written; callers then pass the text
// inline.
func GXSystemPromptFile() string {
	return promptFile("system-prompt", GXLoopPrompt)
}

// GXFilesPromptFile is GXSystemPromptFile for the files sentence alone, which
// every non-loop Claude session gets. It is a separate file: the loop's holds
// both texts.
func GXFilesPromptFile() string {
	return promptFile("files-prompt", GXFilesSentence)
}

// WithGXFilesPrompt appends the files sentence to claude args, through file
// (from GXFilesPromptFile) or inline when file is "". It skips args that
// already carry either prompt flag.
func WithGXFilesPrompt(args []string, file string) []string {
	if hasPromptFlag(args) {
		return args
	}
	if file == "" {
		return append(args, "--append-system-prompt", GXFilesSentence)
	}
	return append(args, "--append-system-prompt-file", file)
}

func promptFile(name, text string) string {
	dir, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	dir = filepath.Join(dir, "gx")
	path := filepath.Join(dir, name+".md")
	if cur, err := os.ReadFile(path); err == nil && string(cur) == text {
		return path
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return ""
	}
	tmp, err := os.CreateTemp(dir, name+"-*.tmp")
	if err != nil {
		return ""
	}
	_, werr := tmp.WriteString(text)
	cerr := tmp.Close()
	if werr != nil || cerr != nil || os.Rename(tmp.Name(), path) != nil {
		os.Remove(tmp.Name())
		return ""
	}
	return path
}
