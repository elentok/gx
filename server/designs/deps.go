package designs

import (
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/elentok/gx/ralphloop/notify"
	"github.com/elentok/gx/server/broker"
	"github.com/elentok/gx/server/designs/drafts"
	"github.com/elentok/gx/server/drafthistory"
	"github.com/elentok/gx/server/index"
	"github.com/elentok/gx/server/projects"
	"github.com/elentok/gx/server/runs"
	"github.com/elentok/gx/server/sessions"
	"github.com/elentok/gx/server/wire"
)

// Deps is everything the design code copies when it is built. A field the root
// swaps later is not seen.
type Deps struct {
	Log              *slog.Logger
	TicketStore      string
	ConversationsDir string
	// StateDir holds the New Design drafts.
	StateDir string
	// History is the Draft history store, shared with the other draft kinds.
	History  *drafthistory.Store
	Idx      *index.Index
	Projects *projects.Registry
	Events   *broker.Broker
	Guards   *runs.Guards
	// Chat sends the Ready message; WebURL is the web listener's base URL, "" when off.
	Chat   *notify.ServerChat
	WebURL func() string
	// Workspaces places a promoted Design in its chat's workspace; nil skips it.
	Workspaces Workspaces
	// OnBranch is called when a map's design step moves to Branch inside a
	// running server; nil skips it.
	OnBranch func(project, epic string)
	// ForkHookDir is the test-hook dir (see chatdrafts.HookArmed); empty turns the hooks off.
	ForkHookDir string
}

// Workspaces is the slice of the workspace store a promoted Design uses.
type Workspaces interface {
	Workspace(key string) string
	// WorkspaceOfSession resolves a chat or a Design/Queue/Fix session id.
	WorkspaceOfSession(id string) string
	SetMember(key, workspace string) error
	Remove(key string) error
}

// Designs opens, cleans up and deletes the Design sessions of map epics.
type Designs struct {
	log              *slog.Logger
	ticketStore      string
	conversationsDir string
	// drafts keeps the New Design drafts, which are not epics.
	drafts     *drafts.Store
	history    *drafthistory.Store
	idx        *index.Index
	projects   *projects.Registry
	events     *broker.Broker
	guards     *runs.Guards
	chat       *notify.ServerChat
	webURL     func() string
	workspaces Workspaces
	onBranch   func(project, epic string)
	// forkHookDir is the test-hook dir; empty turns the hooks off.
	forkHookDir string
	// seeded: the Ready seed mark has been settled this run.
	seeded atomic.Bool
	// sessions comes later than Designs: the Manager calls CleanupDesign back,
	// so it is built after Designs and handed over by GiveSessions.
	sessions *sessions.Manager

	// designOpens maps an epic address to the *sync.Mutex serializing designs open.
	designOpens sync.Map
	// designSteps maps an epic address to its design step at the last rescan.
	designSteps sync.Map
}

// New builds Designs from d.
func New(d Deps) *Designs {
	if d.History == nil {
		d.History = drafthistory.New(d.StateDir)
	}
	return &Designs{
		log: d.Log, ticketStore: d.TicketStore, conversationsDir: d.ConversationsDir,
		idx: d.Idx, projects: d.Projects, events: d.Events, guards: d.Guards,
		chat: d.Chat, webURL: d.WebURL, workspaces: d.Workspaces,
		onBranch: d.OnBranch, forkHookDir: d.ForkHookDir,
		drafts: drafts.NewWith(d.StateDir, d.History), history: d.History,
	}
}

// Startup migrates draft names, sweeps empty drafts and drops their workspace
// membership. The server calls it once at start; New deletes nothing.
func (s *Designs) Startup() {
	if err := s.drafts.MigrateNames(); err != nil && s.log != nil {
		s.log.Warn("design drafts: name migration failed", "err", err)
	}
	removed, err := s.drafts.SweepEmpty()
	if err != nil && s.log != nil {
		s.log.Warn("design drafts: sweep failed", "err", err)
	}
	s.dropDrafts(removed...)
}

// DraftExists reports whether a New Design draft with this id is stored.
func (s *Designs) DraftExists(id string) bool {
	_, err := s.drafts.Get(id)
	return err == nil
}

// dropDrafts clears the workspace membership of deleted drafts and announces
// the changed rows once. Every path that deletes a draft goes through it, so
// none leaves a dead member or a stale row. A nil workspace store skips the
// membership.
func (s *Designs) dropDrafts(ids ...string) {
	if len(ids) == 0 {
		return
	}
	if s.workspaces != nil {
		for _, id := range ids {
			if err := s.workspaces.Remove(drafts.MemberKey(id)); err != nil && s.log != nil {
				s.log.Warn("drop draft membership", "draft", id, "err", err)
			}
		}
	}
	publishDrafts(s.events)
}

// publishDrafts announces that the draft rows changed.
func publishDrafts(events *broker.Broker) {
	if events != nil {
		events.Publish(wire.EventDesignDraftsChanged, "")
	}
}

// DraftRows lists the drafts as snapshot rows, most recently edited first.
// workspace names the workspace of a member key; "" or an unknown key is Default.
func (s *Designs) DraftRows(workspace func(key string) string) []wire.DesignDraftRow {
	items, err := s.drafts.List()
	if err != nil {
		if s.log != nil {
			s.log.Warn("list design drafts", "err", err)
		}
		return nil
	}
	rows := make([]wire.DesignDraftRow, 0, len(items))
	for _, it := range items {
		key := drafts.MemberKey(it.ID)
		rows = append(rows, wire.DesignDraftRow{
			ID: it.ID, Label: it.Label, Project: it.Project, Slug: it.Slug,
			EditedAt: it.EditedAt, Workspace: workspace(key), MemberKey: key,
		})
	}
	return rows
}

// GiveSessions sets the Manager the design sessions are made and stopped through.
func (s *Designs) GiveSessions(m *sessions.Manager) { s.sessions = m }
