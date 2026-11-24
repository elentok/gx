package webpush_test

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/webauth"
	"github.com/elentok/gx/server/wire"
)

const testEmail = "me@example.com"

type pushWeb struct {
	h             *servertest.Harness
	base          string
	offset        atomic.Int64
	hits          atomic.Int32
	status        atomic.Int32
	first, second *http.Cookie
	secondID      string
	svcURL        string
}

func (p *pushWeb) now() time.Time { return time.Now().Add(time.Duration(p.offset.Load())) }

// startPushWeb is a web listener with two logins, a fake push service and the given contact email.
func startPushWeb(t *testing.T, contact string, tweak ...func(*server.Config)) *pushWeb {
	t.Helper()
	p := &pushWeb{}
	p.status.Store(http.StatusCreated)
	svc := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p.hits.Add(1)
		w.WriteHeader(int(p.status.Load()))
	}))
	t.Cleanup(svc.Close)
	p.svcURL = svc.URL
	p.h = servertest.StartWithStore(t, t.TempDir(), func(c *server.Config) {
		c.WebAddr = "127.0.0.1:0"
		c.Clock = p.now
		c.PushHTTPClient = svc.Client()
		c.Settings.WebPushContact = contact
		for _, f := range tweak {
			f(c)
		}
	})
	hs, err := p.h.Client.Handshake(context.Background())
	if err != nil || hs.WebURL == "" {
		t.Fatalf("handshake %+v, %v", hs, err)
	}
	p.base = servertest.WebBase(hs.WebURL)
	var secret string
	p.first, secret = servertest.SignIn(t, p.h, p.base, p.now)
	p.offset.Add(int64(31 * time.Second))
	code, err := webauth.TOTPCode(secret, p.now().Unix()/30)
	if err != nil {
		t.Fatal(err)
	}
	resp, _ := p.post(t, "/v1/web/auth/login", fmt.Sprintf(`{"password":%q,"totp_code":%q}`, servertest.LoginPassword, code), nil)
	if len(resp.Cookies()) != 1 {
		t.Fatalf("second login: %d cookies", len(resp.Cookies()))
	}
	p.second = resp.Cookies()[0]
	for _, l := range p.logins(t) {
		if !l.ThisDevice {
			p.secondID = l.ID
		}
	}
	if p.secondID == "" {
		t.Fatal("no second login id")
	}
	return p
}

type loginRow struct {
	ID         string `json:"id"`
	ThisDevice bool   `json:"this_device"`
}

// logins lists the logins as the first one sees them.
func (p *pushWeb) logins(t *testing.T) []loginRow {
	t.Helper()
	_, body := p.post(t, "/v1/web/logins", `{}`, p.first)
	var out struct{ Logins []loginRow }
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatalf("logins body %q: %v", body, err)
	}
	return out.Logins
}

func (p *pushWeb) post(t *testing.T, path, body string, cookie *http.Cookie) (*http.Response, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, p.base+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range servertest.JSONPost(p.base) {
		req.Header.Set(k, v)
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	resp, err := servertest.WebClient().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	return resp, string(out)
}

func (p *pushWeb) reason(t *testing.T, path, body string, cookie *http.Cookie) string {
	t.Helper()
	_, out := p.post(t, path, body, cookie)
	return servertest.ReasonOf(t, out)
}

type pushState struct {
	PublicKey   string `json:"public_key"`
	ContactSet  bool   `json:"contact_set"`
	ServerOn    bool   `json:"server_on"`
	Subscribed  bool   `json:"subscribed"`
	Endpoint    string `json:"endpoint"`
	LastFailure *struct {
		Reason string `json:"reason"`
	} `json:"last_failure"`
}

func (p *pushWeb) state(t *testing.T, cookie *http.Cookie) (pushState, string) {
	t.Helper()
	code, body := servertest.WebDoCookie(t, http.MethodGet, p.base+"/v1/web/push/state/", cookie)
	if code != http.StatusOK {
		t.Fatalf("state: status %d, body %q", code, body)
	}
	var s pushState
	if err := json.Unmarshal([]byte(body), &s); err != nil {
		t.Fatalf("state body %q: %v", body, err)
	}
	return s, body
}

// subBody is a subscription body a browser could send.
func subBody(t *testing.T, key, endpoint string, extra string) string {
	t.Helper()
	ua, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	auth := make([]byte, 16)
	_, _ = rand.Read(auth)
	b64 := base64.RawURLEncoding
	return fmt.Sprintf(`{"endpoint":%q,"p256dh":%q,"auth":%q,"vapid_key":%q%s}`,
		endpoint, b64.EncodeToString(ua.PublicKey().Bytes()), b64.EncodeToString(auth), key, extra)
}

func TestPushRoutes_HTTPEndpointNeedsTheTestHook(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name   string
		tweak  func(*server.Config)
		reason string
	}{
		{"default", func(*server.Config) {}, wire.ReasonPushBadSubscription},
		{"hook on", func(c *server.Config) { c.PushAllowHTTPEndpoints = true }, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p := startPushWeb(t, testEmail, tt.tweak)
			st, _ := p.state(t, p.first)
			body := subBody(t, st.PublicKey, "http://127.0.0.1:9/push/1", "")
			if r := p.reason(t, "/v1/web/push/subscribe", body, p.first); r != tt.reason {
				t.Errorf("reason %q, want %q", r, tt.reason)
			}
		})
	}
}

