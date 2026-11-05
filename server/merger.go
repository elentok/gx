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

// mergerHost is what Merger needs from the root Server that is not a plain
// dependency: the refs of a root and the runner a hosted verify run uses.
type mergerHost interface {
	rootRefs(project string, epics []tickets.Epic, epic string, t tickets.Ticket, repo string) (base, target string, err error)
	hostingDeps(addr tickets.Address) ralphloop.Deps
}

// Merger is the server's epic merge code: the merge verb, the root's merge
// decision and its outcome, the merge ticket fork and the crash recovery. Its
// dependencies are fields copied from Server when it is built, so a Server
// field swapped later is not seen; the rest of the root comes in through host.
type Merger struct {
	host     mergerHost
	log      *slog.Logger
	cfg      Config
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
	return &Merger{
		host: s, log: s.log, cfg: s.cfg, idx: s.idx, projects: s.projects, queue: s.queue,
		oneOffs: s.oneOffs, events: s.events, writer: s.writer, chat: s.chat, registry: s.registry,
		guards: s.guards, settings: &s.settings, rec: s.rec, landing: s.landing,
		mergeInto: git.MergeBranchInto,
	}
}

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
	base, target, err = s.host.rootRefs(project, epics, epic, root, repo)
	return root, base, target, err
}
