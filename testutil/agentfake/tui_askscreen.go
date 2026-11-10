package agentfake

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
	"unicode/utf8"
)

// AskScreen is an AskUserQuestion dialog with no hook: the TUI fake draws the
// real layouts, takes the keys claude takes (arrows, digits, Enter, Esc and
// typed text) and writes what it did to files, so a test can read the keys.
type AskScreen struct {
	Questions []AskQuestion `json:"questions"`
	// KeysFile receives one line per key read: "down", "up", "enter", "esc", or
	// the character typed.
	KeysFile string `json:"keys_file,omitempty"`
	// AnswersFile receives the answers as JSON once the dialog ends: an object
	// keyed by question, or the string "cancelled".
	AnswersFile string `json:"answers_file,omitempty"`
	// MutateAfterKeys, when above zero, rewrites the question once that many
	// keys have been read, as a dialog closed and reopened under a person.
	MutateAfterKeys int `json:"mutate_after_keys,omitempty"`
}

// askUI is the live state of an AskScreen.
type askUI struct {
	spec    AskScreen
	cur     int // question index; len(Questions) is the review screen
	pos     int // cursor row of the current screen
	checked [][]bool
	typed   []string
	done    []bool
	keys    int
	esc     bool // an Esc was read; a "[" makes it an arrow, a timeout a cancel
	seq     bool // after Esc "["
}

func (t *tui) startAskScreen(s *AskScreen) error {
	if s == nil {
		return fmt.Errorf("ActionAskScreen needs AskScreen")
	}
	u := &askUI{spec: *s, done: make([]bool, len(s.Questions)), typed: make([]string, len(s.Questions))}
	for _, q := range s.Questions {
		u.checked = append(u.checked, make([]bool, len(q.Options)+1))
	}
	t.askUI = u
	t.dialog = u.rows()
	return t.setStatus(StatusWaiting, "input needed")
}

// askScreenKey takes one key of an open AskScreen.
func (t *tui) askScreenKey(b byte) error {
	u := t.askUI
	switch {
	case u.seq:
		u.seq, u.esc = false, false
		switch b {
		case 'A':
			u.record("up")
			u.move(-1)
		case 'B':
			u.record("down")
			u.move(1)
		}
	case u.esc && b == '[':
		u.seq = true
	case b == EscKey:
		u.esc = true
		go func() {
			time.Sleep(60 * time.Millisecond)
			_ = SendControl(t.ctlPath, Control{Action: ActionAskEscTimeout})
		}()
		return nil
	default:
		u.esc = false
		if err := u.key(t, b); err != nil {
			return err
		}
	}
	return t.redrawAsk()
}

// askEscTimeout turns a lone Esc into a cancel.
func (t *tui) askEscTimeout() error {
	u := t.askUI
	if u == nil || !u.esc || u.seq {
		return nil
	}
	u.esc = false
	u.record("esc")
	return t.endAsk("cancelled")
}

func (t *tui) redrawAsk() error {
	if t.askUI == nil {
		return nil
	}
	t.dialog = t.askUI.rows()
	t.draw()
	return nil
}

func (t *tui) endAsk(answers any) error {
	u := t.askUI
	t.askUI, t.dialog = nil, nil
	if u.spec.AnswersFile != "" {
		b, _ := json.Marshal(answers)
		if err := os.WriteFile(u.spec.AnswersFile, b, 0o644); err != nil {
			return err
		}
	}
	return t.setStatus(StatusIdle, "")
}

func (u *askUI) record(k string) {
	u.keys++
	if u.spec.KeysFile != "" {
		f, err := os.OpenFile(u.spec.KeysFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err == nil {
			fmt.Fprintln(f, k)
			f.Close()
		}
	}
	if n := u.spec.MutateAfterKeys; n > 0 && u.keys == n && u.cur < len(u.spec.Questions) {
		u.spec.Questions[u.cur].Question += " (changed)"
	}
}

func (u *askUI) q() AskQuestion { return u.spec.Questions[u.cur] }
func (u *askUI) review() bool   { return u.cur >= len(u.spec.Questions) }

// other is the row index of "Type something".
func (u *askUI) other() int { return len(u.q().Options) }

// advanceRow is the row of Next/Submit on a multi-select, else -1.
func (u *askUI) advanceRow() int {
	if u.q().MultiSelect {
		return u.other() + 1
	}
	return -1
}

func (u *askUI) chatRow() int { return u.other() + 1 + btoi(u.q().MultiSelect) }

func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}

func (u *askUI) move(d int) {
	last := 1
	if !u.review() {
		last = u.chatRow()
	}
	u.pos = max(0, min(last, u.pos+d))
}

// key takes a byte that is not part of an arrow.
func (u *askUI) key(t *tui, b byte) error {
	if u.review() {
		switch {
		case b == '\r':
			u.record("enter")
			return u.reviewPick(t, u.pos)
		case b == '1' || b == '2':
			u.record(string(b))
			return u.reviewPick(t, int(b-'1'))
		}
		return nil
	}
	m := u.q().MultiSelect
	onOther := u.pos == u.other()
	switch {
	case b == '\r':
		u.record("enter")
		switch {
		case u.pos == u.chatRow():
			return t.endAsk("chat")
		case u.pos == u.advanceRow():
			return u.advance(t)
		case m:
			u.checked[u.cur][u.pos] = !u.checked[u.cur][u.pos]
		case onOther:
			if u.typed[u.cur] != "" {
				return u.advance(t)
			}
		default:
			return u.pickSingle(t, u.pos)
		}
	case b == 0x7f && onOther:
		u.record("backspace")
		if s := u.typed[u.cur]; s != "" {
			_, n := utf8.DecodeLastRuneInString(s)
			u.typed[u.cur] = s[:len(s)-n]
		}
	case onOther && b >= ' ':
		u.record(string(b))
		u.typed[u.cur] += string(b)
		if m {
			u.checked[u.cur][u.pos] = true
		}
	case b >= '0' && b <= '9':
		return u.digit(t, int(b-'0'))
	}
	return nil
}

