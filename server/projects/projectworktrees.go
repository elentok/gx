package projects

import (
	"path/filepath"
	"sort"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
)

// tierModels resolves each tier's model for the harness a web-created session
// launches.
func (p *Registry) TierModels(project string) map[string]string {
	agents := p.AgentsConfig(project)
	agent := ralphloop.AgentFor(p.LaunchHarness(project))
	out := map[string]string{}
	for _, t := range ralphloop.AllTiers {
		out[string(t)], _ = ralphloop.ResolveTier(agents, agent, t)
	}
	return out
}

// listProjectWorktrees lists each project that has a repo in project.json.
func (p *Registry) ListWorktrees() ([]wire.ProjectWorktrees, error) {
	dirs, err := tickets.ProjectDirs(p.cfg().TicketStore)
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
		p := wire.ProjectWorktrees{Project: name, Root: repo.Root, Worktrees: []string{}, Tiers: p.TierModels(name)}
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
