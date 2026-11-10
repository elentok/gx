package prompt

import (
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/elentok/gx/server/sessions"
	"github.com/elentok/gx/server/wire"
)

// dismissReason is what Claude reads when the person dismisses a held question.
const dismissReason = "The user doesn't want to proceed with this question. Do not ask again. STOP and wait for the user to tell you how to proceed."

// answerClaudeDialog resolves the held AskUserQuestion call of a claude-dialog
// card (already checked as the current one): Release hands it to the terminal
// dialog, an answer with nothing picked denies it, and anything else must
// answer every question. A call that is no longer held is stale.
func (p *Prompt) answerClaudeDialog(req wire.SessionAnswerRequest) wire.SessionResult {
	stale := sessions.Refusal(wire.ReasonStale, "the question moved on; review it first")
	var held *wire.AskHookRequest
	for _, h := range p.sessions.HeldAsks(req.ID) {
		if h.ToolUseID == req.CardID {
			held = &h
		}
	}
	if held == nil {
		return stale
	}
	var res wire.AskHookResult
	switch {
	case req.Release:
		if !p.sessions.ReleaseAsk(req.ID, req.CardID) {
			return stale
		}
		return wire.SessionResult{}
	case !slices.ContainsFunc(req.Answers, isClaudeAnswered):
		res = wire.AskHookResult{Decision: wire.AskHookDeny, Reason: dismissReason}
	default:
		input, refusal := claudeUpdatedInput(*held, req.Answers)
		if refusal != "" {
			return sessions.Refusal(wire.ReasonSessionBadRequest, refusal)
		}
		res = wire.AskHookResult{Decision: wire.AskHookAllow, UpdatedInput: input}
	}
	if !p.sessions.ResolveAsk(req.ID, req.CardID, res) {
		return stale
	}
	return wire.SessionResult{}
}

func isClaudeAnswered(a wire.SessionAnswer) bool {
	return a.Choice != "" || len(a.Choices) > 0 || strings.TrimSpace(a.Text) != ""
}

// claudeUpdatedInput is the AskUserQuestion input that answers the call: the
// original questions, answers keyed by question text (multi-select labels
// joined with ", "), and the per-question notes as annotations. A refusal
// message says which question is not answered right.
func claudeUpdatedInput(held wire.AskHookRequest, answers []wire.SessionAnswer) (json.RawMessage, string) {
	type option struct {
		Label       string `json:"label"`
		Description string `json:"description"`
	}
	type question struct {
		Question    string   `json:"question"`
		Header      string   `json:"header"`
		MultiSelect bool     `json:"multiSelect"`
		Options     []option `json:"options"`
	}
	type annotation struct {
		Notes string `json:"notes"`
	}
	var qs []question
	given := map[string]string{}
	notes := map[string]annotation{}
	for i, q := range held.Questions {
		a := answerAt(answers, i)
		text := strings.TrimSpace(a.Text)
		ids := a.Choices
		if a.Choice != "" {
			ids = append([]string{a.Choice}, ids...)
		}
		cq := question{Question: q.Question, Header: q.Header, MultiSelect: q.MultiSelect, Options: []option{}}
		for _, o := range q.Options {
			cq.Options = append(cq.Options, option{o.Label, o.Description})
		}
		var parts []string
		for _, id := range ids {
			if id == "other" {
				if text == "" {
					return nil, fmt.Sprintf("question %d needs text for Other", i+1)
				}
				parts = append(parts, text)
				text = ""
				continue
			}
			n, err := strconv.Atoi(id)
			if err != nil || n < 1 || n > len(q.Options) {
				return nil, fmt.Sprintf("choice %q is not an option of question %d", id, i+1)
			}
			parts = append(parts, q.Options[n-1].Label)
		}
		if text != "" {
			parts = append(parts, text)
		}
		if len(parts) == 0 {
			return nil, fmt.Sprintf("question %d needs an answer", i+1)
		}
		if !q.MultiSelect && len(parts) > 1 {
			return nil, fmt.Sprintf("question %d takes one answer", i+1)
		}
		qs = append(qs, cq)
		given[q.Question] = strings.Join(parts, ", ")
		if n := strings.TrimSpace(a.Note); n != "" {
			notes[q.Question] = annotation{Notes: n}
		}
	}
	in := map[string]any{"questions": qs, "answers": given}
	if len(notes) > 0 {
		in["annotations"] = notes
	}
	b, err := json.Marshal(in)
	if err != nil {
		return nil, err.Error()
	}
	return b, ""
}
