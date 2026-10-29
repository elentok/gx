package server

import (
	"net/http"
	"path/filepath"
	"sort"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/tickets"
)

// ProjectWorktrees is one project in the /v1/projects/worktrees payload: its
// repo root and the dirs of its other worktrees.
type ProjectWorktrees struct {
	Project   string   `json:"project"`
	Root      string   `json:"root"`
	Worktrees []string `json:"worktrees"`
}

func (s *Server) projectWorktrees(w http.ResponseWriter, _ *http.Request) {
	list, err := s.listProjectWorktrees()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, list)
}

// listProjectWorktrees lists each project that has a repo in project.json.
func (s *Server) listProjectWorktrees() ([]ProjectWorktrees, error) {
	dirs, err := tickets.ProjectDirs(s.cfg.TicketStore)
	if err != nil {
		return nil, err
	}
	list := []ProjectWorktrees{}
	for _, dir := range dirs {
		pf, err := config.ReadProjectFile(dir)
		if err != nil || pf.Repo == nil {
			continue
		}
		repo, err := git.FindRepo(*pf.Repo)
		if err != nil {
			continue
		}
		p := ProjectWorktrees{Project: tickets.ProjectName(dir), Root: repo.Root, Worktrees: []string{}}
		wts, _ := git.ListWorktrees(*repo)
		for _, wt := range wts {
			if filepath.Clean(wt.Path) != filepath.Clean(repo.Root) {
				p.Worktrees = append(p.Worktrees, wt.Path)
			}
		}
		list = append(list, p)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Project < list[j].Project })
	return list, nil
}
