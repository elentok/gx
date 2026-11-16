package triage

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/elentok/gx/tickets"
)

// Round limits: an ordinary round holds at most MaxRoundQuestions questions
// and a Triage has at most MaxOrdinaryRounds of them. An objection round is
// outside the round cap and has the same question cap.
const (
	MaxRoundQuestions = 6
	MaxOrdinaryRounds = 3
)

// Plan actions. ActionBacklog replaces the version 1 "map"; Store.upgrade
// rewrites old records, so no other code sees the old name.
const (
	ActionReady   = "ready"
	ActionBacklog = "backlog"
	ActionClose   = "close"
	legacyMap     = "map"
)

func (rd *Round) unanswered() []Question {
	var out []Question
	for _, q := range rd.Questions {
		if strings.TrimSpace(rd.Answers[q.ID]) == "" {
			out = append(out, q)
		}
	}
	return out
}

// OpenRound reports the first saved round with unanswered questions and those
// questions, so a fresh session reprints it and skips what was answered.
func (r *Record) OpenRound() (index int, open []Question, ok bool) {
	for i := range r.Rounds {
		if qs := r.Rounds[i].unanswered(); len(qs) > 0 {
			return i, qs, true
		}
	}
	return -1, nil, false
}

func (r *Record) ordinaryRounds() int {
	n := 0
	for _, rd := range r.Rounds {
		if !rd.Objection {
			n++
		}
	}
	return n
}

// questionPlaced reports whether any saved round already holds the question.
func (r *Record) questionPlaced(id string) bool {
	for _, rd := range r.Rounds {
		if slices.ContainsFunc(rd.Questions, func(q Question) bool { return q.ID == id }) {
			return true
		}
	}
	return false
}

// Unrounded lists the ask questions of rows that are not gone and not going to
// a backlog that no round holds yet: the candidates for the next round.
func (r *Record) Unrounded() []Question {
	var out []Question
	for _, row := range r.Rows {
		if row.gone() {
			continue
		}
		if action, _ := row.Effective(); action == ActionBacklog {
			continue
		}
		for _, q := range row.AllQuestions() {
			if q.Kind == QuestionAsk && !r.questionPlaced(q.ID) {
				q.RowID = row.Address
				out = append(out, q)
			}
		}
	}
	return out
}

// checkOptions needs 2 to 4 distinct options and a recommended answer among
// them. An ask question always needs them; a design question only when it
// sends options or a recommendation.
func checkOptions(label string, q Question) error {
	if q.Text == "" {
		return phaseErr("%s question %s: the text is empty", label, q.ID)
	}
	if q.Kind != QuestionAsk && len(q.Options) == 0 && q.Default == "" {
		return nil
	}
	opts := map[string]bool{}
	for _, o := range q.Options {
		if o == "" || opts[o] {
			return phaseErr("%s question %s: options must be non-empty and distinct", label, q.ID)
		}
		opts[o] = true
	}
	if len(q.Options) < minOptions || len(q.Options) > maxOptions {
		return phaseErr("%s question %s: a question needs %d to %d options, got %d", label, q.ID, minOptions, maxOptions, len(q.Options))
	}
	if !opts[q.Default] {
		return phaseErr("%s question %s: the recommended answer %q must be one of the options", label, q.ID, q.Default)
	}
	return nil
}

// SaveRound saves a round before it is printed. A decide question never
// reaches a round. An ordinary round is refused past the 3-round cap and while
// another round is open; an objection round is outside the cap.
func (s *Store) SaveRound(id string, revision int, qs []Question, objection bool) (*Record, error) {
	return s.modify(id, revision, func(rec *Record) error {
		if err := requireStep(rec, "round save", tickets.StepQuestions); err != nil {
			return err
		}
		if len(qs) == 0 {
			return phaseErr("a round needs at least one question")
		}
		if len(qs) > MaxRoundQuestions {
			return phaseErr("a round holds at most %d questions, got %d", MaxRoundQuestions, len(qs))
		}
		if _, _, open := rec.OpenRound(); open {
			return phaseErr("a round is still open: answer it first")
		}
		if !objection && rec.ordinaryRounds() >= MaxOrdinaryRounds {
			return phaseErr("at most %d rounds: further questions go to an objection round", MaxOrdinaryRounds)
		}
		rd := Round{Answers: map[string]string{}, Objection: objection}
		for _, q := range qs {
			if q.Kind != QuestionAsk && q.Kind != QuestionDesign {
				return phaseErr("question %s is %q: only ask and design questions go in a round (a decide question is saved as a decision)", q.ID, q.Kind)
			}
			if q.ID == "" || rec.questionPlaced(q.ID) || slices.ContainsFunc(rd.Questions, func(o Question) bool { return o.ID == q.ID }) {
				return phaseErr("question id %q is empty or already in a round", q.ID)
			}
			row, err := rec.row(q.RowID)
			if err != nil {
				return err
			}
			if row.gone() {
				return phaseErr("row %s is gone", q.RowID)
			}
			if err := checkOptions("round", q); err != nil {
				return err
			}
			rd.Questions = append(rd.Questions, q)
		}
		rec.Rounds = append(rec.Rounds, rd)
		return nil
	})
}

