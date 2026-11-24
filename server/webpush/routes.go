package webpush

import (
	"crypto/ecdh"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/elentok/gx/server/webauth"
	"github.com/elentok/gx/server/wire"
)

const (
	pathSubscribe = "/v1/web/push/subscribe"
	pathRemove    = "/v1/web/push/remove"
	pathRenew     = "/v1/web/push/renew"
	pathTest      = "/v1/web/push/test"
	pathState     = "/v1/web/push/state/"

	routeBodyMax = 8 << 10
	authBytes    = 16
)

// Store is the part of the web login gate the push routes write through.
type Store interface {
	PushSubscriptionOf(loginID string) (webauth.PushSubscription, bool)
	SetPushSubscription(loginID string, sub webauth.PushSubscription) error
	RenewPushSubscription(loginID, oldEndpoint string, sub webauth.PushSubscription) error
	DeletePushSubscription(loginID string) error
}

// Service is what the push routes need. Contact is the configured email: it is
// only ever tested for validity, never written to a response or a log line.
type Service struct {
	Store      Store
	Dispatcher *Dispatcher
	Contact    func() string
	// On is the server-wide Push switch; nil means on.
	On        func() bool
	PublicKey func() string
	// AllowHTTPEndpoints is a test seam: accept an http: endpoint, for the
	// fixture's loopback push service. False for every user.
	AllowHTTPEndpoints bool
}

type routeBody struct {
	LoginID     string `json:"login_id"`
	Endpoint    string `json:"endpoint"`
	P256dh      string `json:"p256dh"`
	Auth        string `json:"auth"`
	VAPIDKey    string `json:"vapid_key"`
	OldEndpoint string `json:"old_endpoint"`
}

type routeResult struct {
	Refused bool   `json:"refused,omitempty"`
	Reason  string `json:"reason,omitempty"`
}

type testResult struct {
	routeResult
	Sent    bool   `json:"sent"`
	Failure string `json:"failure,omitempty"`
}

type stateFailure struct {
	Reason string    `json:"reason"`
	At     time.Time `json:"at"`
}

type stateResult struct {
	PublicKey   string        `json:"public_key"`
	ContactSet  bool          `json:"contact_set"`
	ServerOn    bool          `json:"server_on"`
	Subscribed  bool          `json:"subscribed"`
	Endpoint    string        `json:"endpoint"`
	LastFailure *stateFailure `json:"last_failure,omitempty"`
}

func refuse(w http.ResponseWriter, reason string) {
	wire.WriteJSON(w, routeResult{Refused: true, Reason: reason})
}

// Routes are the web-only push routes. The service resolves lazily: the routes
// are listed before the web listener is bound.
func Routes(get func() *Service) []wire.Route {
	h := func(f func(*Service, http.ResponseWriter, *http.Request)) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) { f(get(), w, r) }
	}
	return []wire.Route{
		{Pattern: "POST " + pathSubscribe, Web: wire.WebWrite, WebHandler: h((*Service).subscribe)},
		{Pattern: "POST " + pathRemove, Web: wire.WebWrite, WebHandler: h((*Service).remove)},
		{Pattern: "POST " + pathRenew, Web: wire.WebWrite, WebHandler: h((*Service).renew)},
		{Pattern: "POST " + pathTest, Web: wire.WebWrite, WebHandler: h((*Service).test)},
		{Pattern: "GET " + pathState, Web: wire.WebReadPrefix, WebHandler: h((*Service).state)},
	}
}

func readBody(w http.ResponseWriter, r *http.Request) (routeBody, bool) {
	var b routeBody
	r.Body = http.MaxBytesReader(w, r.Body, routeBodyMax)
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		http.Error(w, "bad JSON", http.StatusBadRequest)
		return b, false
	}
	return b, true
}

func (s *Service) contactSet() bool { return ValidContact(s.Contact()) }

func (s *Service) switchOn() bool { return s.On == nil || s.On() }

