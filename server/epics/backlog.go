package epics

import (
	"net/http"
	"slices"

	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/ticketmove"
)

// Backlog serves the Backlog page: per project with a backlog, its epics with
// their open counts and any unfinished Start Design move.
func (s *Epics) Backlog(w http.ResponseWriter, _ *http.Request) {
	list, err := s.cardList()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	dirs, err := tickets.ProjectDirs(s.TicketStore)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	workspacesOf := s.projectWorkspaces(list.Epics)
	out := wire.BacklogList{Projects: []wire.BacklogProject{}}
	for _, dir := range dirs {
		project := tickets.ProjectName(dir)
		epics, err := s.Projects.Epics(project)
		if err != nil {
			continue
		}
		if bl := backlogEpics(project, epics); len(bl) > 0 {
			out.Projects = append(out.Projects, wire.BacklogProject{Project: project, Workspaces: workspacesOf(project), Epics: bl})
		}
	}
	wire.WriteJSON(w, out)
}

// projectWorkspaces returns each project's workspaces: those of its Design and
// Implementation cards, or the Default workspace when it has none.
func (s *Epics) projectWorkspaces(rows []wire.EpicListRow) func(project string) []string {
	byProject := map[string][]string{}
	for _, r := range rows {
		if r.Workspace != "" && !slices.Contains(byProject[r.Project], r.Workspace) {
			byProject[r.Project] = append(byProject[r.Project], r.Workspace)
		}
	}
	return func(project string) []string {
		if ws := byProject[project]; len(ws) > 0 {
			return ws
		}
		if s.Workspaces == nil {
			return []string{}
		}
		// A project name is never a member key, so this is the Default workspace.
		return []string{s.Workspaces.Workspace(project)}
	}
}

func backlogEpics(project string, all []tickets.Epic) []wire.BacklogEpic {
	var out []wire.BacklogEpic
	for _, e := range all {
		if !e.IsBacklog {
			continue
		}
		a := wire.BacklogEpic{Address: queue.RootRef{Project: project, Epic: e.Name}.String(), Name: e.Name}
		inBacklog := map[string]bool{}
		a.Open = e.OpenTicketCount()
		for _, t := range e.Tickets {
			inBacklog[t.DisplayNumber()] = true
		}
		a.Unfinished = unfinishedMove(project, e.Name, inBacklog, all)
		out = append(out, a)
	}
	return out
}

// unfinishedMove finds the charting map of a partial Start Design run for backlog.
func unfinishedMove(project, backlog string, inBacklog map[string]bool, epics []tickets.Epic) *wire.BacklogUnfinishedMove {
	for _, m := range epics {
		ids, ok := ticketmove.UnfinishedRunIDs(m, backlog)
		if !ok {
			continue
		}
		left := 0
		for _, id := range ids {
			if inBacklog[id] {
				left++
			}
		}
		if left > 0 {
			return &wire.BacklogUnfinishedMove{Map: queue.RootRef{Project: project, Epic: m.Name}.String(), ToMove: left}
		}
	}
	return nil
}