func TestPushRoutes_SubscribeStoresAndStateShowsIt(t *testing.T) {
	t.Parallel()
	p := startPushWeb(t, testEmail)
	st, _ := p.state(t, p.first)
	if st.Subscribed || st.PublicKey == "" || !st.ContactSet {
		t.Fatalf("initial state %+v", st)
	}
	if r := p.reason(t, "/v1/web/push/subscribe", subBody(t, st.PublicKey, "https://push.example/a", ""), p.first); r != "" {
		t.Fatalf("subscribe refused: %q", r)
	}
	got, _ := p.state(t, p.first)
	if !got.Subscribed || got.Endpoint != "https://push.example/a" || got.PublicKey != st.PublicKey {
		t.Fatalf("state after subscribe %+v", got)
	}
	if other, _ := p.state(t, p.second); other.Subscribed {
		t.Error("the other login shows a subscription it never made")
	}
	if r := p.reason(t, "/v1/web/push/subscribe", subBody(t, st.PublicKey, "https://push.example/b", ""), p.first); r != "" {
		t.Fatalf("second subscribe refused: %q", r)
	}
	if got, _ := p.state(t, p.first); got.Endpoint != "https://push.example/b" {
		t.Errorf("endpoint %q, want the second subscription to replace the first", got.Endpoint)
	}
}

func TestPushRoutes_RefuseBadSubscription(t *testing.T) {
	t.Parallel()
	p := startPushWeb(t, testEmail)
	st, _ := p.state(t, p.first)
	good := subBody(t, st.PublicKey, "https://push.example/a", "")
	bad := map[string]string{
		"http endpoint":   subBody(t, st.PublicKey, "http://push.example/a", ""),
		"not a URL":       subBody(t, st.PublicKey, "::", ""),
		"missing auth":    strings.Replace(good, `"auth"`, `"x"`, 1),
		"auth not base64": strings.Replace(good, `"auth":"`, `"auth":"!!`, 1),
		"key not a point": strings.Replace(good, `"p256dh":"`, `"p256dh":"AAAA`, 1),
		"other vapid key": subBody(t, "other", "https://push.example/a", ""),
	}
	for name, body := range bad {
		if r := p.reason(t, "/v1/web/push/subscribe", body, p.first); r != wire.ReasonPushBadSubscription {
			t.Errorf("%s: reason %q, want %q", name, r, wire.ReasonPushBadSubscription)
		}
	}
	if got, _ := p.state(t, p.first); got.Subscribed {
		t.Error("a refused subscription was stored")
	}

	noMail := startPushWeb(t, "")
	st2, _ := noMail.state(t, noMail.first)
	if st2.ContactSet {
		t.Error("contact_set is true with no email")
	}
	if r := noMail.reason(t, "/v1/web/push/subscribe", subBody(t, st2.PublicKey, "https://push.example/a", ""), noMail.first); r != wire.ReasonPushNoContact {
		t.Errorf("no email: reason %q, want %q", r, wire.ReasonPushNoContact)
	}
	if got, _ := noMail.state(t, noMail.first); got.Subscribed {
		t.Error("a subscription was stored without a contact email")
	}
}

