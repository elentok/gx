package prompt

import (
	"cmp"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/elentok/gx/agentrunner/claudeui"
	"github.com/elentok/gx/agentrunner/codexui"
	"github.com/elentok/gx/nativerunner/holder"
	"github.com/elentok/gx/server/sessions"
	"github.com/elentok/gx/server/sessions/chat"
	"github.com/elentok/gx/server/wire"
)

// DefaultCodexQuestionWait bounds the wait for the overlay to show the next
// question's header.
const DefaultCodexQuestionWait = 2 * time.Second

const codexQuestionPoll = 20 * time.Millisecond

// answerCodexQuestion types the answers into Codex's question overlay, one
// question at a time. Before each question it reads the screen and checks it
// against the card; on any difference it stops, so gx never sends keys to a
// screen it did not verify. The cursor starts on row 1 on every question, so
// the moves do not depend on where it was.
func (p *Prompt) answerCodexQuestion(req wire.SessionAnswerRequest, card wire.QuestionCard, source string) (wire.SessionResult, error) {
	plan, refusal := codexQuestionKeys(card, req.Answers)
	if refusal != nil {
		return *refusal, nil
	}
	dir, res, err := p.codexQuestionReady(req.ID)
	if res.Refused || err != nil {
		return res, err
	}
	c, err := holder.DialHolder(dir)
	if err != nil {
		return sessions.Refusal(wire.ReasonSessionNotLive, fmt.Sprintf("session %s has no live holder", req.ID)), nil
	}
	if rows, _, err := c.Screen(0); err != nil {
		return sessions.Refusal(wire.ReasonCodexNotReady, "the question is not on the screen yet; try again"), nil
	} else if _, ok := codexui.ParseOverlay(rows); !ok {
		return sessions.Refusal(wire.ReasonCodexNotReady, "the question is not on the screen yet; try again"), nil
	}
	wait := cmp.Or(p.cfg.CodexQuestionWait, DefaultCodexQuestionWait)
	sentAny := false
	stop := func(msg string) (wire.SessionResult, error) {
		if sentAny {
			return sessions.Refusal(wire.ReasonCodexStopped, msg+"; finish this question in Screen"), nil
		}
		return sessions.Refusal(wire.ReasonCodexMismatch, msg+"; nothing was sent"), nil
	}
	for i, q := range card.Questions {
		if msg := checkOverlay(c, wait, i, len(card.Questions), q); msg != "" {
			return stop(msg)
		}
		for j, k := range plan[i] {
			k.ID = req.ID
			if i == len(plan)-1 && j == len(plan[i])-1 {
				// The last key submits; it waits for the session to leave Blocked.
				res, err := p.keysSession(k, source)
				if res.Refused && sentAny {
					return stop(res.Message)
				}
				return res, err
			}
			seq, n, refusal := validateSessionKeys(k)
			sent, err := p.sendSessionKeys(k, source, seq, n, refusal)
			if err != nil {
				return wire.SessionResult{}, err
			}
			if sent.res.Refused {
				return stop(sent.res.Message)
			}
			sentAny = true
		}
	}
	return wire.SessionResult{}, nil
}

// codexQuestionReady refuses unless the session is Blocked on a question.
func (p *Prompt) codexQuestionReady(id string) (string, wire.SessionResult, error) {
	p.sessions.Watches.MetaMu.Lock()
	defer p.sessions.Watches.MetaMu.Unlock()
	dir, meta, res, err := p.sessions.SessionDir(id)
	if res.Refused || err != nil {
		return dir, res, err
	}
	if st := meta.Info().Status; st.State != wire.SessionBlocked || st.Kind != "question" {
		return dir, sessions.Refusal(wire.ReasonCodexNotReady, fmt.Sprintf("session %s is not blocked on a question yet; try again", id)), nil
	}
	return dir, wire.SessionResult{}, nil
}

