package epics

import (
	"errors"
	"net/http"
	"slices"

	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/initiative"
	"github.com/elentok/gx/tickets/schema"
)

// Initiatives serves the Initiatives list: per project with an Initiative, one
// row each with its state, done counts and current Milestone.
func (s *Epics) Initiatives(w http.ResponseWriter, _ *http.Request) {
	dirs, err := tickets.ProjectDirs(s.TicketStore)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	list, err := s.cardList()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	workspacesOf := s.projectWorkspaces(list.Epics)
	out := wire.InitiativeList{Projects: []wire.InitiativeProject{}}
	for _, dir := range dirs {
		project := tickets.ProjectName(dir)
		live, err := s.Projects.Epics(project)
		if err != nil {
			s.Log.Warn("initiatives: read project epics", "project", project, "err", err)
			continue
		}
		archived, err := tickets.LoadArchived(dir)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		var rows []wire.InitiativeRow
		for _, e := range live {
			if !e.IsInitiative {
				continue
			}
			v := initiative.Read(e, live, archived)
			row := wire.InitiativeRow{
				Address: queue.RootRef{Project: project, Epic: e.Name}.String(), Slug: v.Slug, State: v.State,
				Done: v.Done, Total: v.Total, Current: v.Current, StoppedAt: v.StoppedAt,
			}
			for _, m := range v.Milestones {
				if m.Slug == v.Current {
					row.CurrentNumber, row.CurrentState = m.Number, string(m.State)
				}
			}
			rows = append(rows, row)
		}
		if len(rows) > 0 {
			out.Projects = append(out.Projects, wire.InitiativeProject{Project: project, Workspaces: workspacesOf(project), Initiatives: rows})
		}
	}
	wire.WriteJSON(w, out)
}

// InitiativeDetail serves one Initiative's page data. An unknown address, or
// one that is not an Initiative, answers 404.
func (s *Epics) InitiativeDetail(w http.ResponseWriter, r *http.Request) {
	address := r.URL.Query().Get("address")
	root, err := queue.ParseRootRef(address)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	live, err := s.Projects.Epics(root.Project)
	if _, ok := errors.AsType[*tickets.AddressError](err); ok {
		http.Error(w, "no initiative "+address, http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	i := slices.IndexFunc(live, func(e tickets.Epic) bool { return e.Name == root.Epic && e.IsInitiative })
	if i < 0 {
		http.Error(w, "no initiative "+address, http.StatusNotFound)
		return
	}
	archived, err := s.archivedOf(root.Project)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	wire.WriteJSON(w, initiativePage(root.Project, live[i], live, archived))
}

func (s *Epics) archivedOf(project string) ([]tickets.Epic, error) {
	dirs, err := tickets.ProjectDirs(s.TicketStore)
	if err != nil {
		return nil, err
	}
	for _, dir := range dirs {
		if tickets.ProjectName(dir) == project {
			return tickets.LoadArchived(dir)
		}
	}
	return nil, nil
}

func initiativePage(project string, e tickets.Epic, live, archived []tickets.Epic) wire.InitiativePage {
	v := initiative.Read(e, live, archived)
	body := initiative.ParseBody(e.Body)
	addr := func(name string) string { return queue.RootRef{Project: project, Epic: name}.String() }
	p := wire.InitiativePage{
		Address: addr(e.Name), Slug: v.Slug, State: v.State, StoppedAt: v.StoppedAt,
		Destination: schema.Section(e.Body, "Destination"),
		Milestones:  []wire.InitiativeMilestone{}, Assumptions: []wire.InitiativeAssumption{},
		Deferred: []wire.InitiativeDeferred{}, GateNotes: []wire.InitiativeGateNote{}, Unparsed: []wire.InitiativeUnparsed{},
	}
	for _, m := range v.Milestones {
		wm := wire.InitiativeMilestone{
			Number: m.Number, Slug: m.Slug, Name: m.Name, Goal: m.Goal, State: m.State, Broken: m.Broken, Design: m.Design,
		}
		if m.ImplEpic != "" {
			wm.DesignAddress, wm.ImplAddress = addr(m.Design), addr(m.ImplEpic)
			wm.ImplExists, wm.DesignArchived, wm.ImplArchived = m.ImplExists, m.DesignArchived, m.ImplArchived
		}
		p.Milestones = append(p.Milestones, wm)
	}
	for _, u := range v.Unparsed {
		p.Unparsed = append(p.Unparsed, wire.InitiativeUnparsed{Section: u.Section, Raw: u.Raw})
	}
	for _, a := range body.Assumptions {
		if !a.Unparsed() {
			p.Assumptions = append(p.Assumptions, wire.InitiativeAssumption{
				N: a.Value.N, Text: a.Value.Text, TestedBy: a.Value.TestedBy, State: a.Value.State, Evidence: a.Value.Evidence,
			})
		}
	}
	for _, d := range body.Deferred {
		if !d.Unparsed() {
			p.Deferred = append(p.Deferred, wire.InitiativeDeferred{
				Item: d.Value.Item, To: d.Value.To, Why: d.Value.Why, From: d.Value.From, State: d.Value.State,
			})
		}
	}
	var stray []string
	p.GateNotes, stray = gateNotes(body.GateNotes)
	for _, raw := range stray {
		p.Unparsed = append(p.Unparsed, wire.InitiativeUnparsed{Section: initiative.GateNotes, Raw: raw})
	}
	return p
}

// gateNotes groups the parsed gate lines under their "### slug" headings. Lines
// before the first heading belong to no note; they come back as stray, in their
// canonical text, so the page shows them rather than dropping them.
func gateNotes(lines []initiative.Entry[initiative.GateLine]) (out []wire.InitiativeGateNote, stray []string) {
	out = []wire.InitiativeGateNote{}
	for _, e := range lines {
		if e.Unparsed() {
			continue
		}
		g := e.Value
		if g.Kind == initiative.GateHeading {
			out = append(out, wire.InitiativeGateNote{Slug: g.Slug, Assumptions: []wire.InitiativeGateAssumption{}, Taught: []string{}, Placed: []string{}})
			continue
		}
		if len(out) == 0 {
			if raw, err := initiative.RenderGateLine(g); err == nil {
				stray = append(stray, raw)
			}
			continue
		}
		n := &out[len(out)-1]
		switch g.Kind {
		case initiative.GateOutcome:
			n.Outcome = g.Outcome
		case initiative.GateAssumption:
			n.Assumptions = append(n.Assumptions, wire.InitiativeGateAssumption{N: g.N, State: g.State, Evidence: g.Evidence})
		case initiative.GateTaught:
			n.Taught = append(n.Taught, g.Text)
		case initiative.GatePlacement:
			n.Placed = append(n.Placed, g.Text)
		}
	}
	return out, stray
}
