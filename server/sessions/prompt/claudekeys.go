package prompt

import (
	"cmp"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/elentok/gx/agentrunner/claudeui"
	"github.com/elentok/gx/nativerunner/holder"
	"github.com/elentok/gx/server/sessions"
	"github.com/elentok/gx/server/wire"
)

// maxDialogMoves bounds the cursor moves toward one row; a dialog has a handful
// of rows, so more means the keys are not reaching it.
const maxDialogMoves = 24

// dialogMismatch is why a keystroke answer stopped: the screen is not what the
// next key assumes. The dialog is left open for the terminal.
type dialogMismatch string

func (m dialogMismatch) Error() string { return string(m) }

// answerClaudeKeys answers a keystroke-fallback card by keys: the session's own
// dialog, with no hook behind it. Before every key it re-reads the screen and
// stops on any difference from what that key assumes, as the 60 s
// askUserQuestionTimeout or a person at the terminal can change the dialog
// under it. No answer dismisses with Esc.
func (p *Prompt) answerClaudeKeys(req wire.SessionAnswerRequest, dir string) wire.SessionResult {
	if req.Release {
		return sessions.Refusal(wire.ReasonStale, "the question moved on; review it first")
	}
	conn, err := holder.DialHolder(dir)
	if err != nil {
		return sessions.Refusal(wire.ReasonSessionNotLive, fmt.Sprintf("session %s has no live holder", req.ID))
	}
	k := &keyDriver{p: p, conn: conn}
	err = k.run(req)
	switch e := err.(type) {
	case nil:
		return wire.SessionResult{}
	case dialogMismatch:
		return sessions.Refusal(wire.ReasonNotAnswerable, "Stopped, the dialog is still open: "+string(e))
	case badAnswer:
		return sessions.Refusal(wire.ReasonBadChoice, string(e))
	default:
		p.log.Warn("answer claude dialog by keys", "session", req.ID, "err", err)
		return sessions.Refusal(wire.ReasonNotAnswerable, "Stopped, the dialog is still open: "+e.Error())
	}
}

// badAnswer is an answer that does not fit the dialog, found before any key.
type badAnswer string

func (b badAnswer) Error() string { return string(b) }

// keyDriver sends keys to one dialog, checking the screen between them.
type keyDriver struct {
	p    *Prompt
	conn *holder.HolderClient
	id   string   // the question block it started on
	rows []string // the last screen read
}

func (k *keyDriver) run(req wire.SessionAnswerRequest) error {
	rows, _, err := k.conn.Screen(0)
	if err != nil {
		return err
	}
	k.rows = rows
	d, ok := claudeui.ParseAskDialog(rows)
	if !ok || d.Review || !strings.HasPrefix(req.CardID, d.ID+"-") {
		return dialogMismatch("the dialog on screen is not the one the card shows")
	}
	k.id = d.ID
	a := answerAt(req.Answers, 0)
	if !isClaudeAnswered(a) {
		_, err := k.press("\x1b")
		return err
	}
	picks, text, err := dialogPicks(d, a)
	if err != nil {
		return err
	}
	if d.Multi {
		err = k.multi(d, picks, text)
	} else {
		err = k.single(picks, text)
	}
	if err != nil {
		return err
	}
	return k.submitReview()
}

// dialogPicks turns the answer into 0-based option rows. The Other row is the
// last option; text alone picks it.
func dialogPicks(d claudeui.AskDialog, a wire.SessionAnswer) (picks []int, text string, err error) {
	text = strings.TrimSpace(a.Text)
	if strings.ContainsAny(text, "\r\n\x1b\t") {
		return nil, "", badAnswer("the text has a line break or control character")
	}
	other := len(d.Options) - 1
	ids := a.Choices
	if a.Choice != "" {
		ids = append([]string{a.Choice}, ids...)
	}
	for _, id := range ids {
		if id == "other" {
			picks = append(picks, other)
			continue
		}
		n, err := strconv.Atoi(id)
		if err != nil || n < 1 || n > other {
			return nil, "", badAnswer(fmt.Sprintf("choice %q is not an option of the dialog", id))
		}
		picks = append(picks, n-1)
	}
	if text != "" && !slices.Contains(picks, other) {
		picks = append(picks, other)
	}
	if slices.Contains(picks, other) && text == "" {
		return nil, "", badAnswer("Other needs text")
	}
	slices.Sort(picks)
	picks = slices.Compact(picks)
	if !d.Multi && len(picks) != 1 {
		return nil, "", badAnswer("the question takes one answer")
	}
	return picks, text, nil
}

func (k *keyDriver) single(picks []int, text string) error {
	i := picks[0]
	d, err := k.moveTo(i)
	if err != nil {
		return err
	}
	if d.Options[i].Other {
		if err := k.typeOther(i, text, false); err != nil {
			return err
		}
	}
	return k.enter()
}

