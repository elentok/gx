package server

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"regexp"
	"sync"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/migrations"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
)

// ScratchProject is the built-in repo-less project; no registration may take
// its name.
const ScratchProject = "scratch"

// projectNameRe is what an address's first segment can hold (no ':', '/' or
// whitespace), minus a leading dot or dash.
var projectNameRe = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9._-]*$`)

// projectAddMu serializes adds, so two racing adds cannot claim one name.
var projectAddMu sync.Mutex

func refuseAdd(reason, msg string) (wire.AddProjectResult, error) {
	return wire.AddProjectResult{Refused: true, Reason: reason, Message: msg}, nil
}

func (p *Registry) Add(req wire.AddProjectRequest) (wire.AddProjectResult, error) {
	if req.VCS != "" && req.VCS != config.VCSNone {
		return refuseAdd(wire.ReasonBadVCS, "--vcs only accepts \""+config.VCSNone+"\"")
	}
	repo := canonicalDir(req.Path)
	defaultName := filepath.Base(repo)
	if req.VCS == "" {
		info, err := git.FindRepo(req.Path)
		if err != nil {
			return refuseAdd(wire.ReasonNotRepo, req.Path+" is not a git repo (use --vcs none for a project without one)")
		}
		repo = canonicalDir(info.Root)
		dir := repo
		if filepath.Base(dir) == ".bare" {
			dir = filepath.Dir(dir)
		}
		defaultName = filepath.Base(dir)
	}
	name := req.Name
	if name == "" {
		name = defaultName
	}
	if name == ScratchProject {
		return refuseAdd(wire.ReasonReservedName, "\""+ScratchProject+"\" is reserved")
	}
	if !projectNameRe.MatchString(name) {
		return refuseAdd(wire.ReasonBadName, "project name "+name+" must be a slug of letters, digits, '.', '_' or '-'")
	}

	projectAddMu.Lock()
	defer projectAddMu.Unlock()
	dirs, err := tickets.ProjectDirs(p.cfg.TicketStore)
	if err != nil {
		return wire.AddProjectResult{}, err
	}
	for _, dir := range dirs {
		pf, err := config.ReadProjectFile(dir)
		if err == nil && pf.Repo != nil && canonicalDir(*pf.Repo) == repo {
			return wire.AddProjectResult{Name: tickets.ProjectName(dir), Repo: repo, Existing: true}, nil
		}
	}
	for _, dir := range dirs {
		if tickets.ProjectName(dir) == name {
			msg := "a project named " + name + " already exists"
			if pf, err := config.ReadProjectFile(dir); err == nil && pf.VCS != nil && *pf.VCS == config.VCSNone {
				msg += "; to turn that ticket-only project into this clone, run gx project set-path " + name + " <clone>"
			}
			return refuseAdd(wire.ReasonNameTaken, msg)
		}
	}
	pf := config.ProjectFile{Name: &name, Repo: &repo}
	if req.VCS != "" {
		pf.VCS = &req.VCS
	}
	projectDir := filepath.Join(p.cfg.TicketStore, name)
	if err := tickets.WriteProjectFile(projectDir, pf); err != nil {
		return wire.AddProjectResult{}, err
	}
	if err := migrations.WriteFullApplied(projectDir); err != nil {
		return wire.AddProjectResult{}, err
	}
	return wire.AddProjectResult{Name: name, Repo: repo}, nil
}

// canonicalDir resolves symlinks so one directory has one identity; a path
// that cannot be resolved is compared as given.
func canonicalDir(p string) string {
	if abs, err := filepath.Abs(p); err == nil {
		p = abs
	}
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		return resolved
	}
	return filepath.Clean(p)
}

func (s *Server) projectAdd(w http.ResponseWriter, r *http.Request) {
	var req wire.AddProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	res, err := s.projects.Add(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, res)
}
