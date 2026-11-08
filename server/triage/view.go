package triage

import (
	"fmt"
	"net/http"
	"slices"
	"sort"
	"strings"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
	triagerec "github.com/elentok/gx/triage"
)

// flowSteps are the Triage steps the page shows, in order.
var flowSteps = []struct {
	name string
	step wire.ProgressStep
}{
	{tickets.StepVerify, wire.ProgressVerify},
	{tickets.StepPlan, wire.ProgressPlan},
	{tickets.StepApprove, wire.ProgressApprove},
	{tickets.StepWrite, wire.ProgressWrite},
	{string(tickets.StepDone), wire.ProgressDone},
}

// The offer rules live here and nowhere else.

// CanKeep is whether a row offers Keep for later: it failed at write and its
// follow-up is still in follow-ups.
func CanKeep(live bool, step string, r triagerec.Row) bool {
	return live && step == tickets.StepWrite && r.Write == triagerec.WriteFailed &&
		r.NewAddress == "" && !r.Person.KeepForLater
}

// CanRetry is whether a row offers Retry: any failed row at write, including
// one that failed after its move.
func CanRetry(live bool, step string, r triagerec.Row) bool {
	return live && step == tickets.StepWrite && r.Write == triagerec.WriteFailed
}

// CanAbandon is whether the Triage offers Abandon. Never at write. When the
// Triage session is stopped, only while the stored step is verify or plan.
func CanAbandon(live bool, step string, stopped bool) bool {
	if !live {
		return false
	}
	switch step {
	case tickets.StepVerify, tickets.StepPlan:
		return true
	case tickets.StepApprove:
		return !stopped
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
	v := buildView(project, rec, t.stopped(project))
	v.Destinations = t.destinations(project, rec)
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

// stopped reports whether the project has a Triage session and all of them
// have exited. No session at all is not stopped.
func (t *Triage) stopped(project string) bool {
	if t.sessions == nil {
		return false
	}
	infos, err := t.sessions()
	if err != nil {
		return false
	}
	found := false
	for _, s := range infos {
		if s.Flow != string(tickets.FlowTriage) || s.FlowKey != project {
			continue
		}
		if s.Status.State != wire.SessionExited {
			return false
		}
		found = true
	}
	return found
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

func buildView(project string, rec *triagerec.Record, stopped bool) wire.TriageView {
	v := wire.TriageView{
		Project: project, State: wire.TriageNone, Flow: string(tickets.FlowTriage),
		Rows: []wire.TriageRow{}, ActionCounts: map[string]int{}, Destinations: []wire.TriageDestination{},
		History: []wire.TriageHistoryItem{},
	}
	if rec == nil {
		v.Steps = steps("")
		return v
	}
	isLive := rec.State == triagerec.StateLive
	v.ID, v.State, v.Revision = rec.ID, wire.TriageState(rec.State), rec.Revision
	v.Steps = steps(rec.Step)
	v.CanAbandon = CanAbandon(isLive, rec.Step, stopped)

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
	case tickets.StepPlan:
		idx, within, note = 1, frac(plans), fmt.Sprintf("%d of %d rows planned", plans, total)
	case tickets.StepApprove:
		idx = 2
	case tickets.StepWrite:
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
		WriteState: string(r.Write), NewAddress: r.NewAddress, Refusal: r.Refusal,
		CanKeep: CanKeep(live, rec.Step, r), CanRetry: CanRetry(live, rec.Step, r),
	}
	if r.Plan != nil {
		row.Action, row.Destination = r.Plan.Action, r.Plan.Destination
	}
	open := 0
	for _, q := range r.AllQuestions() {
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
	order := []string{"ready", "map", "merge", "close", "keep"}
	by := map[string][]wire.TriageResultItem{}
	res := wire.TriageResult{Groups: []wire.TriageResultGroup{}}
	for _, r := range rec.Rows {
		row, _, _ := buildRow(rec, r, false)
		if row.Gone || (r.Write != triagerec.WriteDone && !r.Person.KeepForLater) {
			continue
		}
		item := wire.TriageResultItem{Address: r.Address, Title: r.Title}
		switch row.Action {
		case "ready", "map":
			item.Target = epicOf(r.NewAddress)
			if row.Action == "ready" && res.ReadyEpic == "" {
				res.ReadyEpic = item.Target
			}
		case "merge":
			item.Target = row.Destination
		case "close":
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
