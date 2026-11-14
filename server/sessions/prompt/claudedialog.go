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

// pickedIDs is the answer's chosen option ids as 0-based rows of the nOptions
// numbered options, whether it picked Other, and how many picks sat before
// Other (all of them if none). The error says only that a choice is no option;
// the caller names the question or dialog.
func pickedIDs(a wire.SessionAnswer, nOptions int) (picks []int, other bool, otherAt int, err error) {
	for _, id := range slices.DeleteFunc(append([]string{a.Choice}, a.Choices...), func(id string) bool { return id == "" }) {
		if id == "other" {
			other, otherAt = true, len(picks)
			continue
		}
		n, err := strconv.Atoi(id)
		if err != nil || n < 1 || n > nOptions {
			return nil, false, 0, fmt.Errorf("choice %q is not an option", id)
		}
		picks = append(picks, n-1)
		if !other {
			otherAt = len(picks)
		}
	}
	return picks, other, otherAt, nil
}

// answerClaudeDialog resolves the held AskUserQuestion call of a claude-dialog
// card (already checked as the current one): Release hands it to the terminal
// dialog, an answer with nothing picked denies it, and anything else must
// answer every question. A card with no held call is a fallback card, answered
// by keys.
func (p *Prompt) answerClaudeDialog(req wire.SessionAnswerRequest, dir string) wire.SessionResult {
	stale := sessions.Refusal(wire.ReasonStale, "the question moved on; review it first")
	var held *wire.AskHookRequest
	for _, h := range p.sessions.HeldAsks(req.ID) {
		if h.ToolUseID == req.CardID {
			held = &h
		}
	}
	if held == nil {
		return p.answerClaudeKeys(req, dir)
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
		cq := question{Question: q.Question, Header: q.Header, MultiSelect: q.MultiSelect, Options: []option{}}
		for _, o := range q.Options {
			cq.Options = append(cq.Options, option{o.Label, o.Description})
		}
		picks, other, otherAt, err := pickedIDs(a, len(q.Options))
		if err != nil {
			return nil, fmt.Sprintf("%v of question %d", err, i+1)
		}
		if other && text == "" {
			return nil, fmt.Sprintf("question %d needs text for Other", i+1)
		}
		var parts []string
		for _, p := range picks {
			parts = append(parts, q.Options[p].Label)
		}
		if text != "" {
			parts = slices.Insert(parts, otherAt, text)
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
