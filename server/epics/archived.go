package epics

import (
	"cmp"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/server/workspaces"
	"github.com/elentok/gx/tickets"
)

// ArchivedList serves the archived epics and Designs, newest archived first.
func (s *Epics) ArchivedList(w http.ResponseWriter, _ *http.Request) {
	dirs, err := tickets.ProjectDirs(s.TicketStore)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	known := map[string]bool{}
	if s.Workspaces != nil {
		for _, ws := range s.Workspaces.Listed() {
			known[ws.Name] = true
		}
	}
	list := wire.ArchivedEpicList{Epics: []wire.ArchivedEpicRow{}}
	for _, dir := range dirs {
		archived, err := tickets.LoadArchived(dir)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		list.Epics = append(list.Epics, archivedRows(tickets.ProjectName(dir), archived, known)...)
	}
	slices.SortStableFunc(list.Epics, func(a, b wire.ArchivedEpicRow) int {
		// A row with no stamp sorts last.
		if a.ArchivedAt.IsZero() != b.ArchivedAt.IsZero() {
			if a.ArchivedAt.IsZero() {
				return 1
			}
			return -1
		}
		return cmp.Or(b.ArchivedAt.Compare(a.ArchivedAt), cmp.Compare(a.Address, b.Address))
	})
	wire.WriteJSON(w, list)
}

// archivedRows makes one row per archived loop epic or Design. A Design's
// -impl epic joins its row.
func archivedRows(project string, archived []tickets.Epic, knownWorkspaces map[string]bool) []wire.ArchivedEpicRow {
	byName := map[string]tickets.Epic{}
	for _, e := range archived {
		byName[e.Name] = e
	}
	var rows []wire.ArchivedEpicRow
	for _, e := range archived {
		if design, ok := strings.CutSuffix(e.Name, tickets.ImplEpicSuffix); ok && byName[design].IsMap {
			continue
		}
		addr := project + ":" + e.Name
		row := wire.ArchivedEpicRow{
			Address: addr, Project: project, Name: e.Name,
			Title:      cmp.Or(tickets.BodyTitle(e.Body, ""), e.Name),
			ArchivedAt: e.ArchivedAt, CompletedAt: e.CompletedAt,
			Kind: wire.EpicKindLoop, Addresses: []string{addr},
			Workspace: workspaces.DefaultName,
		}
		if e.IsMap {
			row.Kind = wire.EpicKindMap
			if _, ok := byName[e.Name+tickets.ImplEpicSuffix]; ok {
				row.Addresses = append(row.Addresses, addr+tickets.ImplEpicSuffix)
			}
		}
		if knownWorkspaces[e.ArchivedWorkspace] {
			row.Workspace = e.ArchivedWorkspace
		}
		rows = append(rows, row)
	}
	return rows
}

// ArchivedDetail serves one archived epic's detail by its archive address.
func (s *Epics) ArchivedDetail(w http.ResponseWriter, r *http.Request) {
	address := r.URL.Query().Get("address")
	notFound := func() { http.Error(w, "no archived epic "+address, http.StatusNotFound) }
	root, err := queue.ParseRootRef(address)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	dir, err := tickets.ProjectDirByName(s.TicketStore, root.Project)
	if err != nil {
		if _, isAddr := errors.AsType[*tickets.AddressError](err); isAddr {
			notFound()
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	archived, err := tickets.LoadArchived(dir)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	i := slices.IndexFunc(archived, func(e tickets.Epic) bool { return e.Name == root.Epic })
	if i < 0 {
		notFound()
		return
	}
	d := s.buildEpicDetail(root, archived, archived[i], time.Now())
	d.ArchivedAt = archived[i].ArchivedAt
	if d.Kind == wire.EpicKindLoop {
		// An archived epic runs nothing, so no ticket has a claim.
		d.State = EpicStateFor(root, archived[i], "", func(string) (time.Time, bool) { return time.Time{}, false })
	}
	wire.WriteJSON(w, d)
}
