package index

import (
	"bufio"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sort"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/ralphloop/merge"
	"github.com/elentok/gx/ralphloop/runlog"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
)

const (
	lockKindMerge = "land"
)

// Routes are the store reads and the change ping the root serves. idx is
// resolved per request, so the list can be built before the index exists;
// migrated gives the channel that closes when the start-up migrations end.
func Routes(idx func() *Index, migrated func() <-chan struct{}) []wire.Route {
	return []wire.Route{
		{Pattern: "GET /v1/projects", Handler: func(w http.ResponseWriter, r *http.Request) { idx().projects(w, r) }},
		{Pattern: "GET /v1/locks", Handler: func(w http.ResponseWriter, r *http.Request) { idx().locks(w, r) }},
		{Pattern: "GET /v1/tickets/history", Handler: func(w http.ResponseWriter, r *http.Request) { idx().history(w, r) }},
		{Pattern: "POST /v1/tickets/changed", Handler: func(w http.ResponseWriter, r *http.Request) { idx().ticketChanged(migrated)(w, r) }},
	}
}

func (i *Index) projects(w http.ResponseWriter, _ *http.Request) {
	list, err := i.ListProjects()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	wire.WriteJSON(w, list)
}

// ListProjects is every project in the store with its ticket counts.
func (i *Index) ListProjects() ([]wire.ProjectInfo, error) {
	counts := map[string]map[string]int{}
	for _, t := range i.Snapshot().Tickets {
		a, err := tickets.ParseAddress(t.Address, tickets.AddressContext{})
		if err != nil {
			continue
		}
		if counts[a.Project] == nil {
			counts[a.Project] = map[string]int{}
		}
		counts[a.Project][t.Status]++
	}
	list := []wire.ProjectInfo{}
	dirs, err := tickets.ProjectDirs(i.storePath)
	if err != nil {
		return nil, err
	}
	for _, dir := range dirs {
		name := tickets.ProjectName(dir)
		p := wire.ProjectInfo{Name: name, Tickets: counts[name]}
		if p.Tickets == nil {
			p.Tickets = map[string]int{}
		}
		if pf, err := config.ReadProjectFile(dir); err == nil {
			if pf.Repo != nil {
				p.Repo = *pf.Repo
			}
			if pf.VCS != nil {
				p.VCS = *pf.VCS
			}
		}
		list = append(list, p)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })
	return list, nil
}

func (i *Index) locks(w http.ResponseWriter, _ *http.Request) {
	list := []wire.LockInfo{}
	dirs, err := tickets.ProjectDirs(i.storePath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	for _, dir := range dirs {
		project := tickets.ProjectName(dir)
		epicDirs, err := filepath.Glob(filepath.Join(dir, "*", "issues"))
		if err != nil {
			continue
		}
		for _, issues := range epicDirs {
			epicDir := filepath.Dir(issues)
			owner, err := merge.ReadMergeLock(epicDir)
			if err != nil || owner == nil {
				continue
			}
			list = append(list, wire.LockInfo{
				Kind: lockKindMerge, Epic: project + ":" + filepath.Base(epicDir),
				Pid: owner.PID, Since: owner.Time, Ticket: owner.Ticket, Alive: owner.Alive(),
			})
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Epic < list[j].Epic })
	wire.WriteJSON(w, list)
}

func (i *Index) history(w http.ResponseWriter, r *http.Request) {
	addr, err := tickets.ParseAddress(r.URL.Query().Get("address"), tickets.AddressContext{})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	events, err := i.ReadHistory(addr)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	wire.WriteJSON(w, wire.History{Address: addr.String(), Events: events})
}

// ReadHistory is the ticket's events from its epic's run log, in log order.
func (i *Index) ReadHistory(addr tickets.Address) ([]runlog.Event, error) {
	dirs, err := tickets.ProjectDirs(i.storePath)
	if err != nil {
		return nil, err
	}
	for _, dir := range dirs {
		if tickets.ProjectName(dir) != addr.Project {
			continue
		}
		return filterEvents(runlog.RunLogPath(dir, addr.Epic), addr.String())
	}
	return nil, &tickets.AddressError{Code: tickets.CodeUnknownProject, Msg: "no project " + addr.Project}
}

// filterEvents streams the epic log and keeps the events naming address. A
// missing log is an empty history; unparseable lines are skipped, since the log
// is append-only and a torn line must not hide the rest.
func filterEvents(path, address string) ([]runlog.Event, error) {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return []runlog.Event{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	out := []runlog.Event{}
	sc := bufio.NewScanner(f)
	sc.Buffer(nil, 1<<20)
	for sc.Scan() {
		var ev runlog.Event
		if json.Unmarshal(sc.Bytes(), &ev) == nil && ev.Address == address {
			out = append(out, ev)
		}
	}
	return out, sc.Err()
}
