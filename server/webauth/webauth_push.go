package webauth

import (
	"errors"
	"time"

	"github.com/elentok/gx/server/wire/ops"
)

var (
	// ErrPushNoSubscription is a renewal for a login with no subscription at the
	// old endpoint.
	ErrPushNoSubscription = errors.New("no push subscription to renew")
)

// PushSubscription is the one push subscription a web login can hold. VAPIDKey
// is the public key (base64url) the browser subscribed with.
type PushSubscription struct {
	Endpoint string    `json:"endpoint"`
	P256dh   string    `json:"p256dh"`
	Auth     string    `json:"auth"`
	VAPIDKey string    `json:"vapid_key"`
	Created  time.Time `json:"created"`
}

// PushTarget is a login that should get a push now.
type PushTarget struct {
	LoginID string
	Name    string
	Sub     PushSubscription
}

// SetPushKey remembers the server's VAPID public key and deletes every
// subscription made with another key, in one write.
func (w *WebAuth) SetPushKey(public string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.pushKey = public
	changed := false
	next := w.file
	next.Logins = make([]*webLoginRecord, len(w.file.Logins))
	for i, rec := range w.file.Logins {
		next.Logins[i] = rec
		if rec.Push != nil && rec.Push.VAPIDKey != public {
			cleared := *rec
			cleared.Push = nil
			next.Logins[i] = &cleared
			changed = true
		}
	}
	if !changed {
		return nil
	}
	return w.commitLocked(next)
}

// PushKey is the VAPID public key SetPushKey last got.
func (w *WebAuth) PushKey() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.pushKey
}

// SetPushSubscription stores sub as the login's subscription, replacing any
// earlier one.
func (w *WebAuth) SetPushSubscription(loginID string, sub PushSubscription) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.updatePushLocked(loginID, func(*PushSubscription) (*PushSubscription, error) {
		sub.Created = w.now()
		return &sub, nil
	})
}

// RenewPushSubscription replaces the login's subscription only when it is still
// at oldEndpoint under the current key, so a renewal cannot undo a removal.
func (w *WebAuth) RenewPushSubscription(loginID, oldEndpoint string, sub PushSubscription) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	err := w.updatePushLocked(loginID, func(cur *PushSubscription) (*PushSubscription, error) {
		if cur == nil || cur.Endpoint != oldEndpoint || cur.VAPIDKey != w.pushKey || sub.VAPIDKey != w.pushKey {
			return nil, ErrPushNoSubscription
		}
		sub.Created = w.now()
		return &sub, nil
	})
	if errors.Is(err, ErrWebLoginNotFound) {
		// A login dropped while the renewal was in flight is a missing subscription.
		return ErrPushNoSubscription
	}
	return err
}

// DeletePushSubscription removes the login's subscription and keeps the login.
func (w *WebAuth) DeletePushSubscription(loginID string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.updatePushLocked(loginID, func(*PushSubscription) (*PushSubscription, error) { return nil, nil })
}

// DeletePushSubscriptionIfEndpoint removes the subscription only when it is
// still at endpoint, so a concurrent renewal survives a 404 or 410 for the old one.
func (w *WebAuth) DeletePushSubscriptionIfEndpoint(loginID, endpoint string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.updatePushLocked(loginID, func(cur *PushSubscription) (*PushSubscription, error) {
		if cur == nil || cur.Endpoint != endpoint {
			return cur, nil
		}
		return nil, nil
	})
}

// updatePushLocked replaces the login's Push with what change returns, copy on
// write. It writes nothing when the result equals the current value.
func (w *WebAuth) updatePushLocked(loginID string, change func(cur *PushSubscription) (*PushSubscription, error)) error {
	next := w.file
	next.Logins = make([]*webLoginRecord, len(w.file.Logins))
	found := false
	for i, rec := range w.file.Logins {
		next.Logins[i] = rec
		if rec.ID != loginID {
			continue
		}
		found = true
		updated, err := change(rec.Push)
		if err != nil {
			return err
		}
		if updated == rec.Push {
			return nil
		}
		copied := *rec
		copied.Push = updated
		next.Logins[i] = &copied
	}
	if !found {
		return ErrWebLoginNotFound
	}
	return w.commitLocked(next)
}

// PushSubscriptionOf is the login's subscription. One made with another key counts as absent.
func (w *WebAuth) PushSubscriptionOf(loginID string) (PushSubscription, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, rec := range w.file.Logins {
		if rec.ID == loginID && w.pushCurrentLocked(rec) {
			return *rec.Push, true
		}
	}
	return PushSubscription{}, false
}

// PushTargets drops every login idle for 90 days with its subscription, then
// returns the logins that hold a subscription under the current key.
func (w *WebAuth) PushTargets() []PushTarget {
	w.mu.Lock()
	defer w.mu.Unlock()
	now := w.now()
	var idle []string
	for _, rec := range w.file.Logins {
		if now.Sub(rec.LastUsed) >= webLoginIdleLimit {
			idle = append(idle, rec.ID)
		}
	}
	for _, id := range idle {
		w.dropLoginLocked(id)
	}
	var out []PushTarget
	for _, rec := range w.file.Logins {
		if w.pushCurrentLocked(rec) {
			out = append(out, PushTarget{LoginID: rec.ID, Name: rec.Name, Sub: *rec.Push})
		}
	}
	return out
}

func (w *WebAuth) pushCurrentLocked(rec *webLoginRecord) bool {
	return rec.Push != nil && rec.Push.VAPIDKey == w.pushKey
}

// viewLocked is the record's wire login with the push fields filled in. The
// record's own WebLogin never carries them, or they would be saved.
func (w *WebAuth) viewLocked(rec *webLoginRecord) ops.WebLogin {
	out := rec.WebLogin
	if w.pushCurrentLocked(rec) {
		out.PushOn, out.PushSince = true, rec.Push.Created
	}
	return out
}