func (k *keyDriver) multi(d claudeui.AskDialog, picks []int, text string) error {
	other := len(d.Options) - 1
	for j := range other {
		want := slices.Contains(picks, j)
		cur, err := k.read()
		if err != nil {
			return err
		}
		if cur.Options[j].Checked == want {
			continue
		}
		if _, err := k.moveTo(j); err != nil {
			return err
		}
		if err := k.enter(); err != nil {
			return err
		}
		if cur, err = k.read(); err != nil {
			return err
		}
		if cur.Options[j].Checked != want {
			return dialogMismatch(fmt.Sprintf("row %d did not toggle", j+1))
		}
	}
	if slices.Contains(picks, other) {
		if _, err := k.moveTo(other); err != nil {
			return err
		}
		// Typing checks the row; Enter on it would uncheck it.
		if err := k.typeOther(other, text, true); err != nil {
			return err
		}
	}
	cur, err := k.read()
	if err != nil {
		return err
	}
	for j, o := range cur.Options {
		if o.Checked != slices.Contains(picks, j) {
			return dialogMismatch(fmt.Sprintf("row %d is not in the state the answer needs", j+1))
		}
	}
	if cur.Advance == "" {
		return dialogMismatch("there is no Next or Submit row")
	}
	if _, err := k.moveTo(len(cur.Options)); err != nil {
		return err
	}
	return k.enter()
}

// typeOther types text into the Other row under the cursor and checks that it
// landed there.
func (k *keyDriver) typeOther(row int, text string, checked bool) error {
	if _, err := k.press(text); err != nil {
		return err
	}
	d, err := k.read()
	if err != nil {
		return err
	}
	o := d.Options[row]
	if o.Label != text || d.Cursor != claudeui.AskCursorOption || d.CursorIndex != row || (checked && !o.Checked) {
		return dialogMismatch("the text did not land on the Other row")
	}
	return nil
}

// enter presses Enter on the row the cursor is on and needs the screen to
// react.
func (k *keyDriver) enter() error {
	changed, err := k.press("\r")
	if err != nil {
		return err
	}
	if !changed {
		return dialogMismatch("the dialog did not react to Enter")
	}
	return nil
}

// submitReview confirms the review screen of a dialog with several questions,
// which the card cannot show, once the last question is answered.
func (k *keyDriver) submitReview() error {
	d, ok := claudeui.ParseAskDialog(k.rows)
	if !ok || !d.Review {
		return nil
	}
	if d.Cursor != claudeui.AskCursorOption || d.CursorIndex != 0 {
		return dialogMismatch("the review screen is not on Submit answers")
	}
	return k.enter()
}

// rowOf is the cursor's row: the options, then Next/Submit, then Chat about this.
func rowOf(d claudeui.AskDialog) int {
	switch d.Cursor {
	case claudeui.AskCursorOption:
		return d.CursorIndex
	case claudeui.AskCursorAdvance:
		return len(d.Options)
	}
	if d.Advance != "" {
		return len(d.Options) + 1
	}
	return len(d.Options)
}

// read parses the screen as last read and refuses one that is not the
// question the answer started on.
func (k *keyDriver) read() (claudeui.AskDialog, error) {
	d, ok := claudeui.ParseAskDialog(k.rows)
	switch {
	case !ok:
		return d, dialogMismatch("the screen is not a dialog gx can read")
	case d.Review || d.ID != k.id:
		return d, dialogMismatch("the question on screen changed")
	}
	return d, nil
}

// moveTo walks the cursor to row, checking the screen after every key.
func (k *keyDriver) moveTo(row int) (claudeui.AskDialog, error) {
	for range maxDialogMoves {
		d, err := k.read()
		if err != nil {
			return d, err
		}
		at := rowOf(d)
		if at == row {
			return d, nil
		}
		key := "\x1b[B"
		if at > row {
			key = "\x1b[A"
		}
		if changed, err := k.press(key); err != nil {
			return d, err
		} else if !changed {
			return d, dialogMismatch("the cursor did not move")
		}
	}
	return claudeui.AskDialog{}, dialogMismatch("the cursor never reached the row")
}

// press sends keys, waits for the screen to change (it may not) and then to
// settle. It reports whether the screen differs from before the keys.
func (k *keyDriver) press(keys string) (bool, error) {
	before := k.rows
	if err := k.conn.SendPersonKeys(keys); err != nil {
		return false, err
	}
	step := cmp.Or(k.p.cfg.KeysSettleStep, DefaultKeysSettleStep)
	limit := cmp.Or(k.p.cfg.KeysSettleCap, DefaultKeysSettleCap)
	read := func() ([]string, error) {
		rows, _, err := k.conn.Screen(0)
		return rows, err
	}
	deadline := time.Now().Add(limit)
	for {
		rows, err := read()
		if err != nil {
			return false, err
		}
		if !slices.Equal(rows, before) || !time.Now().Before(deadline) {
			break
		}
		time.Sleep(step)
	}
	if rows := SettleScreen(read, step, limit); rows != nil {
		k.rows = rows
	}
	return !slices.Equal(k.rows, before), nil
}
