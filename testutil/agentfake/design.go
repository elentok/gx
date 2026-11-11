package agentfake

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// DesignPromptPrefix, at the start of a submitted prompt, makes the TUI fake
// play a gx-design turn: it writes the next scripted assistant message to its
// transcript instead of working.
const DesignPromptPrefix = "/gx-design "

// DesignRunEnd is the closing line of a gx-design run, which the server reads
// as the Continue action.
const DesignRunEnd = "✅ Design: the scripted run is over. Next: continue in a fresh session"

// DesignRoundQuestions are the question ids of the scripted grilling rounds,
// one list per /gx-design prompt in turn; the last round repeats. The first
// round has a lettered id.
var DesignRoundQuestions = [][]string{{"8", "8b"}, {"9"}}

const (
	yesNoBlock   = "```gx-question\ntype: yes-no\nrecommended: yes\nyes: Runs the step now.\nno: Holds the step back.\n```"
	optionsBlock = "```gx-question\ntype: options\nrecommended: a\noptions:\n  - id: a\n    label: SQLite\n    consequence: One file, no server to run.\n  - id: b\n    label: Files\n    consequence: Plain files, easy to diff.\n```"
)

var designRounds = []string{
	"Two questions before the spec.\n\n" +
		"❓ **Q8** - **Which store?**: SQLite or files?\n\n" + optionsBlock + "\n\n---\n\n" +
		"❓ **Q8b** - **Migrate at startup?**: Run the migration when the server starts?\n\n" + yesNoBlock,
	"A fresh round.\n\n" +
		"❓ **Q9** - **Keep going?**: Ready for the next step?\n\n" + yesNoBlock,
}

// The turn kinds a WorkerConfig.Design script names, one per prompt in order.
const (
	// DesignTurnRound is the next scripted grilling round.
	DesignTurnRound = "round"
	// DesignTurnRunEnd is a message that closes the run.
	DesignTurnRunEnd = "run-end"
	// DesignTurnPlain is text with neither a question nor the run-end line.
	DesignTurnPlain = "plain"
	// DesignTurnError is error text, then the run-end line.
	DesignTurnError = "error"
	// DesignTurnDefaulted is a round whose questions are all defaulted: plain
	// text under a heading, and one yes/no card to accept them.
	DesignTurnDefaulted = "defaulted"
	// DesignTurnMixed is a round with one asked question and two defaulted ones.
	DesignTurnMixed = "mixed"
	// DesignTurnText is a round with one free-text question.
	DesignTurnText = "text"
	// DesignTurnSlow is a plain-text turn that stays busy for designSlowTime.
	DesignTurnSlow = "slow"
)

// defaultsSection is the gx-grilling format: after the asked questions, after
// a `---` line, plain text under its own heading.
const defaultsSection = "\n\n---\n\n**Decided unless you object**\n\n" +
	"- Q10: keep the current store.\n- Q11: migrate at startup."

const defaultedRound = "Everything else has a clear answer.\n\n" +
	"❓ **Q12** - **Accept these defaults?**: Say so, or object in the message box.\n\n" + yesNoBlock +
	defaultsSection

const mixedRound = "One question needs you.\n\n" +
	"❓ **Q12** - **Keep going?**: Ready for the next step?\n\n" + yesNoBlock +
	defaultsSection

// A text question carries neither options nor a recommendation: the parser
// rejects both.
const textRound = "One question needs a typed answer.\n\n" +
	"❓ **Q13** - **Name the migration**: What should the migration be called?\n\n" +
	"```gx-question\ntype: text\n```"

// DesignRoundText is the scripted text of a DesignTurnDefaulted,
// DesignTurnMixed or DesignTurnText turn, for tests that check it against the
// server's parser.
func DesignRoundText(turn string) string {
	switch turn {
	case DesignTurnMixed:
		return mixedRound
	case DesignTurnText:
		return textRound
	}
	return defaultedRound
}

// TurnScript is the turn script a prompt plays under. A design prompt naming a
// map in WorkerConfig.DesignByMap pins that map's script for the session.
func (d *DesignState) TurnScript(cwd, text string) []string {
	cfg, err := LoadWorkerConfig(cwd)
	if err != nil {
		return nil
	}
	if IsDesignPrompt(text) {
		for addr, s := range cfg.DesignByMap {
			if strings.Contains(text, addr) {
				d.script = s
			}
		}
	}
	if d.script != nil {
		return d.script
	}
	return cfg.Design
}

func (d *DesignState) Message(text string, script []string) string {
	kind := ""
	if d.Turns < len(script) {
		kind = script[d.Turns]
	} else if IsDesignPrompt(text) {
		kind = DesignTurnRound
	}
	d.Turns++
	switch kind {
	case DesignTurnRound:
		msg := designRounds[min(d.rounds, len(designRounds)-1)]
		d.rounds++
		return msg
	case DesignTurnPlain:
		return "Noted, I am thinking it over."
	case DesignTurnError:
		return "Error: the spec step failed, so I retried it.\n\n" + DesignRunEnd
	case DesignTurnDefaulted:
		return defaultedRound
	case DesignTurnMixed:
		return mixedRound
	case DesignTurnText:
		return textRound
	case DesignTurnSlow:
		return "Working through the tickets."
	}
	return "Thanks, that settles it.\n\n" + DesignRunEnd
}

// ChatSlowMarker, anywhere in a plain prompt of a chat fake, makes the turn
// stay busy with no reply until a test ends it with ActionFinish.
const ChatSlowMarker = "[slow]"

// DesignTurnTime is how long a scripted turn stays busy; DesignSlowTime is how
// long a DesignTurnSlow turn does, enough for a test to report steps meanwhile.
const (
	DesignTurnTime = 500 * time.Millisecond
	DesignSlowTime = 20 * time.Second
)

// appendMessage appends a transcript line of the given role to the file at path.
func AppendMessage(path, role string, content any) error {
	return AppendEntry(path, role, content, nil)
}

// appendEntry is appendMessage with extra top-level fields on the line.
func AppendEntry(path, role string, content any, extra map[string]any) error {
	return WriteEntry(path, role, map[string]any{"role": role, "content": content}, extra)
}

// appendReply appends an assistant line that reports model, as claude does.
func AppendReply(path, model string, content any) error {
	return WriteEntry(path, "assistant", map[string]any{"role": "assistant", "content": content, "model": model}, nil)
}

func WriteEntry(path, role string, message, extra map[string]any) error {
	now := time.Now().UTC()
	entry := map[string]any{
		"type": role,
		// Grilling counts dedupe on the line's uuid, so each line needs its own.
		"uuid":      fmt.Sprintf("fake-%d", now.UnixNano()),
		"timestamp": now.Format(time.RFC3339Nano),
		"message":   message,
	}
	for k, v := range extra {
		entry[k] = v
	}
	return AppendLine(path, entry)
}

// appendLine appends entry as one JSON line to the transcript at path.
func AppendLine(path string, entry map[string]any) error {
	line, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(line, '\n'))
	return err
}

// codexDesignPromptPrefix is the same prompt typed into Codex, which names a
// skill with `$`.
const codexDesignPromptPrefix = "$gx-design "

func IsDesignPrompt(text string) bool {
	return strings.HasPrefix(text, DesignPromptPrefix) || strings.HasPrefix(text, codexDesignPromptPrefix)
}