// SaveAnswer saves the answer to one question of the open round. The round is
// answered when all its questions are.
func (s *Store) SaveAnswer(id string, revision int, questionID, answer string) (*Record, error) {
	return s.modify(id, revision, func(rec *Record) error {
		if err := requireStep(rec, "answer save", tickets.StepQuestions); err != nil {
			return err
		}
		if strings.TrimSpace(answer) == "" {
			return phaseErr("the answer to %s is empty", questionID)
		}
		open, _, ok := rec.OpenRound()
		for i := range rec.Rounds {
			rd := &rec.Rounds[i]
			if !slices.ContainsFunc(rd.Questions, func(q Question) bool { return q.ID == questionID }) {
				continue
			}
			if !ok || i != open {
				return phaseErr("question %s is not in the open round", questionID)
			}
			if rd.Answers == nil {
				rd.Answers = map[string]string{}
			}
			rd.Answers[questionID] = answer
			rd.Answered = len(rd.unanswered()) == 0
			return nil
		}
		return phaseErr("no question %q in any round", questionID)
	})
}

// SaveDecision stores a question the agent settled itself.
func (s *Store) SaveDecision(id string, revision int, d Decided) (*Record, error) {
	return s.modify(id, revision, func(rec *Record) error {
		if err := requireStep(rec, "decision save", tickets.StepQuestions); err != nil {
			return err
		}
		if _, err := rec.row(d.RowID); err != nil {
			return err
		}
		if d.Question == "" || d.Choice == "" {
			return phaseErr("a decision needs a question and a choice")
		}
		d.Objected = false
		rec.Decided = append(rec.Decided, d)
		return nil
	})
}

// Object marks the person's objection to a decision. The question becomes an
// ask question in an open objection round (created if none is open) with the
// agent's choice as the recommended answer. options are the 2 to 4 answers to
// offer; the choice must be one of them.
func (s *Store) Object(id string, revision int, rowID, question string, options []string) (*Record, error) {
	return s.modify(id, revision, func(rec *Record) error {
		if err := requireStep(rec, "objection", tickets.StepQuestions); err != nil {
			return err
		}
		di := slices.IndexFunc(rec.Decided, func(d Decided) bool { return d.RowID == rowID && d.Question == question })
		if di < 0 {
			return phaseErr("no decision %q for row %s", question, rowID)
		}
		d := &rec.Decided[di]
		if d.Objected {
			return phaseErr("the decision %q is already objected", question)
		}
		n := 0
		for _, rd := range rec.Rounds {
			for _, rq := range rd.Questions {
				if k, ok := strings.CutPrefix(rq.ID, "obj-"); ok {
					if v, err := strconv.Atoi(k); err == nil {
						n = max(n, v)
					}
				}
			}
		}
		q := Question{ID: fmt.Sprintf("obj-%d", n+1), RowID: rowID, Text: question, Kind: QuestionAsk, Options: options, Default: d.Choice}
		if err := checkOptions("objection", q); err != nil {
			return err
		}
		ri := slices.IndexFunc(rec.Rounds, func(rd Round) bool { return rd.Objection && len(rd.unanswered()) > 0 })
		if ri < 0 {
			rec.Rounds = append(rec.Rounds, Round{Answers: map[string]string{}, Objection: true})
			ri = len(rec.Rounds) - 1
		}
		rd := &rec.Rounds[ri]
		if len(rd.Questions) >= MaxRoundQuestions {
			return phaseErr("an objection round holds at most %d questions", MaxRoundQuestions)
		}
		rd.Questions = append(rd.Questions, q)
		rd.Answered = false
		d.Objected = true
		return nil
	})
}

