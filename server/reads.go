package server

import (
	"bufio"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
)

// ProjectInfo is one project in the /v1/projects payload.
type ProjectInfo struct {
	Name string `json:"name"`
	Repo string `json:"repo,omitempty"`
	VCS  string `json:"vcs,omitempty"`
	// Tickets counts the project's tickets by status.
	Tickets map[string]int `json:"tickets"`
}

// LockInfo is one held lock in the /v1/locks payload. Alive is false for a lock
// a crashed process left behind.
type LockInfo struct {
	Kind   string    `json:"kind"`
	Epic   string    `json:"epic"` // "project:epic"
	Pid    int       `json:"pid"`
	Since  time.Time `json:"since"`
	Ticket string    `json:"ticket,omitempty"`
	Alive  bool      `json:"alive"`
}

// History is the /v1/tickets/history payload: the ticket's events in log order.
type History struct {
	Address string            `json:"address"`
	Events  []ralphloop.Event `json:"events"`
}

const lockKindLand = "land"

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func (s *Server) projects(w http.ResponseWriter, _ *http.Request) {
	list, err := s.listProjects()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, list)
}

func (s *Server) listProjects() ([]ProjectInfo, error) {
	counts := map[string]map[string]int{}
	for _, t := range s.idx.snapshot().Tickets {
		a, err := tickets.ParseAddress(t.Address, tickets.AddressContext{})
		if err != nil {
			continue
		}
		if counts[a.Project] == nil {
			counts[a.Project] = map[string]int{}
		}
		counts[a.Project][t.Status]++
	}
	list := []ProjectInfo{}
	dirs, err := tickets.ProjectDirs(s.cfg.TicketStore)
	if err != nil {
		return nil, err
	}
	for _, dir := range dirs {
		name := tickets.ProjectName(dir)
		p := ProjectInfo{Name: name, Tickets: counts[name]}
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

func (s *Server) locks(w http.ResponseWriter, _ *http.Request) {
	list := []LockInfo{}
	dirs, err := tickets.ProjectDirs(s.cfg.TicketStore)
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
			owner, err := ralphloop.ReadLandLock(epicDir)
			if err != nil || owner == nil {
				continue
			}
			list = append(list, LockInfo{
				Kind: lockKindLand, Epic: project + ":" + filepath.Base(epicDir),
				Pid: owner.PID, Since: owner.Time, Ticket: owner.Ticket, Alive: owner.Alive(),
			})
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Epic < list[j].Epic })
	writeJSON(w, list)
}

func (s *Server) history(w http.ResponseWriter, r *http.Request) {
	addr, err := tickets.ParseAddress(r.URL.Query().Get("address"), tickets.AddressContext{})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	events, err := s.readHistory(addr)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	writeJSON(w, History{Address: addr.String(), Events: events})
}

func (s *Server) readHistory(addr tickets.Address) ([]ralphloop.Event, error) {
	dirs, err := tickets.ProjectDirs(s.cfg.TicketStore)
	if err != nil {
		return nil, err
	}
	for _, dir := range dirs {
		if tickets.ProjectName(dir) != addr.Project {
			continue
		}
		return filterEvents(ralphloop.RunLogPath(dir, addr.Epic), addr.String())
	}
	return nil, &tickets.AddressError{Code: tickets.CodeUnknownProject, Msg: "no project " + addr.Project}
}

// filterEvents streams the epic log and keeps the events naming address. A
// missing log is an empty history; unparseable lines are skipped, since the log
// is append-only and a torn line must not hide the rest.
func filterEvents(path, address string) ([]ralphloop.Event, error) {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return []ralphloop.Event{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	out := []ralphloop.Event{}
	sc := bufio.NewScanner(f)
	sc.Buffer(nil, 1<<20)
	for sc.Scan() {
		var ev ralphloop.Event
		if json.Unmarshal(sc.Bytes(), &ev) == nil && ev.Address == address {
			out = append(out, ev)
		}
	}
	return out, sc.Err()
}
