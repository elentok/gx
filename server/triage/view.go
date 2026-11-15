package triage

import (
	"fmt"
	"net/http"
	"slices"
	"sort"
	"strings"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
	triagerec "github.com/elentok/gx/triage"
)

// flowSteps are the Triage steps the page shows, in order.
var flowSteps = []struct {
	name string
	step wire.ProgressStep
}{
	{tickets.StepVerify, wire.ProgressVerify},
	{tickets.StepRoute, wire.ProgressRoute},
	{tickets.StepQuestions, wire.ProgressQuestions},
	{tickets.StepWrite, wire.ProgressWrite},
	{string(tickets.StepDone), wire.ProgressDone},
}

// The offer rules live here and nowhere else.

// CanKeep is whether a row offers Keep for later: it failed at write and its
// follow-up is still in follow-ups.
func CanKeep(live bool, step string, r triagerec.Row) bool {
	return live && step == tickets.StepWrite && r.Write == triagerec.WriteFailed &&
		len(r.NewAddresses) == 0 && !r.Person.KeepForLater
}

// CanRetry is whether a row offers Retry: any failed row at write, including
// one that failed after its move.
func CanRetry(live bool, step string, r triagerec.Row) bool {
	return live && step == tickets.StepWrite && r.Write == triagerec.WriteFailed
}

// CanRetryMove is whether a move unit offers Retry and Keep for later: it
// failed at write. Both offers share one rule because the store accepts both
// for any failed unit.
func CanRetryMove(live bool, step string, m triagerec.MoveUnit) bool {
	return live && step == tickets.StepWrite && m.State == triagerec.MoveFailed
}

// CanAbandon is whether the Triage offers Abandon: at verify, route and
// questions, never at write, except for a version 1 record at write, where it
// is the only action.
func CanAbandon(live bool, step string, legacyWrite bool) bool {
	if !live {
		return false
	}
	switch step {
	case tickets.StepVerify, tickets.StepRoute, tickets.StepQuestions:
		return true
	case tickets.StepWrite:
		return legacyWrite
	}
	return false
}

func (t *Triage) readHandler(w http.ResponseWriter, r *http.Request) {
	project := r.URL.Query().Get("project")
	view, err := t.View(project)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	wire.WriteJSON(w, view)
}

// View builds the payload of a project.
func (t *Triage) View(project string) (wire.TriageView, error) {
	dir, err := t.projects.Dir(project)
	if err != nil {
		return wire.TriageView{}, err
	}
	store := triagerec.NewStore(dir)
	live, err := store.LoadLive()
	if err != nil {
		return wire.TriageView{}, err
	}
	past, err := store.History()
	if err != nil {
		return wire.TriageView{}, err
	}
	rec := live
	if rec == nil && len(past) > 0 {
		rec = past[0]
	}
	v := buildView(project, rec)
	v.Destinations = t.destinations(project, rec)
	if rec != nil && rec.State == triagerec.StateLive {
		v.Summary.Waiting = t.waitingLines(project)
	}
	v.History = []wire.TriageHistoryItem{}
	for _, p := range past {
		res := resultOf(p)
		v.History = append(v.History, wire.TriageHistoryItem{
			ID: p.ID, State: wire.TriageState(p.State), StartedAt: p.StartedAt.Format("2006-01-02T15:04:05Z07:00"),
			Rows: len(p.Rows), Result: res,
		})
	}
	return v, nil
}

// waitingLines lists the old epics that have nothing to move and hold a claimed
// ticket: the Triage leaves them out and they wait. Read from the store, as the
// move units only name epics that can move.
func (t *Triage) waitingLines(project string) []string {
	out := []string{}
	epics, err := t.projects.Epics(project)
	if err != nil {
		return out
	}
	for _, e := range epics {
		if !triagerec.IsOldEpic(e) {
			continue
		}
		var claimed []string
		movable := false
		for _, tk := range e.Tickets {
			switch schema.Status(tk.Status) {
			case schema.StatusOpen, schema.StatusDraft:
				movable = true
			case schema.StatusClaimed:
				claimed = append(claimed, tickets.Address{Project: project, Epic: e.Name, ID: tk.Identifier}.String())
			}
		}
		if !movable && len(claimed) > 0 {
			out = append(out, fmt.Sprintf("waiting: %s holds claimed %s", e.Name, strings.Join(claimed, ", ")))
		}
	}
	return out
}