// validSubscription is whether b is a subscription a browser could have made
// with the server's key: an https endpoint and keys the encryption can use.
func (s *Service) validSubscription(b routeBody) bool {
	u, err := url.Parse(b.Endpoint)
	if err != nil || (u.Scheme != "https" && !(s.AllowHTTPEndpoints && u.Scheme == "http")) || u.Host == "" || b.VAPIDKey != s.PublicKey() {
		return false
	}
	p, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(b.P256dh, "="))
	if err != nil {
		return false
	}
	if _, err := ecdh.P256().NewPublicKey(p); err != nil {
		return false
	}
	a, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(b.Auth, "="))
	return err == nil && len(a) == authBytes
}

func subscriptionOfBody(b routeBody) webauth.PushSubscription {
	return webauth.PushSubscription{Endpoint: b.Endpoint, P256dh: b.P256dh, Auth: b.Auth, VAPIDKey: b.VAPIDKey}
}

// ownBody reads the body of a route that never takes another login's id.
func ownBody(w http.ResponseWriter, r *http.Request) (routeBody, string, bool) {
	b, ok := readBody(w, r)
	if !ok {
		return b, "", false
	}
	me := webauth.LoginIDFrom(r.Context())
	if b.LoginID != "" && b.LoginID != me {
		refuse(w, wire.ReasonPushNotYours)
		return b, me, false
	}
	return b, me, true
}

// refuseStoreErr answers a store error with its refusal, or a 500.
func refuseStoreErr(w http.ResponseWriter, err error) {
	switch {
	case err == nil:
		wire.WriteJSON(w, routeResult{})
	case errors.Is(err, webauth.ErrPushNoSubscription):
		refuse(w, wire.ReasonPushNoSubscription)
	case errors.Is(err, webauth.ErrWebLoginNotFound):
		refuse(w, wire.ReasonLoginNotFound)
	default:
		http.Error(w, "push unavailable", http.StatusInternalServerError)
	}
}

func (s *Service) subscribe(w http.ResponseWriter, r *http.Request) {
	b, me, ok := ownBody(w, r)
	if !ok {
		return
	}
	if !s.contactSet() {
		refuse(w, wire.ReasonPushNoContact)
		return
	}
	if !s.validSubscription(b) {
		refuse(w, wire.ReasonPushBadSubscription)
		return
	}
	refuseStoreErr(w, s.Store.SetPushSubscription(me, subscriptionOfBody(b)))
}

func (s *Service) remove(w http.ResponseWriter, r *http.Request) {
	b, ok := readBody(w, r)
	if !ok {
		return
	}
	id := b.LoginID
	if id == "" {
		id = webauth.LoginIDFrom(r.Context())
	}
	refuseStoreErr(w, s.Store.DeletePushSubscription(id))
}

func (s *Service) renew(w http.ResponseWriter, r *http.Request) {
	b, me, ok := ownBody(w, r)
	if !ok {
		return
	}
	if !s.validSubscription(b) {
		refuse(w, wire.ReasonPushBadSubscription)
		return
	}
	refuseStoreErr(w, s.Store.RenewPushSubscription(me, b.OldEndpoint, subscriptionOfBody(b)))
}

func (s *Service) test(w http.ResponseWriter, r *http.Request) {
	_, me, ok := ownBody(w, r)
	if !ok {
		return
	}
	if !s.switchOn() {
		refuse(w, wire.ReasonPushSwitchOff)
		return
	}
	if !s.contactSet() {
		refuse(w, wire.ReasonPushNoContact)
		return
	}
	if _, has := s.Store.PushSubscriptionOf(me); !has {
		refuse(w, wire.ReasonPushNoSubscription)
		return
	}
	out := s.Dispatcher.TestPush(r.Context(), me)
	res := testResult{Sent: out.Delivered, Failure: out.Reason}
	if !out.Delivered && out.Reason == "" {
		res.Failure = "The push service did not accept the test push."
	}
	wire.WriteJSON(w, res)
}

func (s *Service) state(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != pathState {
		http.NotFound(w, r)
		return
	}
	me := webauth.LoginIDFrom(r.Context())
	out := stateResult{PublicKey: s.PublicKey(), ContactSet: s.contactSet(), ServerOn: s.switchOn()}
	if sub, ok := s.Store.PushSubscriptionOf(me); ok {
		out.Subscribed, out.Endpoint = true, sub.Endpoint
	}
	if reason, at, ok := s.Dispatcher.LastFailure(me); ok {
		out.LastFailure = &stateFailure{Reason: reason, At: at}
	}
	wire.WriteJSON(w, out)
}
