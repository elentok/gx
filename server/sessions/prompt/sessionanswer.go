package prompt

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

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
// for its source, and sends the answer as the person's prompt. Only a printed
// round is routed so far.
func (p *Prompt) answerSession(req wire.SessionAnswerRequest, source string) (wire.SessionResult, error) {
	dir, meta, res, err := p.sessions.SessionDir(req.ID)
	if res.Refused || err != nil {
		return res, err
	}
	if req.Source != wire.CardSourceRound {
		return sessions.Refusal(wire.ReasonAnswerSourceNotSupported, fmt.Sprintf("answers to %q cards are not supported yet", req.Source)), nil
	}
	card, err := p.currentCard(req.ID, dir, meta, req.Source)
	if err != nil {
		return wire.SessionResult{}, err
	}
	if card == nil || card.ID != req.CardID {
		return sessions.Refusal(wire.ReasonStale, "the question moved on; review it first"), nil
	}
	prompt := wire.SessionPromptRequest{
		ID: req.ID, Text: roundPrompt(card.Questions, req.Answers, req.Note),
		LastMessage: req.LastMessage, Images: req.Images, Huh: huhOf(card.Questions, req.Answers),
	}
	return p.promptSession(prompt, source)
}

// currentCard is the session's current card of source, or nil with none.
func (p *Prompt) currentCard(id, dir string, meta sessions.Meta, source string) (*wire.QuestionCard, error) {
	path, kind, ok := p.sessionChat.Transcript(id, meta.ChatMeta())
	if !ok {
		return nil, nil
	}
	snap, err := p.sessionChat.ReadSnapshot(path, kind, meta.FlowRef(), dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	for _, c := range chat.SessionCards(meta.ChatMeta(), snap.Round) {
		if c.Source == source {
			return &c, nil
		}
	}
	return nil, nil
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
