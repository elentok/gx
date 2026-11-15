package triage

import (
	"fmt"
	"strings"
)

// Ticket types a row is written as.
const (
	TypeGrilling  = "grilling"
	TypeResearch  = "research"
	TypeImplement = "implement"
)

// WriteTarget is where the Write step puts a row's ticket.
type WriteTarget struct {
	Epic string `json:"epic,omitempty"`
	Type string `json:"type,omitempty"`
	// Question is the text for the ticket's ## Question section; set only for
	// a grilling ticket.
	Question string `json:"question,omitempty"`
}

// WriteTarget gives the epic, ticket type and question text for a row that goes
// to an area or the ready epic; ok is false for any other action.
//
// A row whose ask questions no round reached goes to an area as a grilling
// ticket: a ready row at once (to its fallback area), an area row only once the
// ordinary rounds are used up.
func (r *Record) WriteTarget(row *Row) (t WriteTarget, ok bool) {
	action, dest := row.Effective()
	var unreached []string
	for _, q := range row.AllQuestions() {
		if q.Kind == QuestionAsk && !r.questionPlaced(q.ID) && strings.TrimSpace(row.Person.Answers[q.ID]) == "" {
			unreached = append(unreached, fmt.Sprintf("- %s", q.Text))
		}
	}
	switch action {
	case "ready":
		t.Epic = r.ReadyEpic
		if len(unreached) > 0 {
			t.Epic = row.fallbackArea()
		}
	case ActionArea, "map":
		t.Epic = dest
		if len(unreached) > 0 && r.ordinaryRounds() < MaxOrdinaryRounds {
			unreached = nil
		}
	default:
		return WriteTarget{}, false
	}
	switch {
	case len(unreached) > 0:
		t.Type, t.Question = TypeGrilling, strings.Join(unreached, "\n")
	case row.Category == TypeResearch:
		t.Type = TypeResearch
	default:
		t.Type = TypeImplement
	}
	return t, true
}

func (r *Row) fallbackArea() string {
	if r.Fallback != "" {
		return r.Fallback
	}
	if r.Plan != nil {
		return r.Plan.Fallback
	}
	return ""
}

// DecisionLines are the row's answered questions of the saved rounds, then
// the questions the agent settled itself, as Decisions lines.
func (r *Record) DecisionLines(row *Row) []string {
	var out []string
	for _, rd := range r.Rounds {
		for _, q := range rd.Questions {
			if a := strings.TrimSpace(rd.Answers[q.ID]); q.RowID == row.Address && a != "" {
				out = append(out, fmt.Sprintf("- %s: %s", q.Text, a))
			}
		}
	}
	for _, q := range row.AllQuestions() {
		if a, ok := row.Person.Answers[q.ID]; ok && strings.TrimSpace(a) != "" {
			out = append(out, fmt.Sprintf("- %s: %s", q.Text, strings.TrimSpace(a)))
		}
	}
	for _, d := range r.Decided {
		if d.RowID == row.Address {
			out = append(out, fmt.Sprintf("- %s: %s (decided by agent)", d.Question, d.Choice))
		}
	}
	return out
}
