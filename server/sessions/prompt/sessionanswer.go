package prompt

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"slices"
	"strings"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/agentrunner/claudeui"
	"github.com/elentok/gx/server/sessions"
	"github.com/elentok/gx/server/sessions/chat"
	"github.com/elentok/gx/server/wire"
)

// huhMessage is what the prompt says for a question the person marked Huh?.
const huhMessage = "I could not follow this question. Could you explain it another way?"

func (p *Prompt) answerHandler(source string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req wire.SessionAnswerRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		res, err := p.answerSession(req, source)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		wire.WriteJSON(w, res)
	}
}

// answerSession re-reads the chat, refuses a card that is not the current one
// for its source, and sends the answer: a printed round as the person's prompt,
// a permission dialog as the digit of the chosen option.
func (p *Prompt) answerSession(req wire.SessionAnswerRequest, source string) (wire.SessionResult, error) {
	dir, meta, res, err := p.sessions.SessionDir(req.ID)
	if res.Refused || err != nil {
		return res, err
	}
	if req.Source != wire.CardSourceRound && req.Source != wire.CardSourcePermission {
		return sessions.Refusal(wire.ReasonAnswerSourceNotSupported, fmt.Sprintf("answers to %q cards are not supported yet", req.Source)), nil
	}
	card, err := p.currentCard(req.ID, dir, meta, req.Source)
	if err != nil {
		return wire.SessionResult{}, err
	}
	if card == nil || card.ID != req.CardID {
		return sessions.Refusal(wire.ReasonStale, "the question moved on; review it first"), nil
	}
	if req.Source == wire.CardSourcePermission {
		return p.answerPermissionCard(req, *card, source)
	}
	prompt := wire.SessionPromptRequest{
		ID: req.ID, Text: roundPrompt(card.Questions, req.Answers, req.Note),
		LastMessage: req.LastMessage, Images: req.Images, Huh: huhOf(card.Questions, req.Answers),
	}
	return p.promptSession(prompt, source)
}

// currentCard is the session's current card of source, or nil with none.
func (p *Prompt) currentCard(id, dir string, meta sessions.Meta, source string) (*wire.QuestionCard, error) {
	var snap chat.Snapshot
	if path, kind, ok := p.sessionChat.Transcript(id, meta.ChatMeta()); ok {
		var err error
		snap, err = p.sessionChat.ReadSnapshot(path, kind, meta.FlowRef(), dir)
		if err != nil && !os.IsNotExist(err) {
			return nil, err
		}
	}
	for _, c := range p.sessionChat.Cards(dir, meta.ChatMeta(), snap) {
		if c.Source == source {
			return &c, nil
		}
	}
	return nil, nil
}

// answerPermissionCard sends the chosen option's digit. An answer that picks no
// option is a dismiss and denies, as the banner's Deny does.
func (p *Prompt) answerPermissionCard(req wire.SessionAnswerRequest, card wire.QuestionCard, source string) (wire.SessionResult, error) {
	a := answerAt(req.Answers, 0)
	isPlan := strings.HasPrefix(card.Questions[0].Title, claudeui.PlanQuestionPrefix)
	if a.Choice == "" {
		if isPlan {
			// Esc rejects the plan and leaves the session idle in plan mode.
			return p.keysSession(wire.SessionKeysRequest{ID: req.ID, Key: "esc", OnlyBlocked: true}, source)
		}
		return p.keysSession(wire.SessionKeysRequest{ID: req.ID, Permission: string(agentrunner.DecisionDeny)}, source)
	}
	i := slices.IndexFunc(card.Questions[0].Options, func(o wire.QuestionCardOption) bool { return o.ID == a.Choice })
	if i < 0 {
		return sessions.Refusal(wire.ReasonSessionBadRequest, fmt.Sprintf("choice %q is not an option of the dialog", a.Choice)), nil
	}
	if !card.Questions[0].Options[i].Text {
		return p.keysSession(wire.SessionKeysRequest{ID: req.ID, Key: a.Choice, OnlyBlocked: true}, source)
	}
	text := strings.TrimSpace(a.Text)
	if text == "" {
		return sessions.Refusal(wire.ReasonSessionBadRequest, "the option needs text before it can be sent"), nil
	}
	return p.answerTextRow(req.ID, a.Choice, text, source)
}

// answerTextRow picks the text row by its digit, types the text into the
// inline box and submits with Enter. Only the last key waits for the dialog to
// close: the first two leave it open.
func (p *Prompt) answerTextRow(id, digit, text, source string) (wire.SessionResult, error) {
	for _, req := range []wire.SessionKeysRequest{
		{ID: id, Key: digit, OnlyBlocked: true},
		{ID: id, Text: text, OnlyBlocked: true},
	} {
		seq, n, refusal := validateSessionKeys(req)
		sent, err := p.sendSessionKeys(req, source, seq, n, refusal)
		if sent.res.Refused || err != nil {
			return sent.res, err
		}
	}
	return p.keysSession(wire.SessionKeysRequest{ID: id, Key: "enter", OnlyBlocked: true}, source)
}

func answerAt(answers []wire.SessionAnswer, i int) wire.SessionAnswer {
	if i < len(answers) {
		return answers[i]
	}
	return wire.SessionAnswer{}
}

func isAnswered(a wire.SessionAnswer) bool {
	return a.Huh || a.Choice != "" || strings.TrimSpace(a.Text) != ""
}

// answerText is the answer as the prompt words it: the Huh? message, the
// picked option's label, or the typed text.
func answerText(q wire.QuestionCardQuestion, a wire.SessionAnswer) string {
	if a.Huh {
		return huhMessage
	}
	if a.Choice == "" {
		return strings.TrimSpace(a.Text)
	}
	if q.Kind == "yes-no" {
		label, line := "No", q.No
		if a.Choice == "yes" {
			label, line = "Yes", q.Yes
		}
		if line != "" {
			return label + " - " + line
		}
		return label
	}
	for _, o := range q.Options {
		if o.ID == a.Choice {
			return o.Label
		}
	}
	return a.Choice
}

// roundPrompt is one line per question, a per-question note on the next line,
// then the round's note. The web's design/round.ts composed the same text.
func roundPrompt(qs []wire.QuestionCardQuestion, answers []wire.SessionAnswer, note string) string {
	var lines []string
	for i, q := range qs {
		a := answerAt(answers, i)
		text := "(unanswered)"
		if isAnswered(a) {
			text = answerText(q, a)
		}
		lines = append(lines, fmt.Sprintf("Q%s - %s: %s", q.Label, q.Title, text))
		if n := strings.TrimSpace(a.Note); n != "" {
			lines = append(lines, "  Note: "+n)
		}
	}
	if n := strings.TrimSpace(note); n != "" {
		lines = append(lines, "", "Note:", n)
	}
	return strings.Join(lines, "\n")
}

// huhOf is the questions the person marked Huh?, for the conversation record.
func huhOf(qs []wire.QuestionCardQuestion, answers []wire.SessionAnswer) []wire.SessionHuh {
	var out []wire.SessionHuh
	for i, q := range qs {
		if answerAt(answers, i).Huh {
			out = append(out, wire.SessionHuh{Question: q.Body, Title: q.Title})
		}
	}
	return out
}
