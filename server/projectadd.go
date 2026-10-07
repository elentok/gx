package server

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"regexp"
	"sync"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/tickets"
)

// ScratchProject is the built-in repo-less project; no registration may take
// its name.
const ScratchProject = "scratch"

// Refusal reasons of a project add.
const (
	ReasonBadName      = "bad-name"
	ReasonReservedName = "reserved-name"
	ReasonNameTaken    = "name-taken"
	ReasonNotRepo      = "not-a-repo"
	ReasonBadVCS       = "bad-vcs"
)

// AddProjectRequest registers the project at Path. Name defaults to the repo
// directory's name; VCS is "" (git) or config.VCSNone.
type AddProjectRequest struct {
	Path string `json:"path"`
	Name string `json:"name,omitempty"`
	VCS  string `json:"vcs,omitempty"`
}

// AddProjectResult is the outcome of a project add. Existing is true when the
// path was already registered, in which case Name is the existing project's.
type AddProjectResult struct {
	Name     string `json:"name,omitempty"`
	Repo     string `json:"repo,omitempty"`
	Existing bool   `json:"existing,omitempty"`
	Refused  bool   `json:"refused,omitempty"`
	Reason   string `json:"reason,omitempty"`
	Message  string `json:"message,omitempty"`
}

// projectNameRe is what an address's first segment can hold (no ':', '/' or
// whitespace), minus a leading dot or dash.
var projectNameRe = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9._-]*$`)

// projectAddMu serializes adds, so two racing adds cannot claim one name.
var projectAddMu sync.Mutex

func refuseAdd(reason, msg string) (AddProjectResult, error) {
	return AddProjectResult{Refused: true, Reason: reason, Message: msg}, nil
}

func (s *Server) addProject(req AddProjectRequest) (AddProjectResult, error) {
	if req.VCS != "" && req.VCS != config.VCSNone {
		return refuseAdd(ReasonBadVCS, "--vcs only accepts \""+config.VCSNone+"\"")
	}
	repo := canonicalDir(req.Path)
	defaultName := filepath.Base(repo)
	if req.VCS == "" {
		info, err := git.FindRepo(req.Path)
		if err != nil {
			return refuseAdd(ReasonNotRepo, req.Path+" is not a git repo (use --vcs none for a project without one)")
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
		return refuseAdd(ReasonReservedName, "\""+ScratchProject+"\" is reserved")
	}
	if !projectNameRe.MatchString(name) {
		return refuseAdd(ReasonBadName, "project name "+name+" must be a slug of letters, digits, '.', '_' or '-'")
	}

	projectAddMu.Lock()
	defer projectAddMu.Unlock()
	dirs, err := tickets.ProjectDirs(s.cfg.TicketStore)
	if err != nil {
		return AddProjectResult{}, err
	}
	for _, dir := range dirs {
		pf, err := config.ReadProjectFile(dir)
		if err == nil && pf.Repo != nil && canonicalDir(*pf.Repo) == repo {
			return AddProjectResult{Name: tickets.ProjectName(dir), Repo: repo, Existing: true}, nil
		}
	}
	for _, dir := range dirs {
		if tickets.ProjectName(dir) == name {
			return refuseAdd(ReasonNameTaken, "a project named "+name+" already exists")
		}
	}
	pf := config.ProjectFile{Name: &name, Repo: &repo}
	if req.VCS != "" {
		pf.VCS = &req.VCS
	}
	if err := tickets.WriteProjectFile(filepath.Join(s.cfg.TicketStore, name), pf); err != nil {
		return AddProjectResult{}, err
	}
	return AddProjectResult{Name: name, Repo: repo}, nil
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
	var req AddProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	res, err := s.addProject(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, res)
}
