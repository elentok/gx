package epics

import (
	"net/http"
	"slices"

	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
	"github.com/elentok/gx/tickets/ticketmove"
)

// Backlog serves the Backlog page: per project with an area, its areas with
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
		if areas := backlogAreas(project, epics); len(areas) > 0 {
			out.Projects = append(out.Projects, wire.BacklogProject{Project: project, Workspaces: workspacesOf(project), Areas: areas})
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

func backlogAreas(project string, epics []tickets.Epic) []wire.BacklogArea {
	var areas []wire.BacklogArea
	for _, e := range epics {
		if !e.IsArea {
			continue
		}
		a := wire.BacklogArea{Address: queue.RootRef{Project: project, Epic: e.Name}.String(), Name: e.Name}
		inArea := map[string]bool{}
		for _, t := range e.Tickets {
			if t.Status == string(schema.StatusDraft) || t.Status == string(schema.StatusOpen) {
				a.Open++
			}
			inArea[t.DisplayNumber()] = true
		}
		a.Unfinished = unfinishedMove(project, e.Name, inArea, epics)
		areas = append(areas, a)
	}
	return areas
}

// unfinishedMove finds the charting map of a partial Start Design run for area.
func unfinishedMove(project, area string, inArea map[string]bool, epics []tickets.Epic) *wire.BacklogUnfinishedMove {
	for _, m := range epics {
		notesArea, ids, ok := ticketmove.ParseStartDesignNotes(m.Body)
		if !ok || notesArea != area || !m.IsMap || !m.CompletedAt.IsZero() ||
			(m.DesignStep != "" && m.DesignStep != string(schema.StepChart)) {
			continue
		}
		left := 0
		for _, id := range ids {
			if inArea[id] {
				left++
			}
		}
		if left > 0 {
			return &wire.BacklogUnfinishedMove{Map: queue.RootRef{Project: project, Epic: m.Name}.String(), ToMove: left}
		}
	}
	return nil
}