// checkOverlay waits for the overlay's header to read question i+1 (a late
// redraw is not a mismatch), then compares the rest of the screen with the
// card's question. It returns a description of the difference, or "".
func checkOverlay(c *holder.HolderClient, wait time.Duration, i, total int, q wire.QuestionCardQuestion) string {
	want := i + 1
	deadline := time.Now().Add(wait)
	var o codexui.Overlay
	for {
		rows, _, err := c.Screen(0)
		ok := false
		if err == nil {
			o, ok = codexui.ParseOverlay(rows)
		}
		if ok && o.Number == want {
			break
		}
		if time.Now().After(deadline) {
			if !ok {
				return "the screen is not a question layout gx can read"
			}
			return fmt.Sprintf("the screen shows question %d, not %d", o.Number, want)
		}
		time.Sleep(codexQuestionPoll)
	}
	numbered := numberedOptions(q)
	switch {
	case o.Total != total:
		return fmt.Sprintf("the screen has %d questions, the card has %d", o.Total, total)
	case claudeui.Normalize(o.Question) != claudeui.Normalize(q.Title):
		return "the question text on the screen differs from the card"
	case o.Rows != len(numbered)+1:
		return fmt.Sprintf("the screen has %d rows, the card has %d options", o.Rows-1, len(numbered))
	case o.Cursor != 1 || o.NotesOpen:
		return "the screen's cursor is not on the first row"
	case o.LastQuestion != (i == total-1):
		return "the screen's footer does not match the question's position"
	}
	for j, opt := range numbered {
		if claudeui.Normalize(o.Labels[j]) != claudeui.Normalize(opt.Label) {
			return fmt.Sprintf("row %d on the screen differs from the card", j+1)
		}
	}
	return ""
}

// numberedOptions is the options Codex's overlay numbers: the card's rows
// without its free-text row, which is the overlay's own None of the above.
func numberedOptions(q wire.QuestionCardQuestion) []wire.QuestionCardOption {
	return slices.DeleteFunc(slices.Clone(q.Options), func(o wire.QuestionCardOption) bool { return o.Text })
}

// codexQuestionKeys is the keys of every question, in order. An option k is
// digit k; a note on it is Down x (k-1), Tab, the text, Enter; free text on
// None of the above is Down to the last row, Enter, the text, Enter.
func codexQuestionKeys(card wire.QuestionCard, answers []wire.SessionAnswer) ([][]wire.SessionKeysRequest, *wire.SessionResult) {
	bad := func(msg string) ([][]wire.SessionKeysRequest, *wire.SessionResult) {
		res := sessions.Refusal(wire.ReasonSessionBadRequest, msg)
		return nil, &res
	}
	key := func(k string) wire.SessionKeysRequest { return wire.SessionKeysRequest{Key: k, OnlyBlocked: true} }
	typed := func(s string) wire.SessionKeysRequest { return wire.SessionKeysRequest{Text: s, OnlyBlocked: true} }
	downs := func(n int) (out []wire.SessionKeysRequest) {
		for range n {
			out = append(out, key("down"))
		}
		return out
	}
	var plan [][]wire.SessionKeysRequest
	for i, q := range card.Questions {
		a := answerAt(answers, i)
		note, text := strings.TrimSpace(a.Note), strings.TrimSpace(a.Text)
		numbered := numberedOptions(q)
		var ks []wire.SessionKeysRequest
		freeText := a.Choice == chat.CodexFreeTextID && len(numbered) < len(q.Options)
		switch {
		case a.Choice != "" && !freeText:
			k, err := strconv.Atoi(a.Choice)
			if err != nil || k < 1 || k > len(numbered) || k > 9 {
				return bad(fmt.Sprintf("choice %q is not an option of question %d", a.Choice, i+1))
			}
			if note == "" {
				ks = []wire.SessionKeysRequest{key(a.Choice)}
				break
			}
			ks = append(downs(k-1), key("tab"), typed(note), key("enter"))
		case text != "":
			ks = append(downs(len(numbered)), key("enter"), typed(text), key("enter"))
		default:
			return bad(fmt.Sprintf("question %d needs an answer", i+1))
		}
		for _, k := range ks {
			if _, _, refusal := validateSessionKeys(k); refusal != nil {
				return nil, refusal
			}
		}
		plan = append(plan, ks)
	}
	if len(plan) == 0 {
		return bad("the card has no questions")
	}
	return plan, nil
}