// Correct moves a row to a backlog or to the ready epic after the person
// corrected its routing. A move to a backlog drops the row's unanswered
// questions from every round and keeps the answers given. A move to ready
// needs a fallback backlog; the row's ask questions then count as unrounded again.
func (s *Store) Correct(id string, revision int, address, action, destination, fallback string) (*Record, error) {
	return s.modify(id, revision, func(rec *Record) error {
		if err := requireStep(rec, "routing correction", tickets.StepQuestions); err != nil {
			return err
		}
		row, err := rec.row(address)
		if err != nil {
			return err
		}
		if row.gone() {
			return phaseErr("row %s is gone", address)
		}
		switch action {
		case ActionBacklog:
			if destination == "" {
				return phaseErr("a move to a backlog needs the backlog name")
			}
			if err := rec.checkBacklogCap(s.projectDir, destination); err != nil {
				return err
			}
			rec.dropUnanswered(address)
			row.Person.Action, row.Person.Destination = ActionBacklog, destination
		case ActionReady:
			if fallback == "" {
				return phaseErr("a move to ready needs a fallback backlog")
			}
			row.Fallback = fallback
			row.Person.Action, row.Person.Destination = ActionReady, ""
		default:
			return phaseErr("a routing correction goes to %q or ready, not %q", ActionBacklog, action)
		}
		if action != ActionReady && row.Group != "" {
			rec.dissolveGroup(row.Group)
		}
		return nil
	})
}

// dropUnanswered removes the row's unanswered questions from every round.
func (r *Record) dropUnanswered(address string) {
	for i := range r.Rounds {
		rd := &r.Rounds[i]
		rd.Questions = slices.DeleteFunc(rd.Questions, func(q Question) bool {
			return q.RowID == address && strings.TrimSpace(rd.Answers[q.ID]) == ""
		})
		rd.Answered = len(rd.unanswered()) == 0
	}
}

// SaveBacklogs stores backlog entries during Route, before the plan, and the
// person's confirmation of an agent backlog during Questions. An entry with
// the name of a saved one replaces it. A confirmed entry that would be a 13th
// backlog is refused.
func (s *Store) SaveBacklogs(id string, revision int, backlogs []Backlog) (*Record, error) {
	return s.modify(id, revision, func(rec *Record) error {
		if err := requireStep(rec, "backlog save", tickets.StepRoute, tickets.StepQuestions); err != nil {
			return err
		}
		for _, a := range backlogs {
			if a.Name == "" || (a.Source != BacklogTable && a.Source != BacklogAgent) {
				return phaseErr("backlog %q needs a name and source table or agent", a.Name)
			}
			if a.Confirmed {
				if err := rec.checkBacklogCap(s.projectDir, a.Name); err != nil {
					return err
				}
			}
			if i := slices.IndexFunc(rec.Backlogs, func(o Backlog) bool { return o.Name == a.Name }); i >= 0 {
				rec.Backlogs[i] = a
			} else {
				rec.Backlogs = append(rec.Backlogs, a)
			}
		}
		return nil
	})
}

// backlogNames is the backlog epics on disk plus the backlogs this record saved.
func (r *Record) backlogNames(projectDir string) []string {
	names, _ := tickets.BacklogNames(projectDir)
	for _, a := range r.Backlogs {
		if !slices.Contains(names, a.Name) {
			names = append(names, a.Name)
		}
	}
	return names
}

// checkBacklogCap refuses name when it would be a new backlog past the cap.
func (r *Record) checkBacklogCap(projectDir, name string) error {
	names := r.backlogNames(projectDir)
	if !slices.Contains(names, name) && len(names) >= tickets.MaxBacklogs {
		return phaseErr("backlog %q would be backlog %d: a project has at most %d backlogs", name, len(names)+1, tickets.MaxBacklogs)
	}
	return nil
}

// unconfirmedBacklogs lists the agent backlogs the person has not confirmed. A table
// backlog counts as confirmed by the final card.
func (r *Record) unconfirmedBacklogs() []string {
	var out []string
	for _, a := range r.Backlogs {
		if a.Source == BacklogAgent && !a.Confirmed {
			out = append(out, a.Name)
		}
	}
	return out
}