// digit is the number keys of the table in the research: what a digit does
// depends on single or multi and on the Other row.
func (u *askUI) digit(t *tui, n int) error {
	u.record(fmt.Sprint(n))
	m := u.q().MultiSelect
	switch {
	case n == u.other()+2:
		return t.endAsk("chat")
	case n < 1 || n > u.other()+1:
	case n-1 == u.other():
		if m {
			u.checked[u.cur][n-1] = true
		} else {
			u.pos = n - 1
		}
	case m:
		u.checked[u.cur][n-1] = !u.checked[u.cur][n-1]
	default:
		return u.pickSingle(t, n-1)
	}
	return nil
}

func (u *askUI) pickSingle(t *tui, i int) error {
	u.done[u.cur] = true
	u.typed[u.cur] = ""
	for j := range u.checked[u.cur] {
		u.checked[u.cur][j] = j == i
	}
	return u.advance(t)
}

// advance leaves the current question: the next one, the review, or the end.
func (u *askUI) advance(t *tui) error {
	u.done[u.cur] = true
	u.cur++
	u.pos = 0
	if len(u.spec.Questions) == 1 {
		return t.endAsk(u.answers())
	}
	return nil
}

func (u *askUI) reviewPick(t *tui, i int) error {
	if i == 0 {
		return t.endAsk(u.answers())
	}
	return t.endAsk("cancelled")
}

func (u *askUI) answers() map[string]string {
	out := map[string]string{}
	for i, q := range u.spec.Questions {
		var parts []string
		for j, o := range q.Options {
			if u.checked[i][j] {
				parts = append(parts, o.Label)
			}
		}
		if u.typed[i] != "" && (!q.MultiSelect || u.checked[i][len(q.Options)]) {
			parts = append(parts, u.typed[i])
		}
		out[q.Question] = strings.Join(parts, ", ")
	}
	return out
}

// rows draws the screen as claude does.
func (u *askUI) rows() []string {
	if u.review() {
		return u.reviewRows()
	}
	q := u.q()
	rows := []string{u.tabBar(), "", q.Question}
	cursor := func(i int) string {
		if u.pos == i {
			return "❯ "
		}
		return "  "
	}
	for i, o := range q.Options {
		label := o.Label
		if q.MultiSelect {
			label = checkbox(u.checked[u.cur][i]) + " " + label
		}
		rows = append(rows, fmt.Sprintf("%s%d. %s", cursor(i), i+1, label))
		if o.Description != "" {
			rows = append(rows, "     "+o.Description)
		}
	}
	o := u.other()
	switch {
	case q.MultiSelect && u.typed[u.cur] != "":
		rows = append(rows, fmt.Sprintf("%s%d. %s %s", cursor(o), o+1, checkbox(u.checked[u.cur][o]), u.typed[u.cur]))
	case q.MultiSelect:
		rows = append(rows, fmt.Sprintf("%s%d. %s Type something", cursor(o), o+1, checkbox(u.checked[u.cur][o])))
	case u.typed[u.cur] != "":
		rows = append(rows, fmt.Sprintf("%s%d. %s", cursor(o), o+1, u.typed[u.cur]))
	default:
		rows = append(rows, fmt.Sprintf("%s%d. Type something.", cursor(o), o+1))
	}
	if q.MultiSelect {
		name := "Next"
		if u.cur == len(u.spec.Questions)-1 {
			name = "Submit"
		}
		rows = append(rows, cursor(u.advanceRow())+"   "+name)
	}
	chat := u.chatRow()
	return append(rows,
		"────────────",
		fmt.Sprintf("%s%d. Chat about this", cursor(chat), u.other()+2),
		"",
		"Enter to select · Tab/Arrow keys to navigate · Esc to cancel",
	)
}

func checkbox(on bool) string {
	if on {
		return "[✔]"
	}
	return "[ ]"
}

func (u *askUI) tabBar() string {
	bar := "←  "
	for i, q := range u.spec.Questions {
		mark := "☐"
		if u.done[i] {
			mark = "☒"
		}
		bar += mark + " " + cmpHeader(q, i) + "  "
	}
	return bar + "✔ Submit  →"
}

func cmpHeader(q AskQuestion, i int) string {
	if q.Header != "" {
		return q.Header
	}
	return fmt.Sprintf("Q%d", i+1)
}

func (u *askUI) reviewRows() []string {
	rows := []string{u.tabBar(), "", "Review your answers", ""}
	answers := u.answers()
	for _, q := range u.spec.Questions {
		rows = append(rows, " ● "+q.Question, "   → "+answers[q.Question])
	}
	cursor := func(i int) string {
		if u.pos == i {
			return "❯ "
		}
		return "  "
	}
	return append(rows, "", "Ready to submit your answers?",
		cursor(0)+"1. Submit answers", cursor(1)+"2. Cancel", "",
		"Enter to select · Tab/Arrow keys to navigate · Esc to cancel")
}