func TestPushRoutes_RemoveOnlyDeletesAndNotYours(t *testing.T) {
	t.Parallel()
	p := startPushWeb(t, testEmail)
	st, _ := p.state(t, p.first)
	for _, c := range []*http.Cookie{p.first, p.second} {
		if r := p.reason(t, "/v1/web/push/subscribe", subBody(t, st.PublicKey, "https://push.example/a", ""), c); r != "" {
			t.Fatalf("subscribe refused: %q", r)
		}
	}
	if r := p.reason(t, "/v1/web/push/remove", fmt.Sprintf(`{"login_id":%q}`, p.secondID), p.first); r != "" {
		t.Fatalf("remove refused: %q", r)
	}
	if got, _ := p.state(t, p.second); got.Subscribed {
		t.Error("the removed login still has a subscription")
	}
	if got, _ := p.state(t, p.first); !got.Subscribed {
		t.Error("remove deleted the caller's own subscription too")
	}
	if len(p.logins(t)) != 2 {
		t.Error("remove deleted a login")
	}
	if r := p.reason(t, "/v1/web/push/remove", fmt.Sprintf(`{"login_id":%q}`, p.secondID), p.first); r != "" {
		t.Errorf("removing a login with no subscription: reason %q, want none", r)
	}
	if r := p.reason(t, "/v1/web/push/remove", `{"login_id":"nope"}`, p.first); r != wire.ReasonLoginNotFound {
		t.Errorf("unknown login: reason %q, want %q", r, wire.ReasonLoginNotFound)
	}
	if r := p.reason(t, "/v1/web/push/remove", `{}`, p.first); r != "" {
		t.Fatalf("own remove refused: %q", r)
	}
	if got, _ := p.state(t, p.first); got.Subscribed {
		t.Error("remove with no id left the caller's subscription")
	}

	other := fmt.Sprintf(`,"login_id":%q`, p.secondID)
	for _, path := range []string{"/v1/web/push/subscribe", "/v1/web/push/renew"} {
		body := subBody(t, st.PublicKey, "https://push.example/c", other+`,"old_endpoint":"https://push.example/a"`)
		if r := p.reason(t, path, body, p.first); r != wire.ReasonPushNotYours {
			t.Errorf("%s: reason %q, want %q", path, r, wire.ReasonPushNotYours)
		}
	}
	if r := p.reason(t, "/v1/web/push/test", fmt.Sprintf(`{"login_id":%q}`, p.secondID), p.first); r != wire.ReasonPushNotYours {
		t.Errorf("test: reason %q, want %q", r, wire.ReasonPushNotYours)
	}
	if got, _ := p.state(t, p.second); got.Subscribed {
		t.Error("a refused not-yours write stored a subscription")
	}
	if p.hits.Load() != 0 {
		t.Error("a refused test push reached the push service")
	}
}

func TestPushRoutes_RenewNeedsRecordAndOldEndpoint(t *testing.T) {
	t.Parallel()
	p := startPushWeb(t, testEmail)
	st, _ := p.state(t, p.first)
	const a, b, c = "https://push.example/a", "https://push.example/b", "https://push.example/c"
	renew := func(old, next string) string {
		return p.reason(t, "/v1/web/push/renew", subBody(t, st.PublicKey, next, fmt.Sprintf(`,"old_endpoint":%q`, old)), p.first)
	}
	if r := renew(a, b); r != wire.ReasonPushNoSubscription {
		t.Fatalf("renew with no record: reason %q, want %q", r, wire.ReasonPushNoSubscription)
	}
	if got, _ := p.state(t, p.first); got.Subscribed {
		t.Fatal("a renewal with no record stored a subscription")
	}
	_ = p.reason(t, "/v1/web/push/subscribe", subBody(t, st.PublicKey, a, ""), p.first)
	if r := renew("https://push.example/wrong", c); r != wire.ReasonPushNoSubscription {
		t.Errorf("renew with a wrong old endpoint: reason %q", r)
	}
	if got, _ := p.state(t, p.first); got.Endpoint != a {
		t.Errorf("endpoint %q after a refused renewal, want %q", got.Endpoint, a)
	}
	if r := renew(a, "http://push.example/b"); r != wire.ReasonPushBadSubscription {
		t.Errorf("non-https renewal: reason %q, want %q", r, wire.ReasonPushBadSubscription)
	}
	if r := renew(a, b); r != "" {
		t.Fatalf("renew refused: %q", r)
	}
	if got, _ := p.state(t, p.first); got.Endpoint != b {
		t.Errorf("endpoint %q after renewal, want %q", got.Endpoint, b)
	}
	_ = p.reason(t, "/v1/web/push/remove", `{}`, p.first)
	if r := renew(b, c); r != wire.ReasonPushNoSubscription {
		t.Errorf("renew after remove: reason %q, want %q", r, wire.ReasonPushNoSubscription)
	}
	if got, _ := p.state(t, p.first); got.Subscribed {
		t.Error("a renewal after remove brought the subscription back")
	}
}

