package agentfake

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/elentok/gx/transcript"
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
	yesNoBlock   = "```gx-question\ntype: yes-no\nrecommended: yes\n```"
	optionsBlock = "```gx-question\ntype: options\nrecommended: a\noptions:\n  - id: a\n    label: SQLite\n  - id: b\n    label: Files\n```"
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

// DesignRoundText is the scripted text of a DesignTurnDefaulted or
// DesignTurnMixed turn, for tests that check it against the server's parser.
func DesignRoundText(turn string) string {
	if turn == DesignTurnMixed {
		return mixedRound
	}
	return defaultedRound
}

// designMessage is the assistant message for the turn-th prompt (from zero).
// Without a script a /gx-design prompt gets the next round and any other text
// the run-end message; a script that has run out falls back to the same.
func (t *tui) designMessage(text string, script []string) string {
	kind := ""
	if t.design.turns < len(script) {
		kind = script[t.design.turns]
	} else if isDesignPrompt(text) {
		kind = DesignTurnRound
	}
	t.design.turns++
	switch kind {
	case DesignTurnRound:
		msg := designRounds[min(t.design.rounds, len(designRounds)-1)]
		t.design.rounds++
		return msg
	case DesignTurnPlain:
		return "Noted, I am thinking it over."
	case DesignTurnError:
		return "Error: the spec step failed, so I retried it.\n\n" + DesignRunEnd
	case DesignTurnDefaulted:
		return defaultedRound
	case DesignTurnMixed:
		return mixedRound
	case DesignTurnSlow:
		return "Working through the tickets."
	}
	return "Thanks, that settles it.\n\n" + DesignRunEnd
}

// designTurn plays one turn, picking its message with designMessage. The
// message lands in the transcript while the turn is busy, and the turn ends
// shortly after so the server sees the busy-to-idle change that raises the
// Actionable.
func (t *tui) designTurn(text string) error {
	t.prompt = text
	var script []string
	if cfg, err := LoadWorkerConfig(t.pid.Cwd); err == nil {
		script = cfg.Design
		if isDesignPrompt(text) {
			for addr, s := range cfg.DesignByMap {
				if strings.Contains(text, addr) {
					t.design.script = s
				}
			}
		}
		if t.design.script != nil {
			script = t.design.script
		}
	}
	if isDesignPrompt(text) {
		t.design.active = true
	}
	busy := designTurnTime
	if t.design.turns < len(script) && script[t.design.turns] == DesignTurnSlow {
		busy = designSlowTime
	}
	msg := t.designMessage(text, script)
	// Busy comes first: a message that appears while the session still reads
	// idle lets a test prompt it before the turn has started, and that prompt is lost.
	if err := t.setStatus(StatusBusy, ""); err != nil {
		return err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	path := transcript.PathIn(home, t.pid.Cwd, t.pid.SessionID)
	// An answer is a person message in the transcript, as in claude: it ends
	// the round the answer is for.
	if !isDesignPrompt(text) {
		if err := appendMessage(path, "user", text); err != nil {
			return err
		}
	}
	if err := appendMessage(path, "assistant", []map[string]any{{"type": "text", "text": msg}}); err != nil {
		return err
	}
	go func() {
		time.Sleep(busy)
		_ = SendControl(t.ctlPath, Control{Action: ActionFinish})
	}()
	return nil
}

// designTurnTime is how long a scripted turn stays busy; designSlowTime is how
// long a DesignTurnSlow turn does, enough for a test to report steps meanwhile.
const (
	designTurnTime = 500 * time.Millisecond
	designSlowTime = 20 * time.Second
)

// appendMessage appends a transcript line of the given role to the file at path.
func appendMessage(path, role string, content any) error {
	now := time.Now().UTC()
	line, err := json.Marshal(map[string]any{
		"type": role,
		// Grilling counts dedupe on the line's uuid, so each line needs its own.
		"uuid":      fmt.Sprintf("fake-%d", now.UnixNano()),
		"timestamp": now.Format(time.RFC3339Nano),
		"message":   map[string]any{"role": role, "content": content},
	})
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

func isDesignPrompt(text string) bool { return strings.HasPrefix(text, DesignPromptPrefix) }
