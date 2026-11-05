package server

import (
	"slices"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/tickets"
)

// Merger is the server's epic merge code: the merge verb, the root's merge
// decision and its outcome, the merge ticket fork and the crash recovery. It
// is split from Server in place; the embedded *Server is the way into the rest
// of the server until the package move cuts it.
type Merger struct {
	*Server
	mergeTargets mergeTargetCache
	projectRepos projectRepoCache
	mergeInto    func(git.Repo, []git.Worktree, string, string) (git.MergeOutcome, error) // swapped in tests
}

func newMerger(s *Server) *Merger { return &Merger{Server: s, mergeInto: git.MergeBranchInto} }

// autoFFMerge reports whether the project opted in to automatic ff-only merges,
// read live. An unreadable project file means off: never merge on a guess.
func (s *Merger) autoFFMerge(project string) bool {
	dir, err := s.projects.Dir(project)
	if err != nil {
		return false
	}
	pf, _ := config.ReadProjectFile(dir)
	return pf.AutoFFMergeEnabled()
}

// epicRefs is rootRefs for epic's Root ticket, the one ticket every caller
// that means "the epic's refs" agrees on, so they all lock the same key. A
// non-root ticket's base: never moves where the epic starts or merges.
func (s *Merger) epicRefs(project string, epics []tickets.Epic, epic, repo string) (root tickets.Ticket, base, target string, err error) {
	if i := slices.IndexFunc(epics, func(e tickets.Epic) bool { return e.Name == epic }); i >= 0 && len(epics[i].Tickets) > 0 {
		root = lowestTicket(epics[i])
	}
	base, target, err = s.rootRefs(project, epics, epic, root, repo)
	return root, base, target, err
}
