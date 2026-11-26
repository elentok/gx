package prompt

import (
	"cmp"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/elentok/gx/server/sessions"
	"github.com/elentok/gx/server/sessions/prompt/dialogkeys"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/server/wire/ops"
)

// dismissReason is what Claude reads when the person dismisses a held question.
const dismissReason = "The user doesn't want to proceed with this question. Do not ask again. STOP and wait for the user to tell you how to proceed."

// answerClaudeDialog resolves the held AskUserQuestion call of a claude-dialog
// card (already checked as the current one): Release hands it to the terminal
// dialog, an answer with nothing picked denies it, and anything else must
// answer every question. A card with no held call is a fallback card, answered
// by keys.
func (p *Prompt) answerClaudeDialog(req wire.SessionAnswerRequest, dir string) wire.SessionResult {
	stale := sessions.Refusal(wire.ReasonStale, "the question moved on; review it first")
	var held *ops.AskHookRequest
	for _, h := range p.sessions.HeldAsks(req.ID) {
		if h.ToolUseID == req.CardID {
			held = &h
		}
	}
	if held == nil {
		return dialogkeys.Answer(req, dir, dialogkeys.Config{
			Log:    p.log,
			Step:   cmp.Or(p.cfg.KeysSettleStep, DefaultKeysSettleStep),
			Cap:    cmp.Or(p.cfg.KeysSettleCap, DefaultKeysSettleCap),
			Settle: SettleScreen,
		})
	}
	var res ops.AskHookResult
	switch {
	case req.Release:
		if !p.sessions.ReleaseAsk(req.ID, req.CardID) {
			return stale
		}
		return wire.SessionResult{}
	case !slices.ContainsFunc(req.Answers, dialogkeys.Answered):
		res = ops.AskHookResult{Decision: ops.AskHookDeny, Reason: dismissReason}
	default:
		input, refusal := claudeUpdatedInput(*held, req.Answers)
		if refusal != "" {
			return sessions.Refusal(wire.ReasonSessionBadRequest, refusal)
		}
		res = ops.AskHookResult{Decision: ops.AskHookAllow, UpdatedInput: input}
	}
	if !p.sessions.ResolveAsk(req.ID, req.CardID, res) {
		return stale
	}
	return wire.SessionResult{}
}

// claudeUpdatedInput is the AskUserQuestion input that answers the call: the
// original questions, answers keyed by question text (multi-select labels
// joined with ", "), and the per-question notes as annotations. A refusal
// message says which question is not answered right.
func claudeUpdatedInput(held ops.AskHookRequest, answers []wire.SessionAnswer) (json.RawMessage, string) {
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
		picks, err := dialogkeys.PickedIDs(a, len(q.Options))
		if err != nil {
			return nil, fmt.Sprintf("%v of question %d", err, i+1)
		}
		other := slices.Contains(picks, dialogkeys.PickOther)
		if other && text == "" {
			return nil, fmt.Sprintf("question %d needs text for Other", i+1)
		}
		var parts []string
		for _, p := range picks {
			if p == dialogkeys.PickOther {
				parts = append(parts, text)
			} else {
				parts = append(parts, q.Options[p].Label)
			}
		}
		if text != "" && !other {
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
