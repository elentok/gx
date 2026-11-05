package server

import (
	"log/slog"
	"slices"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/server/broker"
	"github.com/elentok/gx/server/index"
	"github.com/elentok/gx/server/projects"
	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/runs"
	"github.com/elentok/gx/server/settings"
	"github.com/elentok/gx/server/ticketwrite"
	"github.com/elentok/gx/tickets"
)

// Merger is the server's epic merge code: the merge verb, the root's merge
// decision and its outcome, the merge ticket fork and the crash recovery. Its
// dependencies are fields copied from Deps when it is built, so a root
// field swapped later is not seen; the rest of the root comes in through host.
type Merger struct {
	host     Host
	log      *slog.Logger
	cfg      Dirs
	idx      *index.Index
	projects *projects.Registry
	queue    *queue.Queue
	oneOffs  *queue.OneOffs
	events   *broker.Broker
	writer   *ticketwrite.Writer
	chat     *ralphloop.ServerChat
	registry *runs.Registry
	guards   *runs.Guards
	settings *settings.Holder
	rec      *Reconciler
	landing  *Landing

	mergeTargets mergeTargetCache
	projectRepos projectRepoCache
	mergeInto    func(git.Repo, []git.Worktree, string, string) (git.MergeOutcome, error) // swapped in tests
}

func newMerger(s *Server) *Merger {
	d := s.mergeDeps()
	return &Merger{
		host: d.Host, log: d.Log, cfg: d.Dirs, idx: d.Idx, projects: d.Projects, queue: d.Queue,
		oneOffs: d.OneOffs, events: d.Events, writer: d.Writer, chat: d.Chat, registry: d.Registry,
		guards: d.Guards, settings: d.Settings, rec: s.rec, landing: s.landing,
		mergeInto: git.MergeBranchInto,
	}
}

// AutoFFMerge reports whether the project opted in to automatic ff-only merges,
// read live. An unreadable project file means off: never merge on a guess.
func (s *Merger) AutoFFMerge(project string) bool {
	dir, err := s.projects.Dir(project)
	if err != nil {
		return false
	}
	pf, _ := config.ReadProjectFile(dir)
	return pf.AutoFFMergeEnabled()
}

// EpicRefs is rootRefs for epic's Root ticket, the one ticket every caller
// that means "the epic's refs" agrees on, so they all lock the same key. A
// non-root ticket's base: never moves where the epic starts or merges.
func (s *Merger) EpicRefs(project string, epics []tickets.Epic, epic, repo string) (root tickets.Ticket, base, target string, err error) {
	if i := slices.IndexFunc(epics, func(e tickets.Epic) bool { return e.Name == epic }); i >= 0 && len(epics[i].Tickets) > 0 {
		root = lowestTicket(epics[i])
	}
	base, target, err = s.host.rootRefs(project, epics, epic, root, repo)
	return root, base, target, err
}
