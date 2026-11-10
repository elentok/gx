package codex

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/elentok/gx/agentrunner/codexui"
	"github.com/elentok/gx/testutil/agentfake"
)

// CodexQuestionRedrawEnv delays the overlay's redraw after each answered
// question by that many milliseconds, as a slow Codex does.
const CodexQuestionRedrawEnv = "GX_FAKE_CODEX_QUESTION_REDRAW_MS"

// QuestionKeysPath holds one line per key the question overlay took, text
// typed into the notes line as one "text:<typed>" line.
func QuestionKeysPath(codexHome, sessionID string) string {
	return filepath.Join(codexHome, "agentfake", sessionID+".questionkeys")
}

// ReadQuestionKeys is every key the fake's question overlay logged.
func ReadQuestionKeys(codexHome, sessionID string) ([]string, error) {
	data, err := os.ReadFile(QuestionKeysPath(codexHome, sessionID))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return strings.Split(strings.TrimSpace(string(data)), "\n"), nil
}

// codexAsk is the overlay of an open request_user_input call. The cursor
// resets to row 1 on every question, as in Codex.
type codexAsk struct {
	call    agentfake.QuestionCall
	shown   []agentfake.QuestionSpec
	n       int
	cursor  int
	notes   bool
	note    string
	pending string // text typed into the notes line, not yet logged
	esc     []byte
	answers [][]string
	drawn   bool
}

// openQuestion writes the call into the rollout and, unless it is Quiet,
// shows its overlay.
func (c *codexFake) openQuestion(call *agentfake.QuestionCall) error {
	if call == nil {
		return fmt.Errorf("question control has no call")
	}
	args, err := json.Marshal(map[string]any{"questions": call.Questions})
	if err != nil {
		return err
	}
	if err := c.writeLine("response_item", map[string]any{
		"type": "function_call", "name": "request_user_input", "call_id": call.CallID, "arguments": string(args),
	}); err != nil {
		return err
	}
	shown := call.Shown
	if len(shown) == 0 {
		shown = call.Questions
	}
	cursor := max(call.StartCursor, 1)
	c.ask = &codexAsk{call: *call, shown: shown, cursor: cursor, n: call.StartAt}
	if !call.Quiet {
		c.showQuestion()
	}
	return nil
}

// showQuestion reports Blocked on a question and draws the overlay.
func (c *codexFake) showQuestion() {
	if c.ask == nil || c.ask.drawn {
		return
	}
	c.ask.drawn = true
	agentfake.WriteAppReport(c.out, codexAppName, "blocked", "question", "")
	fmt.Fprint(c.out, c.ask.screen())
	c.onKey = c.askKey
}

func (a *codexAsk) rows() int { return len(a.shown[a.n].Options) + 1 }

func (a *codexAsk) label(row int) string {
	if opts := a.shown[a.n].Options; row <= len(opts) {
		return opts[row-1].Label
	}
	return codexui.NoneLabel
}

// screen is the overlay of the current question, in Codex's layout.
func (a *codexAsk) screen() string {
	q := a.shown[a.n]
	width := len(codexui.NoneLabel)
	for _, o := range q.Options {
		width = max(width, len(o.Label))
	}
	lines := []string{
		"\x1b[2J\x1b[H",
		fmt.Sprintf("  Question %d/%d (%d unanswered)", a.n+1, len(a.shown), len(a.shown)-len(a.answers)),
		"  " + q.Question,
	}
	for row := 1; row <= a.rows(); row++ {
		marker, desc := " ", "Optionally, add details in notes (tab)"
		if row == a.cursor {
			marker = "›"
		}
		if row <= len(q.Options) {
			desc = q.Options[row-1].Description
		}
		lines = append(lines, fmt.Sprintf("  %s %d. %-*s  %s", marker, row, width, a.label(row), desc))
	}
	footer := "tab to add notes | enter to submit answer | ←/→ to navigate questions | esc to interrupt"
	if a.notes {
		note := a.note
		if note == "" {
			note = "Add notes"
		}
		lines = append(lines, "  › "+note)
		footer = "tab or esc to clear notes | enter to submit answer"
	}
	if a.n == len(a.shown)-1 {
		footer = strings.Replace(footer, "enter to submit answer", "enter to submit all", 1)
	}
	return strings.Join(append(lines, "  "+footer), "\r\n") + "\r\n"
}

