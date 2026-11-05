package merge

import (
	"log/slog"
	"sync"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/server/broker"
	harnesschoice "github.com/elentok/gx/server/harness"
	"github.com/elentok/gx/server/index"
	"github.com/elentok/gx/server/projects"
	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/runs"
	"github.com/elentok/gx/server/sessions"
	"github.com/elentok/gx/server/settings"
	"github.com/elentok/gx/server/ticketwrite"
)

// Reconciler is the server's startup and rescan repair of merge state: late
// finishes, hand-merged epics, parent defects, missing trunks and the removal
// of merged worktrees. Its dependencies are fields copied from Deps when it is
// built; the rest of the root comes in through host. merger is set by New once
// the Merger exists, and sessions once the session manager does.
type Reconciler struct {
	host     Host
	log      *slog.Logger
	cfg      Dirs
	idx      *index.Index
	projects *projects.Registry
	queue    *queue.Queue
	events   *broker.Broker
	writer   *ticketwrite.Writer
	chat     *ralphloop.ServerChat
	settings *settings.Holder
	harness  *harnesschoice.Harness
	sessions *sessions.Manager
	registry *runs.Registry
	guards   *runs.Guards
	merger   *Merger

	defectScan   sync.Mutex
	lateFinishes inFlight
	TrunkMissing missingTrunks
	mergedProbe  func(repo, ancestor, descendant string) (bool, error) // the poll-time ancestry check; swapped in tests
}

// NewReconciler builds a Reconciler from d; New sets its merger.
func NewReconciler(d Deps) *Reconciler {
	return &Reconciler{
		host: d.Host, log: d.Log, cfg: d.Dirs, idx: d.Idx, projects: d.Projects, queue: d.Queue,
		events: d.Events, writer: d.Writer, chat: d.Chat, settings: d.Settings, harness: d.Harness,
		sessions: d.Sessions, registry: d.Registry, guards: d.Guards,
		mergedProbe: git.IsAncestor,
	}
}

// GiveSessions hands over the session manager, which the root builds after the
// Reconciler.
func (r *Reconciler) GiveSessions(m *sessions.Manager) { r.sessions = m }
