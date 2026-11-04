package server

import (
	"net/http"
	"path/filepath"
	"sort"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
)

func (s *Server) projectWorktrees(w http.ResponseWriter, _ *http.Request) {
	list, err := s.listProjectWorktrees()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, list)
}

// tierModels resolves each tier's model for the harness a web-created session
// launches.
func (s *Server) tierModels(project string) map[string]string {
	agents := s.agentsConfig(project)
	agent := ralphloop.AgentFor(s.launchHarness(project))
	out := map[string]string{}
	for _, t := range ralphloop.AllTiers {
		out[string(t)], _ = ralphloop.ResolveTier(agents, agent, t)
	}
	return out
}

// listProjectWorktrees lists each project that has a repo in project.json.
func (s *Server) listProjectWorktrees() ([]wire.ProjectWorktrees, error) {
	dirs, err := tickets.ProjectDirs(s.cfg.TicketStore)
	if err != nil {
		return nil, err
	}
	list := []wire.ProjectWorktrees{}
	for _, dir := range dirs {
		pf, err := config.ReadProjectFile(dir)
		if err != nil || pf.Repo == nil {
			continue
		}
		repo, err := git.FindRepo(*pf.Repo)
		if err != nil {
			continue
		}
		name := tickets.ProjectName(dir)
		p := wire.ProjectWorktrees{Project: name, Root: repo.Root, Worktrees: []string{}, Tiers: s.tierModels(name)}
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
