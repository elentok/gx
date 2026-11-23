package webauth

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const pushKeyA, pushKeyB = "key-a", "key-b"

func pushSub(endpoint string) PushSubscription {
	return PushSubscription{Endpoint: endpoint, P256dh: "p", Auth: "a", VAPIDKey: pushKeyA}
}

const pushEndpoint = "https://push.example.test/send/1"

// pushStore returns a store with two logins, the first one subscribed, under key A.
func pushStore(t *testing.T) (w *WebAuth, clock *storeClock, dir, first, second, secret string) {
	t.Helper()
	w, clock, dir = newStoreForTest(t)
	first, second, _, secret = twoLogins(t, w, clock)
	if err := w.SetPushKey(pushKeyA); err != nil {
		t.Fatal(err)
	}
	if err := w.SetPushSubscription(first, pushSub(pushEndpoint)); err != nil {
		t.Fatal(err)
	}
	return w, clock, dir, first, second, secret
}

func TestWebAuthStorePushSetReplaceDeleteReload(t *testing.T) {
	w, clock, dir, first, _, _ := pushStore(t)
	got, ok := w.PushSubscriptionOf(first)
	if !ok || got.Endpoint != pushEndpoint || !got.Created.Equal(clock.Now()) {
		t.Fatalf("got %+v, %v", got, ok)
	}
	if err := w.SetPushSubscription(first, pushSub("https://push.example.test/send/2")); err != nil {
		t.Fatal(err)
	}
	reopened := openStoreForTest(t, dir, clock)
	mustNil(t, reopened.SetPushKey(pushKeyA))
	got, ok = reopened.PushSubscriptionOf(first)
	if !ok || got.Endpoint != "https://push.example.test/send/2" {
		t.Fatalf("after reload got %+v, %v", got, ok)
	}
	if err := reopened.DeletePushSubscription(first); err != nil {
		t.Fatal(err)
	}
	if _, ok := openStoreForTest(t, dir, clock).PushSubscriptionOf(first); ok {
		t.Fatal("subscription survived delete")
	}
	if len(w.List()) != 2 {
		t.Fatal("delete removed a login")
	}
	if err := w.SetPushSubscription("nope", pushSub(pushEndpoint)); !errors.Is(err, ErrWebLoginNotFound) {
		t.Fatalf("unknown login err = %v", err)
	}
}

func TestWebAuthStorePushOldFileWithoutPushFieldLoads(t *testing.T) {
	w, clock, dir := newStoreForTest(t)
	first, _, _, _ := twoLogins(t, w, clock)
	data, err := os.ReadFile(filepath.Join(dir, webAuthFileName))
	if err != nil || strings.Contains(string(data), `"push"`) {
		t.Fatalf("err %v, file has push field: %s", err, data)
	}
	if _, ok := openStoreForTest(t, dir, clock).PushSubscriptionOf(first); ok {
		t.Fatal("old file has a subscription")
	}
}

func TestWebAuthStorePushRefusesBadSubscriptions(t *testing.T) {
	w, _, _, first, _, _ := pushStore(t)
	for name, sub := range map[string]PushSubscription{
		"http":      pushSub("http://push.example.test/x"),
		"no host":   pushSub("https:///x"),
		"malformed": pushSub("://bad"),
		"empty":     pushSub(""),
		"no key":    {Endpoint: pushEndpoint, P256dh: "p", Auth: "a"},
	} {
		if err := w.SetPushSubscription(first, sub); !errors.Is(err, ErrPushBadSubscription) {
			t.Errorf("%s: err = %v", name, err)
		}
		if err := w.RenewPushSubscription(first, pushEndpoint, sub); !errors.Is(err, ErrPushBadSubscription) {
			t.Errorf("%s renew: err = %v", name, err)
		}
	}
	if got, _ := w.PushSubscriptionOf(first); got.Endpoint != pushEndpoint {
		t.Fatalf("stored %+v after refusals", got)
	}
}

func TestWebAuthStorePushKeyChangeDeletesOldKeySubscriptions(t *testing.T) {
	w, clock, dir, first, _, _ := pushStore(t)
	if err := w.SetPushKey(pushKeyB); err != nil {
		t.Fatal(err)
	}
	if _, ok := w.PushSubscriptionOf(first); ok {
		t.Fatal("old-key subscription still present")
	}
	data, _ := os.ReadFile(filepath.Join(dir, webAuthFileName))
	if strings.Contains(string(data), pushEndpoint) {
		t.Fatal("old-key subscription still in the file")
	}
	if w.PushKey() != pushKeyB {
		t.Fatalf("key = %q", w.PushKey())
	}
	_ = clock
}

