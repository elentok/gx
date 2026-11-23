package server

import (
	"net/http"
	"time"

	"github.com/elentok/gx/server/webauth"
	"github.com/elentok/gx/server/webpush"
)

const pushClientTimeout = 15 * time.Second

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
		Badge: s.BadgeCount,
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
