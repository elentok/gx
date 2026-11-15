package triage

import (
	"fmt"
	"os"
	"path/filepath"
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

// Plan actions. ActionArea replaces the version 1 "map"; Store.upgrade
// rewrites old records, so no other code sees the old name.
const (
	ActionReady = "ready"
	ActionArea  = "area"
	ActionClose = "close"
	legacyMap   = "map"
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
// an area that no round holds yet: the candidates for the next round.
func (r *Record) Unrounded() []Question {
	var out []Question
	for _, row := range r.Rows {
		if row.gone() {
			continue
		}
		if action, _ := row.Effective(); action == ActionArea {
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

// Correct moves a row to an area or to the ready epic after the person
// corrected its routing. A move to an area drops the row's unanswered
// questions from every round and keeps the answers given. A move to ready
// needs a fallback area; the row's ask questions then count as unrounded again.
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
		case ActionArea:
			if destination == "" {
				return phaseErr("a move to an area needs the area name")
			}
			if err := rec.checkAreaCap(s.projectDir, destination); err != nil {
				return err
			}
			rec.dropUnanswered(address)
			row.Person.Action, row.Person.Destination = ActionArea, destination
		case ActionReady:
			if fallback == "" {
				return phaseErr("a move to ready needs a fallback area")
			}
			row.Fallback = fallback
			row.Person.Action, row.Person.Destination = ActionReady, ""
		default:
			return phaseErr("a routing correction goes to %q or ready, not %q", ActionArea, action)
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

// SaveAreas stores area entries during Route, before the plan, and the
// person's confirmation of an agent area during Questions. An entry with
// the name of a saved one replaces it. A confirmed entry that would be a 13th
// area is refused.
func (s *Store) SaveAreas(id string, revision int, areas []Area) (*Record, error) {
	return s.modify(id, revision, func(rec *Record) error {
		if err := requireStep(rec, "area save", tickets.StepRoute, tickets.StepQuestions); err != nil {
			return err
		}
		for _, a := range areas {
			if a.Name == "" || (a.Source != AreaTable && a.Source != AreaAgent) {
				return phaseErr("area %q needs a name and source table or agent", a.Name)
			}
			if a.Confirmed {
				if err := rec.checkAreaCap(s.projectDir, a.Name); err != nil {
					return err
				}
			}
			if i := slices.IndexFunc(rec.Areas, func(o Area) bool { return o.Name == a.Name }); i >= 0 {
				rec.Areas[i] = a
			} else {
				rec.Areas = append(rec.Areas, a)
			}
		}
		return nil
	})
}

// areaNames is the area epics on disk plus the areas this record saved.
func (r *Record) areaNames(projectDir string) []string {
	var names []string
	if entries, err := os.ReadDir(projectDir); err == nil {
		for _, e := range entries {
			if e.IsDir() && !strings.HasPrefix(e.Name(), ".") && tickets.IsAreaEpic(filepath.Join(projectDir, e.Name())) {
				names = append(names, e.Name())
			}
		}
	}
	for _, a := range r.Areas {
		if !slices.Contains(names, a.Name) {
			names = append(names, a.Name)
		}
	}
	return names
}

// checkAreaCap refuses name when it would be a new area past the cap.
func (r *Record) checkAreaCap(projectDir, name string) error {
	names := r.areaNames(projectDir)
	if !slices.Contains(names, name) && len(names) >= tickets.MaxAreas {
		return phaseErr("area %q would be area %d: a project has at most %d areas", name, len(names)+1, tickets.MaxAreas)
	}
	return nil
}

// unconfirmedAreas lists the agent areas the person has not confirmed. A table
// area counts as confirmed by the final card.
func (r *Record) unconfirmedAreas() []string {
	var out []string
	for _, a := range r.Areas {
		if a.Source == AreaAgent && !a.Confirmed {
			out = append(out, a.Name)
		}
	}
	return out
}