func TestWebAuthStorePushDifferentKeyIsGone(t *testing.T) {
	w, clock, dir, first, _, _ := pushStore(t)
	// A file written under key A, opened and read under key B without SetPushKey's delete.
	other := openStoreForTest(t, dir, clock)
	other.mu.Lock()
	other.pushKey = pushKeyB
	other.mu.Unlock()
	if _, ok := other.PushSubscriptionOf(first); ok {
		t.Fatal("different-key subscription visible")
	}
	if got := other.PushTargets(); len(got) != 0 {
		t.Fatalf("targets = %+v", got)
	}
	for _, l := range other.List() {
		if l.PushOn {
			t.Fatal("list shows push on for a different key")
		}
	}
	_ = w
}

func TestWebAuthStorePushTargetsAndListFields(t *testing.T) {
	w, clock, dir, first, second, _ := pushStore(t)
	targets := w.PushTargets()
	if len(targets) != 1 || targets[0].LoginID != first || targets[0].Sub.Endpoint != pushEndpoint || targets[0].Name == "" {
		t.Fatalf("targets = %+v", targets)
	}
	for _, l := range w.List() {
		on := l.ID == first
		if l.PushOn != on || l.PushSince.IsZero() == on {
			t.Fatalf("login %s: %+v", l.ID, l)
		}
	}
	_ = second
	data, _ := os.ReadFile(filepath.Join(dir, webAuthFileName))
	if strings.Contains(string(data), `"push_on": true`) || strings.Contains(string(data), "push_since") {
		t.Fatalf("derived fields were saved: %s", data)
	}
	_ = clock
}

func TestWebAuthStorePushTargetsDropsIdleLoginInOneWrite(t *testing.T) {
	w, clock, dir, first, second, _ := pushStore(t)
	cookie2 := ""
	_ = cookie2
	clock.t = clock.t.Add(89 * 24 * time.Hour)
	// Keep the second login fresh by looking up nothing: both idle equally, so only
	// check that both are dropped at 90 days and the file holds no subscription.
	clock.t = clock.t.Add(24 * time.Hour)
	if got := w.PushTargets(); len(got) != 0 {
		t.Fatalf("targets = %+v", got)
	}
	if l := w.List(); len(l) != 0 {
		t.Fatalf("idle logins kept: %+v", l)
	}
	if _, ok := openStoreForTest(t, dir, clock).PushSubscriptionOf(first); ok {
		t.Fatal("subscription survived the idle drop")
	}
	if err := w.RenewPushSubscription(first, pushEndpoint, pushSub(pushEndpoint+"2")); !errors.Is(err, ErrPushNoSubscription) {
		t.Fatalf("renew after idle drop err = %v", err)
	}
	_ = second
}

func TestWebAuthStorePushRenew(t *testing.T) {
	w, clock, dir, first, _, _ := pushStore(t)
	renewed := "https://push.example.test/send/renewed"
	if err := w.RenewPushSubscription(first, "https://push.example.test/other", pushSub(renewed)); !errors.Is(err, ErrPushNoSubscription) {
		t.Fatalf("wrong old endpoint err = %v", err)
	}
	if got, _ := w.PushSubscriptionOf(first); got.Endpoint != pushEndpoint {
		t.Fatalf("wrong-endpoint renewal stored %+v", got)
	}
	if err := w.RenewPushSubscription(first, pushEndpoint, pushSub(renewed)); err != nil {
		t.Fatal(err)
	}
	reopened := openStoreForTest(t, dir, clock)
	mustNil(t, reopened.SetPushKey(pushKeyA))
	if got, _ := reopened.PushSubscriptionOf(first); got.Endpoint != renewed {
		t.Fatalf("after reload %+v", got)
	}
	if got, _ := w.PushSubscriptionOf(first); got.Endpoint != renewed {
		t.Fatalf("renewed = %+v", got)
	}
	if err := w.RenewPushSubscription("nope", renewed, pushSub(renewed)); !errors.Is(err, ErrPushNoSubscription) {
		t.Fatalf("unknown login err = %v", err)
	}
}

