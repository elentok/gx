package prompt

import (
	"cmp"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"slices"
	"strings"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/agentrunner/claudeui"
	"github.com/elentok/gx/harness"
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
	switch req.Source {
	case wire.CardSourceTicketAsk:
		return p.answerTicketAsk(req, source)
	case wire.CardSourceRound, wire.CardSourcePermission, wire.CardSourceCodex, wire.CardSourceClaudeDialog:
	default:
		return sessions.Refusal(wire.ReasonAnswerSourceNotSupported, fmt.Sprintf("answers to %q cards are not supported yet", req.Source)), nil
	}
	dir, meta, res, err := p.sessions.SessionDir(req.ID)
	if res.Refused || err != nil {
		return res, err
	}
	snap, err := p.snapshot(req.ID, dir, meta)
	if err != nil {
		return wire.SessionResult{}, err
	}
	card := findCard(p.sessionChat.Cards(dir, meta.ChatMeta(), snap), req)
	if card == nil {
		return sessions.Refusal(wire.ReasonStale, "the question moved on; review it first"), nil
	}
	switch req.Source {
	case wire.CardSourceClaudeDialog:
		return p.answerClaudeDialog(req, dir), nil
	case wire.CardSourceCodex:
		return p.answerCodexQuestion(req, *card, source)
	case wire.CardSourcePermission:
		if meta.Harness == string(harness.Codex) {
			return p.answerCodexCard(req, *card, source)
		}
		return p.answerPermissionCard(req, *card, source)
	}
	prompt := wire.SessionPromptRequest{
		ID: req.ID, Text: roundPrompt(card.Questions, req.Answers, req.Note),
		LastMessage: req.LastMessage, Images: req.Images, Huh: huhOf(card.Questions, req.Answers),
	}
	return p.promptSession(prompt, source)
}

// answerTicketAsk answers a ticket-ask card through the ticket answer code. A
// Queue agent id is not a sessions-dir session, so it resolves as a read does.
// The card id carries the ask's state token, which the answer code rechecks
// under the ticket lock: a stale id reaches nothing.
func (p *Prompt) answerTicketAsk(req wire.SessionAnswerRequest, source string) (wire.SessionResult, error) {
	dir, meta, res, err := p.sessions.ReadableSession(req.ID)
	if res.Refused || err != nil {
		return res, err
	}
	card := findCard(p.sessionChat.Cards(dir, meta.ChatMeta(), chat.Snapshot{}), req)
	if card == nil {
		return sessions.Refusal(wire.ReasonStale, "the question moved on; review it first"), nil
	}
	address, state, _ := chat.SplitTicketAskID(card.ID)
	a := answerAt(req.Answers, 0)
	q := wire.QueueRequest{
		Address: address, State: state, Source: source,
		Choice: a.Choice, Text: a.Text, Huh: a.Huh, Note: cmp.Or(a.Note, req.Note),
	}
	qr, err := p.AnswerTicket(q)
	if err != nil {
		return wire.SessionResult{}, err
	}
	if qr.Refused {
		return sessions.Refusal(qr.Reason, qr.Message), nil
	}
	return wire.SessionResult{}, nil
}

// snapshot re-reads the session's transcript; no transcript is an empty one.
func (p *Prompt) snapshot(id, dir string, meta sessions.Meta) (chat.Snapshot, error) {
	var snap chat.Snapshot
	if path, kind, ok := p.sessionChat.Transcript(id, meta.ChatMeta()); ok {
		var err error
		snap, err = p.sessionChat.ReadSnapshot(path, kind, meta.FlowRef(), dir)
		if err != nil && !os.IsNotExist(err) {
			return chat.Snapshot{}, err
		}
	}
	return snap, nil
}

// findCard is the card the answer targets: the one of its source, and only when
// its id is the one the person saw. Nil means the question moved on.
func findCard(cards []wire.QuestionCard, req wire.SessionAnswerRequest) *wire.QuestionCard {
	i := slices.IndexFunc(cards, func(c wire.QuestionCard) bool { return c.Source == req.Source })
	if i < 0 || cards[i].ID != req.CardID {
		return nil
	}
	return &cards[i]
}

// answerCodexCard routes a hooked Codex card's Allow or Deny through the
// permission answer. The card id is the hook request id; the answer re-reads
// the request and refuses an id that moved on. A dismiss denies.
func (p *Prompt) answerCodexCard(req wire.SessionAnswerRequest, card wire.QuestionCard, source string) (wire.SessionResult, error) {
	decision := cmp.Or(answerAt(req.Answers, 0).Choice, string(agentrunner.DecisionDeny))
	if !slices.ContainsFunc(card.Questions[0].Options, func(o wire.QuestionCardOption) bool { return o.ID == decision }) {
		return sessions.Refusal(wire.ReasonSessionBadRequest, fmt.Sprintf("choice %q is not an option of the dialog", decision)), nil
	}
	return p.keysSession(wire.SessionKeysRequest{ID: req.ID, Permission: decision, RequestID: card.ID}, source)
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
