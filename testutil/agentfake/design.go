package agentfake

import (
	"encoding/json"
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

// designTurn plays one turn: a /gx-design prompt gets the next scripted round,
// and any other text (the person's answer) gets the run-end message. The
// message lands in the transcript before the turn starts, and the turn ends
// shortly after so the server sees the busy-to-idle change that raises the
// Actionable.
func (t *tui) designTurn(text string) error {
	t.prompt = text
	msg := "Thanks, that settles it.\n\n" + DesignRunEnd
	if isDesignPrompt(text) {
		t.design.active = true
		msg = designRounds[min(t.design.rounds, len(designRounds)-1)]
		t.design.rounds++
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
		time.Sleep(designTurnTime)
		_ = SendControl(t.ctlPath, Control{Action: ActionFinish})
	}()
	return t.setStatus(StatusBusy, "")
}

// designTurnTime is how long a scripted turn stays busy.
const designTurnTime = 500 * time.Millisecond

// appendMessage appends a transcript line of the given role to the file at path.
func appendMessage(path, role string, content any) error {
	line, err := json.Marshal(map[string]any{
		"type":      role,
		"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
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
