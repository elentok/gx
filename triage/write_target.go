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
// to a backlog or the ready epic; ok is false for any other action.
//
// A row whose ask questions no round reached goes to a backlog as a grilling
// ticket, a ready row to its fallback backlog. A backlog row with a design question
// is grilling too; a backlog row marked investigate is research.
func (r *Record) WriteTarget(row *Row) (t WriteTarget, ok bool) {
	action, dest := row.Effective()
	var unreached []string
	for _, q := range row.AllQuestions() {
		kind := q.Kind
		if (kind == QuestionAsk || (kind == QuestionDesign && action == ActionBacklog)) &&
			!r.questionPlaced(q.ID) && strings.TrimSpace(row.Person.Answers[q.ID]) == "" {
			unreached = append(unreached, fmt.Sprintf("- %s", q.Text))
		}
	}
	switch action {
	case ActionReady:
		t.Epic = r.ReadyEpic
		if len(unreached) > 0 {
			t.Epic = row.fallbackBacklog()
		}
	case ActionBacklog:
		t.Epic = dest
	default:
		return WriteTarget{}, false
	}
	switch {
	case len(unreached) > 0:
		t.Type, t.Question = TypeGrilling, strings.Join(unreached, "\n")
	case action == ActionBacklog && row.Plan != nil && row.Plan.Investigate:
		t.Type = TypeResearch
	default:
		t.Type = TypeImplement
	}
	return t, true
}

func (r *Row) fallbackBacklog() string {
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