// destinations lists the choices for a row's destination: the epics the
// verdicts suggested, the project's open map epics, and a new map epic.
func (t *Triage) destinations(project string, rec *triagerec.Record) []wire.TriageDestination {
	out := []wire.TriageDestination{}
	seen := map[string]bool{}
	if rec != nil {
		var sug []string
		for _, r := range rec.Rows {
			if r.SuggestedDestination != "" && !seen[r.SuggestedDestination] {
				seen[r.SuggestedDestination] = true
				sug = append(sug, r.SuggestedDestination)
			}
		}
		sort.Strings(sug)
		for _, s := range sug {
			out = append(out, wire.TriageDestination{Kind: "suggested", Value: s, Label: s})
		}
	}
	epics, err := t.projects.Epics(project)
	if err == nil {
		var maps []string
		for _, e := range epics {
			name := e.Name[strings.LastIndex(e.Name, ":")+1:]
			if e.IsMap && e.QueueRefusal() == tickets.QueueRefusalNone && !seen[name] {
				maps = append(maps, name)
			}
		}
		sort.Strings(maps)
		for _, name := range maps {
			out = append(out, wire.TriageDestination{Kind: "map-epic", Value: name, Label: name})
		}
	}
	return append(out, wire.TriageDestination{Kind: "new-map-epic", Label: "New map epic"})
}

func buildView(project string, rec *triagerec.Record) wire.TriageView {
	v := wire.TriageView{
		Project: project, State: wire.TriageNone, Flow: string(tickets.FlowTriage),
		Rows: []wire.TriageRow{}, ActionCounts: map[string]int{}, Destinations: []wire.TriageDestination{},
		EpicActions: triagerec.EpicActions,
		History:     []wire.TriageHistoryItem{},
		Summary: &wire.TriageSummary{
			Actions: map[string]int{}, Areas: map[string]int{}, Moves: []wire.TriageMove{}, Waiting: []string{},
		},
	}
	if rec == nil {
		v.Steps = steps("")
		return v
	}
	isLive := rec.State == triagerec.StateLive
	v.ID, v.State, v.Revision = rec.ID, wire.TriageState(rec.State), rec.Revision
	v.Steps = steps(rec.Step)
	v.LegacyWrite = rec.LegacyWrite()
	v.CanAbandon = CanAbandon(isLive, rec.Step, v.LegacyWrite)
	if isLive && rec.Step == tickets.StepQuestions {
		v.OpenRound, v.FinalApproval = openRound(rec)
	}
	for _, m := range rec.Moves {
		can := CanRetryMove(isLive, rec.Step, m)
		v.Summary.Moves = append(v.Summary.Moves, wire.TriageMove{
			OldEpic: m.OldEpic, Area: m.Area, State: string(m.State), LastError: m.LastError,
			CanRetry: can, CanKeep: can,
		})
	}

	orphans := rec.MergeOrphans()
	var verdicts, plans, written int
	for _, r := range rec.Rows {
		row, open, changed := buildRow(rec, r, isLive)
		row.MergeOrphan = isLive && slices.Contains(orphans, r.Address)
		v.Rows = append(v.Rows, row)
		v.OpenQuestions += open
		if changed {
			v.ChangedRows++
		}
		if r.Write == triagerec.WriteGone {
			verdicts++
			plans++
			written++
			continue
		}
		if action, dest := r.Effective(); action != "" {
			v.Summary.Actions[action]++
			if action == triagerec.ActionArea && dest != "" {
				v.Summary.Areas[dest]++
			}
		}
		if r.Verdict != "" {
			verdicts++
		}
		if r.Plan != nil {
			plans++
			v.ActionCounts[row.Action]++
		}
		if r.Write == triagerec.WriteDone || r.Person.KeepForLater {
			written++
		}
	}
	total := len(rec.Rows)
	v.StepNote, v.RingPercent = progress(rec, total, verdicts, plans, written)
	res := resultOf(rec)
	v.ReadyEpic = res.ReadyEpic
	if rec.State == triagerec.StateDone {
		v.Result = &res
	}
	return v
}

// openRound reports the first saved round that waits for an answer, and the
// final-approval marker: no round open and no ask question left to round.
func openRound(rec *triagerec.Record) (*wire.TriageOpenRound, bool) {
	index, open, ok := rec.OpenRound()
	if ok {
		round := rec.Rounds[index]
		return &wire.TriageOpenRound{
			Number: index + 1, Questions: len(round.Questions), Answered: len(round.Questions) - len(open),
		}, false
	}
	return nil, !rec.Approved && len(rec.Unrounded()) == 0
}

func steps(current string) []wire.MapStep {
	out := make([]wire.MapStep, 0, len(flowSteps))
	state := wire.MapStepDone
	if current == "" {
		state = wire.MapStepTodo
	}
	spec, _ := tickets.FlowSpecFor(tickets.FlowTriage)
	for _, s := range flowSteps {
		st := state
		switch {
		case current == string(tickets.StepDone):
		case s.name == current:
			st, state = wire.MapStepCurrent, wire.MapStepTodo
		}
		out = append(out, wire.MapStep{Step: s.step, Label: spec.Labels[s.name], State: st})
	}
	return out
}