func (c *codexFake) logQuestionKey(line string) {
	path := QuestionKeysPath(c.home, c.id)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintln(f, line)
}

// flushQuestionText logs the typed text as one entry before the key that ends it.
func (c *codexFake) flushQuestionText() {
	if a := c.ask; a.pending != "" {
		c.logQuestionKey("text:" + a.pending)
		a.pending = ""
	}
}

// askKey is the overlay's key handler: digits pick a row and submit, Down and
// Up move the cursor, Tab opens the notes line, Enter submits the row.
func (c *codexFake) askKey(b byte) error {
	a := c.ask
	c.onKey = c.askKey
	if len(a.esc) > 0 {
		a.esc = append(a.esc, b)
		switch {
		case len(a.esc) == 2 && b != '[':
			a.esc = nil
		case len(a.esc) == 3:
			c.flushQuestionText()
			switch b {
			case 'A':
				a.cursor = a.rows() - (a.rows()-a.cursor+1)%a.rows()
				c.logQuestionKey("Up")
			case 'B':
				a.cursor = a.cursor%a.rows() + 1
				c.logQuestionKey("Down")
			}
			a.esc = nil
			fmt.Fprint(c.out, a.screen())
		}
		return nil
	}
	switch {
	case b == agentfake.EscKey:
		a.esc = []byte{b}
	case b == tabKey:
		c.flushQuestionText()
		c.logQuestionKey("Tab")
		a.notes = true
		fmt.Fprint(c.out, a.screen())
	case b == '\r':
		c.flushQuestionText()
		c.logQuestionKey("Enter")
		if a.cursor == a.rows() && !a.notes {
			a.notes = true
			fmt.Fprint(c.out, a.screen())
			return nil
		}
		return c.submitQuestion(a.cursor)
	case a.notes && b >= ' ':
		a.note += string(b)
		a.pending += string(b)
		fmt.Fprint(c.out, a.screen())
	case b >= '1' && b <= '9' && int(b-'0') <= a.rows():
		c.logQuestionKey(string(b))
		a.note = ""
		return c.submitQuestion(int(b - '0'))
	}
	return nil
}

// submitQuestion records the row's answer and moves to the next question, or
// writes the call's output after the last.
func (c *codexFake) submitQuestion(row int) error {
	a := c.ask
	ans := []string{a.label(row)}
	if a.note != "" {
		ans = append(ans, "user_note: "+a.note)
	}
	a.answers = append(a.answers, ans)
	a.n, a.cursor, a.notes, a.note = a.n+1, 1, false, ""
	if a.n < len(a.shown) {
		delay, _ := strconv.Atoi(os.Getenv(CodexQuestionRedrawEnv))
		screen := a.screen()
		fmt.Fprint(c.out, "\x1b[2J\x1b[H")
		if delay > 0 {
			time.AfterFunc(time.Duration(delay)*time.Millisecond, func() { fmt.Fprint(c.out, screen) })
		} else {
			fmt.Fprint(c.out, screen)
		}
		return nil
	}
	c.ask = nil
	// Codex's object key order is not the question order; the reverse makes the
	// match by id observable.
	var parts []string
	for i := len(a.answers) - 1; i >= 0; i-- {
		id, _ := json.Marshal(a.call.Questions[i].ID)
		list, _ := json.Marshal(a.answers[i])
		parts = append(parts, fmt.Sprintf(`%s:{"answers":%s}`, id, list))
	}
	output := `{"answers":{` + strings.Join(parts, ",") + `}}`
	fmt.Fprint(c.out, "\x1b[2J\x1b[H")
	if err := c.writeLine("response_item", map[string]any{
		"type": "function_call_output", "call_id": a.call.CallID, "output": output,
	}); err != nil {
		return err
	}
	c.report("idle")
	return nil
}
