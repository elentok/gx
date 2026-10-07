package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/tickets"
)

// Refusal reasons of a project remove or set-path.
const (
	ReasonUnknownProject = "unknown-project"
	ReasonProjectBusy    = "project-busy"
	ReasonPathTaken      = "path-taken"
)

// ProjectRequest names a project; Path is what set-path points it at.
type ProjectRequest struct {
	Name string `json:"name"`
	Path string `json:"path,omitempty"`
}

// ProjectResult is the outcome of a project remove or set-path.
type ProjectResult struct {
	Name    string `json:"name,omitempty"`
	Repo    string `json:"repo,omitempty"`
	Refused bool   `json:"refused,omitempty"`
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
}

func refuseProject(reason, msg string) (ProjectResult, error) {
	return ProjectResult{Refused: true, Reason: reason, Message: msg}, nil
}

// projectBusy reports a queued or running ticket of project.
func (s *Server) projectBusy(project string) (address string, busy bool) {
	inProject := func(addr string) bool {
		a, err := tickets.ParseAddress(addr, tickets.AddressContext{})
		return err == nil && a.Project == project
	}
	for _, it := range s.queued.list() {
		if inProject(it.Address) {
			return it.Address, true
		}
	}
	for _, r := range s.registry.list() {
		if inProject(r.Address) {
			return r.Address, true
		}
	}
	return "", false
}

// removeProject unregisters a project by deleting its project.json. Its
// tickets stay in the store, so adding the path again picks them back up.
func (s *Server) removeProject(req ProjectRequest) (ProjectResult, error) {
	projectAddMu.Lock()
	defer projectAddMu.Unlock()
	if req.Name == ScratchProject {
		return refuseProject(ReasonReservedName, "\""+ScratchProject+"\" is built in and cannot be removed")
	}
	dir, err := s.projectDir(req.Name)
	if err != nil {
		return refuseProject(ReasonUnknownProject, "no project named "+req.Name)
	}
	if addr, busy := s.projectBusy(req.Name); busy {
		return refuseProject(ReasonProjectBusy, addr+" is queued or running; remove it from the queue first")
	}
	if err := os.Remove(filepath.Join(dir, config.ProjectFileName)); err != nil {
		return ProjectResult{}, err
	}
	return ProjectResult{Name: req.Name}, nil
}

// setProjectPath points an existing project at another repo.
func (s *Server) setProjectPath(req ProjectRequest) (ProjectResult, error) {
	projectAddMu.Lock()
	defer projectAddMu.Unlock()
	dir, err := s.projectDir(req.Name)
	if err != nil {
		return refuseProject(ReasonUnknownProject, "no project named "+req.Name)
	}
	pf, err := config.ReadProjectFile(dir)
	if err != nil {
		return ProjectResult{}, err
	}
	repo := canonicalDir(req.Path)
	if pf.VCS == nil || *pf.VCS != config.VCSNone {
		info, err := git.FindRepo(req.Path)
		if err != nil {
			return refuseProject(ReasonNotRepo, req.Path+" is not a git repo")
		}
		repo = canonicalDir(info.Root)
	}
	dirs, err := tickets.ProjectDirs(s.cfg.TicketStore)
	if err != nil {
		return ProjectResult{}, err
	}
	for _, d := range dirs {
		other, err := config.ReadProjectFile(d)
		if d != dir && err == nil && other.Repo != nil && canonicalDir(*other.Repo) == repo {
			return refuseProject(ReasonPathTaken, repo+" is already project "+tickets.ProjectName(d))
		}
	}
	pf.Repo = &repo
	if err := tickets.WriteProjectFile(dir, pf); err != nil {
		return ProjectResult{}, err
	}
	return ProjectResult{Name: req.Name, Repo: repo}, nil
}

func (s *Server) projectWrite(do func(ProjectRequest) (ProjectResult, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req ProjectRequest
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