func progress(rec *triagerec.Record, total, verdicts, plans, written int) (string, int) {
	frac := func(n int) float64 {
		if total == 0 {
			return 1
		}
		return float64(n) / float64(total)
	}
	idx, within, note := 0, 0.0, ""
	switch rec.Step {
	case tickets.StepVerify:
		within, note = frac(verdicts), fmt.Sprintf("%d of %d verdicts saved", verdicts, total)
	case tickets.StepRoute:
		idx, within, note = 1, frac(plans), fmt.Sprintf("%d of %d rows planned", plans, total)
	case tickets.StepQuestions:
		idx = 2
	case tickets.StepWrite:
		if rec.LegacyWrite() {
			return "Waiting for you", 75
		}
		idx, within, note = 3, frac(written), fmt.Sprintf("rows written %d of %d", written, total)
	default:
		return "", 100
	}
	return note, int((float64(idx) + within) * 100 / 4)
}

func buildRow(rec *triagerec.Record, r triagerec.Row, live bool) (wire.TriageRow, int, bool) {
	row := wire.TriageRow{
		Address: r.Address, Title: r.Title, Category: r.Category,
		Verdict: r.Verdict, Evidence: r.Evidence, Files: r.Files,
		Answers: r.Person.Answers, Gone: r.Write == triagerec.WriteGone,
		WriteState: string(r.Write), NewAddresses: r.NewAddresses, Refusal: r.Refusal,
		CanKeep: CanKeep(live, rec.Step, r), CanRetry: CanRetry(live, rec.Step, r),
	}
	if r.Plan != nil {
		row.Action, row.Destination = r.Plan.Action, r.Plan.Destination
		row.OverlapWith = r.Plan.OverlapWith
	}
	row.Group = r.Group
	open := 0
	for _, q := range r.AllQuestions() {
		if q.Kind == triagerec.QuestionDesign {
			row.Travels = append(row.Travels, wireQuestion(q))
			continue
		}
		row.Questions = append(row.Questions, wireQuestion(q))
		if !row.Gone && r.Person.Answers[q.ID] == "" {
			open++
		}
	}
	changed := false
	if r.Person.Action != "" && (r.Plan == nil || r.Person.Action != r.Plan.Action) {
		row.Action, changed = r.Person.Action, true
	}
	if r.Person.Destination != "" && (r.Plan == nil || r.Person.Destination != r.Plan.Destination) {
		row.Destination, changed = r.Person.Destination, true
	}
	if r.Person.KeepForLater {
		row.Action = "keep"
	}
	row.PersonChanged = changed
	return row, open, changed
}

func wireQuestion(q triagerec.Question) wire.TriageQuestion {
	title, body, _ := strings.Cut(q.Text, "\n")
	return wire.TriageQuestion{
		ID: q.ID, Title: title, Body: strings.TrimSpace(body), Kind: string(q.Kind),
		Options: q.Options, Recommended: q.Default,
	}
}

// resultOf groups the settled rows of rec by their action. A row counts once
// it is written or kept.
func resultOf(rec *triagerec.Record) wire.TriageResult {
	order := []string{triagerec.ActionReady, triagerec.ActionArea, "merge", triagerec.ActionClose, "keep"}
	by := map[string][]wire.TriageResultItem{}
	res := wire.TriageResult{Groups: []wire.TriageResultGroup{}}
	for _, r := range rec.Rows {
		row, _, _ := buildRow(rec, r, false)
		if row.Gone || (r.Write != triagerec.WriteDone && !r.Person.KeepForLater) {
			continue
		}
		item := wire.TriageResultItem{Address: r.Address, Title: r.Title}
		switch row.Action {
		case triagerec.ActionReady, triagerec.ActionArea:
			item.Target = epicOf(r.FirstAddress())
			if row.Action == triagerec.ActionReady && res.ReadyEpic == "" {
				res.ReadyEpic = item.Target
			}
		case "merge":
			item.Target = row.Destination
		case triagerec.ActionClose:
			item.Reason = row.Destination
		}
		by[row.Action] = append(by[row.Action], item)
	}
	for _, a := range order {
		if items := by[a]; len(items) > 0 {
			res.Groups = append(res.Groups, wire.TriageResultGroup{Action: a, Items: items})
		}
	}
	return res
}

// epicOf is the project:epic part of a ticket address.
func epicOf(addr string) string {
	if i := strings.LastIndex(addr, "/"); i >= 0 {
		return addr[:i]
	}
	return addr
}