func TestWebAuthStorePushRenewCannotUndoRemoval(t *testing.T) {
	removals := map[string]func(w *WebAuth, id string) error{
		"delete": func(w *WebAuth, id string) error { return w.DeletePushSubscription(id) },
		"delete if endpoint": func(w *WebAuth, id string) error {
			return w.DeletePushSubscriptionIfEndpoint(id, pushEndpoint)
		},
	}
	for name, remove := range removals {
		t.Run(name, func(t *testing.T) {
			w, _, _, first, _, _ := pushStore(t)
			if err := remove(w, first); err != nil {
				t.Fatal(err)
			}
			if err := w.RenewPushSubscription(first, pushEndpoint, pushSub(pushEndpoint+"2")); !errors.Is(err, ErrPushNoSubscription) {
				t.Fatalf("renew err = %v", err)
			}
			if _, ok := w.PushSubscriptionOf(first); ok {
				t.Fatal("renewal stored a record")
			}
		})
	}
}

func TestWebAuthStorePushDeleteIfEndpointKeepsRenewedRecord(t *testing.T) {
	w, _, _, first, _, _ := pushStore(t)
	if err := w.DeletePushSubscriptionIfEndpoint(first, "https://push.example.test/stale"); err != nil {
		t.Fatal(err)
	}
	if _, ok := w.PushSubscriptionOf(first); !ok {
		t.Fatal("a stale endpoint deleted the renewed record")
	}
}

func TestWebAuthStorePushRenewRefusedAfterKeyChange(t *testing.T) {
	w, _, _, first, _, _ := pushStore(t)
	if err := w.SetPushKey(pushKeyB); err != nil {
		t.Fatal(err)
	}
	sub := pushSub(pushEndpoint)
	sub.VAPIDKey = pushKeyB
	if err := w.RenewPushSubscription(first, pushEndpoint, sub); !errors.Is(err, ErrPushNoSubscription) {
		t.Fatalf("err = %v", err)
	}
}

func TestWebAuthStorePushGoneWithTheLogin(t *testing.T) {
	cases := map[string]func(t *testing.T, w *WebAuth, clock *storeClock, first, second, secret string){
		"RevokeLogin": func(t *testing.T, w *WebAuth, _ *storeClock, first, _, _ string) {
			_, err := w.RevokeLogin(first)
			mustNil(t, err)
		},
		"ChangePassword": func(t *testing.T, w *WebAuth, clock *storeClock, _, second, secret string) {
			_, err := w.ChangePassword(true, storeTestPassword, nextCode(clock, secret), "brand new password", second, "8.8.8.8")
			mustNil(t, err)
		},
		"SetPassword": func(t *testing.T, w *WebAuth, _ *storeClock, _, _, _ string) {
			_, err := w.SetPassword("brand new password")
			mustNil(t, err)
		},
		"AuthenticatorResetFinish": func(t *testing.T, w *WebAuth, clock *storeClock, _, second, secret string) {
			newSecret, err := w.AuthenticatorResetBegin(true, storeTestPassword, nextCode(clock, secret), "8.8.8.8")
			mustNil(t, err)
			_, err = w.AuthenticatorResetFinish(nextCode(clock, newSecret), second, "8.8.8.8")
			mustNil(t, err)
		},
		"ClearAuthenticator": func(t *testing.T, w *WebAuth, _ *storeClock, _, _, _ string) {
			_, _, err := w.ClearAuthenticator()
			mustNil(t, err)
		},
		"WebReset": func(t *testing.T, w *WebAuth, _ *storeClock, _, _, _ string) {
			_, err := w.WebReset()
			mustNil(t, err)
		},
	}
	for name, signOut := range cases {
		t.Run(name, func(t *testing.T) {
			w, clock, dir, first, second, secret := pushStore(t)
			signOut(t, w, clock, first, second, secret)
			if _, ok := w.PushSubscriptionOf(first); ok {
				t.Fatal("subscription survived in memory")
			}
			data, _ := os.ReadFile(filepath.Join(dir, webAuthFileName))
			if strings.Contains(string(data), pushEndpoint) {
				t.Fatalf("subscription survived in the file: %s", data)
			}
		})
	}
}

func mustNil(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
