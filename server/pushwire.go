package server

import (
	"net/http"
	"time"

	"github.com/elentok/gx/server/webauth"
	"github.com/elentok/gx/server/webpush"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

const pushClientTimeout = 15 * time.Second

// badgeCount is the app icon badge: Actionable sessions plus Paused tickets.
// The session rule must match isActionable in webui/src/agents/rules.ts, so the
// badge and the top-bar count agree. Change one, change both.
func badgeCount(sessions []wire.SessionInfo, ts []wire.TicketInfo) int {
	n := 0
	for _, s := range sessions {
		attention := !s.Queue && !s.Design && s.Flow != string(tickets.FlowTriage)
		if attention && (s.Actionable.Reason != "" || s.Status.State == wire.SessionBlocked) {
			n++
		}
	}
	for _, t := range ts {
		if t.EpicIsMap {
			continue
		}
		switch t.Status {
		case string(schema.StatusPaused), string(schema.StatusLegacyAnswer), string(schema.StatusLegacyRepair):
			n++
		}
	}
	return n
}

// liveBadge counts the badge from live state. The push sender calls it, and
// the snapshot handler calls badgeCount on the lists it already holds.
func liveBadge(s *Server) int {
	sessions, _ := s.queue.ListSessionsWithQueue()
	return badgeCount(sessions, s.idx.Snapshot().Tickets)
}

// newPush builds the dispatcher. It is called before the Manager, Writer and
// Recovery, which all hold s.push; the web listener binds later, so the
// subscription adapter reads the gate through s each time.
func newPush(s *Server, quiet func() bool) *webpush.Dispatcher {
	client := s.cfg.PushHTTPClient
	if client == nil {
		client = &http.Client{Timeout: pushClientTimeout}
	}
	sender := &webpush.Sender{
		Client:  client,
		Keys:    s.vapid,
		Contact: func() string { return s.settings.Load().WebPushContact },
		Quiet:   quiet,
	}
	return &webpush.Dispatcher{
		Subs:  pushSubs{s},
		Send:  sender.Send,
		Badge: func() int { return liveBadge(s) },
		Merge: s.cfg.PushMergeDelay,
		Log:   s.log,
	}
}

// pushService is the push routes' view of the server. The routes run only
// behind the web login, so the gate exists by then.
func pushService(s *Server) *webpush.Service {
	return &webpush.Service{
		Store:      s.web.Gate(),
		Dispatcher: s.push,
		Contact:    func() string { return s.settings.Load().WebPushContact },
		PublicKey:  s.vapid.Public,

		AllowHTTPEndpoints: s.cfg.PushAllowHTTPEndpoints,
	}
}

// pushSubs adapts the web login gate, which exists only while the web listener is bound.
type pushSubs struct{ s *Server }

func (p pushSubs) gate() *webauth.Gate {
	if p.s.web == nil {
		return nil
	}
	return p.s.web.Gate()
}

func (p pushSubs) PushTargets() []webauth.PushTarget {
	if g := p.gate(); g != nil {
		return g.PushTargets()
	}
	return nil
}

func (p pushSubs) PushSubscriptionOf(loginID string) (webauth.PushSubscription, bool) {
	if g := p.gate(); g != nil {
		return g.PushSubscriptionOf(loginID)
	}
	return webauth.PushSubscription{}, false
}

func (p pushSubs) DeletePushSubscriptionIfEndpoint(loginID, endpoint string) error {
	if g := p.gate(); g != nil {
		return g.DeletePushSubscriptionIfEndpoint(loginID, endpoint)
	}
	return nil
}
