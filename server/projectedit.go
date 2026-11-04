package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
)

func refuseProject(reason, msg string) (wire.ProjectResult, error) {
	return wire.ProjectResult{Refused: true, Reason: reason, Message: msg}, nil
}

// projectBusy reports a queued or running ticket of project.
func (s *Server) projectBusy(project string) (address string, busy bool) {
	inProject := func(addr string) bool {
		a, err := parseQueuedAddress(addr)
		return err == nil && a.Project == project
	}
	for _, it := range s.queued.list() {
		if inProject(it.Address) {
			return it.Address, true
		}
	}
	for _, r := range s.registry.List() {
		if inProject(r.Address) {
			return r.Address, true
		}
	}
	return "", false
}

// removeProject unregisters a project by deleting its project.json. Its
// tickets stay in the store, so adding the path again picks them back up.
func (s *Server) removeProject(req wire.ProjectRequest) (wire.ProjectResult, error) {
	projectAddMu.Lock()
	defer projectAddMu.Unlock()
	if req.Name == ScratchProject {
		return refuseProject(wire.ReasonReservedName, "\""+ScratchProject+"\" is built in and cannot be removed")
	}
	dir, err := s.projectDir(req.Name)
	if err != nil {
		return refuseProject(wire.ReasonUnknownProject, "no project named "+req.Name)
	}
	if addr, busy := s.projectBusy(req.Name); busy {
		return refuseProject(wire.ReasonProjectBusy, addr+" is queued or running; remove it from the queue first")
	}
	if err := os.Remove(filepath.Join(dir, config.ProjectFileName)); err != nil {
		return wire.ProjectResult{}, err
	}
	return wire.ProjectResult{Name: req.Name}, nil
}

// setProjectPath points an existing project at another repo.
func (s *Server) setProjectPath(req wire.ProjectRequest) (wire.ProjectResult, error) {
	projectAddMu.Lock()
	defer projectAddMu.Unlock()
	dir, err := s.projectDir(req.Name)
	if err != nil {
		return refuseProject(wire.ReasonUnknownProject, "no project named "+req.Name)
	}
	pf, err := config.ReadProjectFile(dir)
	if err != nil {
		return wire.ProjectResult{}, err
	}
	repo := canonicalDir(req.Path)
	info, gitErr := git.FindRepo(req.Path)
	if gitErr == nil {
		repo = canonicalDir(info.Root)
		// The outbox became a real clone, so its tickets can commit.
		pf.VCS = nil
	} else if pf.VCS == nil || *pf.VCS != config.VCSNone {
		return refuseProject(wire.ReasonNotRepo, req.Path+" is not a git repo")
	}
	dirs, err := tickets.ProjectDirs(s.cfg.TicketStore)
	if err != nil {
		return wire.ProjectResult{}, err
	}
	for _, d := range dirs {
		other, err := config.ReadProjectFile(d)
		if d != dir && err == nil && other.Repo != nil && canonicalDir(*other.Repo) == repo {
			return refuseProject(wire.ReasonPathTaken, repo+" is already project "+tickets.ProjectName(d))
		}
	}
	pf.Repo = &repo
	if err := tickets.WriteProjectFile(dir, pf); err != nil {
		return wire.ProjectResult{}, err
	}
	return wire.ProjectResult{Name: req.Name, Repo: repo}, nil
}

func (s *Server) projectWrite(do func(wire.ProjectRequest) (wire.ProjectResult, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req wire.ProjectRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		res, err := do(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, res)
	}
}