func TestPushRoutes_TestPushOneAttemptAndFailureReason(t *testing.T) {
	t.Parallel()
	p := startPushWeb(t, testEmail)
	if r := p.reason(t, "/v1/web/push/test", `{}`, p.first); r != wire.ReasonPushNoSubscription {
		t.Fatalf("test with no subscription: reason %q, want %q", r, wire.ReasonPushNoSubscription)
	}
	st, _ := p.state(t, p.first)
	_ = p.reason(t, "/v1/web/push/subscribe", subBody(t, st.PublicKey, p.svcURL+"/push/x", ""), p.first)

	_, body := p.post(t, "/v1/web/push/test", `{}`, p.first)
	if !strings.Contains(body, `"sent":true`) || p.hits.Load() != 1 {
		t.Fatalf("test push body %q, %d hits; want sent once", body, p.hits.Load())
	}
	if got, _ := p.state(t, p.first); got.LastFailure != nil {
		t.Errorf("last_failure %+v before any failure", got.LastFailure)
	}

	p.status.Store(http.StatusInternalServerError)
	_, body = p.post(t, "/v1/web/push/test", `{}`, p.first)
	if !strings.Contains(body, `"sent":false`) || !strings.Contains(body, `"failure":"`) {
		t.Fatalf("failed test push body %q, want sent false and a failure", body)
	}
	if p.hits.Load() != 2 {
		t.Errorf("%d hits, want the failing test push to make one attempt", p.hits.Load()-1)
	}
	got, _ := p.state(t, p.first)
	if got.LastFailure == nil || got.LastFailure.Reason == "" {
		t.Errorf("state last_failure %+v, want the failure reason", got.LastFailure)
	}
	if other, _ := p.state(t, p.second); other.LastFailure != nil {
		t.Error("another login shows this login's failure")
	}
}

// The server switch stops sends and the test route; a service worker's
// subscribe and renew keep working so the devices are never lost.
func TestPushRoutes_SwitchOffRefusesTestAndKeepsSubscriptions(t *testing.T) {
	t.Parallel()
	on := startPushWeb(t, testEmail)
	if st, _ := on.state(t, on.first); !st.ServerOn {
		t.Fatalf("state %+v, want server_on while the switch is on", st)
	}
	p := startPushWeb(t, testEmail, servertest.WithChannel("push", false, nil))
	st, _ := p.state(t, p.first)
	if st.ServerOn || !st.ContactSet {
		t.Fatalf("state %+v, want server_on false and the contact still set", st)
	}
	if r := p.reason(t, "/v1/web/push/subscribe", subBody(t, st.PublicKey, p.svcURL+"/push/x", ""), p.first); r != "" {
		t.Fatalf("subscribe refused while off: %q", r)
	}
	if r := p.reason(t, "/v1/web/push/renew", subBody(t, st.PublicKey, p.svcURL+"/push/y", fmt.Sprintf(`,"old_endpoint":%q`, p.svcURL+"/push/x")), p.first); r != "" {
		t.Fatalf("renew refused while off: %q", r)
	}
	if r := p.reason(t, "/v1/web/push/test", `{}`, p.first); r != wire.ReasonPushSwitchOff {
		t.Fatalf("test while off: reason %q, want %q", r, wire.ReasonPushSwitchOff)
	}
	if p.hits.Load() != 0 {
		t.Errorf("%d hits, want the refused test to send nothing", p.hits.Load())
	}
	if got, _ := p.state(t, p.first); !got.Subscribed {
		t.Error("the switch deleted the subscription")
	}
}

func TestPushRoutes_StateNeverHoldsTheEmail(t *testing.T) {
	t.Parallel()
	p := startPushWeb(t, testEmail)
	st, state := p.state(t, p.first)
	bodies := []string{state}
	for _, req := range []struct{ path, body string }{
		{"/v1/web/push/subscribe", subBody(t, st.PublicKey, p.svcURL+"/push/x", "")},
		{"/v1/web/push/test", `{}`},
		{"/v1/web/push/renew", subBody(t, st.PublicKey, "http://x", "")},
		{"/v1/web/push/remove", `{}`},
	} {
		_, out := p.post(t, req.path, req.body, p.first)
		bodies = append(bodies, out)
	}
	p.status.Store(http.StatusForbidden)
	_ = p.reason(t, "/v1/web/push/subscribe", subBody(t, st.PublicKey, p.svcURL+"/push/x", ""), p.first)
	_, out := p.post(t, "/v1/web/push/test", `{}`, p.first)
	_, state = p.state(t, p.first)
	bodies = append(bodies, out, state)
	for _, b := range bodies {
		if strings.Contains(b, testEmail) || strings.Contains(b, "example.com") {
			t.Errorf("a response holds the email: %q", b)
		}
	}
	if code, _ := servertest.WebDoCookie(t, http.MethodGet, p.base+"/v1/web/push/state/extra", p.first); code != http.StatusNotFound {
		t.Errorf("state/extra: status %d, want 404", code)
	}
}
