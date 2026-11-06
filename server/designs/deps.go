package designs

import (
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/elentok/gx/ralphloop/notify"
	"github.com/elentok/gx/server/broker"
	"github.com/elentok/gx/server/index"
	"github.com/elentok/gx/server/projects"
	"github.com/elentok/gx/server/runs"
	"github.com/elentok/gx/server/sessions"
)

// Deps is everything the design code copies when it is built. A field the root
// swaps later is not seen.
type Deps struct {
	Log              *slog.Logger
	TicketStore      string
	ConversationsDir string
	Idx              *index.Index
	Projects         *projects.Registry
	Events           *broker.Broker
	Guards           *runs.Guards
	// Chat sends the Ready message; WebURL is the web listener's base URL, "" when off.
	Chat   *notify.ServerChat
	WebURL func() string
}

// Designs opens, cleans up and deletes the Design sessions of map epics.
type Designs struct {
	log              *slog.Logger
	ticketStore      string
	conversationsDir string
	idx              *index.Index
	projects         *projects.Registry
	events           *broker.Broker
	guards           *runs.Guards
	chat             *notify.ServerChat
	webURL           func() string
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
	return &Designs{
		log: d.Log, ticketStore: d.TicketStore, conversationsDir: d.ConversationsDir,
		idx: d.Idx, projects: d.Projects, events: d.Events, guards: d.Guards,
		chat: d.Chat, webURL: d.WebURL,
	}
}

// GiveSessions sets the Manager the design sessions are made and stopped through.
func (s *Designs) GiveSessions(m *sessions.Manager) { s.sessions = m }
