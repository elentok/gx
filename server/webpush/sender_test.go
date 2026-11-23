package webpush

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/json"
	"io"
	"log/slog"
	"maps"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	unreachable = "gx can't reach the push service. Check the server's outbound HTTPS"
	contact     = "me@example.com"
)

type received struct {
	header http.Header
	body   []byte
}

type fixture struct {
	t      *testing.T
	sender *Sender
	sub    Subscription
	uaKey  *ecdh.PrivateKey
	auth   []byte

	mu     sync.Mutex
	got    []received
	status []int // status per attempt; the last one repeats
	extra  http.Header
	slept  []time.Duration
	quiet  func() bool
	checks int
}

func newFixture(t *testing.T, status ...int) *fixture {
	t.Helper()
	f := &fixture{t: t, status: status}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		n := len(f.got)
		f.got = append(f.got, received{header: r.Header.Clone(), body: body})
		code := f.status[min(n, len(f.status)-1)]
		maps.Copy(w.Header(), f.extra)
		f.mu.Unlock()
		w.WriteHeader(code)
	}))
	t.Cleanup(srv.Close)
	keys, err := LoadOrCreate(t.TempDir(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	f.uaKey, err = ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	f.auth = make([]byte, 16)
	_, _ = rand.Read(f.auth)
	f.sub = Subscription{Endpoint: srv.URL + "/push/secret-token", P256dh: b64.EncodeToString(f.uaKey.PublicKey().Bytes()), Auth: b64.EncodeToString(f.auth)}
	f.sender = &Sender{
		Client:  srv.Client(),
		Keys:    keys,
		Contact: func() string { return contact },
		Quiet: func() bool {
			f.mu.Lock()
			defer f.mu.Unlock()
			f.checks++
			return f.quiet != nil && f.quiet()
		},
		Sleep: func(_ context.Context, d time.Duration) error {
			f.mu.Lock()
			f.slept = append(f.slept, d)
			f.mu.Unlock()
			return nil
		},
	}
	return f
}

func (f *fixture) send(msg Message, opts Options) Outcome {
	return f.sender.Send(context.Background(), f.sub, msg, opts)
}

func (f *fixture) payload(r received) map[string]any {
	f.t.Helper()
	var m map[string]any
	if err := json.Unmarshal(decrypt(f.t, r.body, f.uaKey, f.auth), &m); err != nil {
		f.t.Fatal(err)
	}
	return m
}

func TestSender_RequestHeaders(t *testing.T) {
	f := newFixture(t, 201)
	out := f.send(Message{Title: "t", Tag: "agent:a/b", Path: "/p"}, Options{Urgency: "high"})
	if !out.Delivered || out.Reason != "" {
		t.Fatalf("outcome %+v", out)
	}
	h := f.got[0].header
	for k, want := range map[string]string{"Content-Encoding": "aes128gcm", "Content-Type": "application/octet-stream", "Ttl": "3600", "Urgency": "high"} {
		if h.Get(k) != want {
			t.Errorf("%s = %q, want %q", k, h.Get(k), want)
		}
	}
	topic := h.Get("Topic")
	if !regexp.MustCompile(`^[A-Za-z0-9_-]{1,32}$`).MatchString(topic) {
		t.Errorf("Topic %q is not 1-32 URL-safe characters", topic)
	}
	f.send(Message{Title: "other", Tag: "agent:a/b"}, Options{Urgency: "normal"})
	if f.got[1].header.Get("Topic") != topic {
		t.Error("same tag gave a different Topic")
	}
	f.send(Message{Title: "other", Tag: "agent:a/c"}, Options{Urgency: "normal"})
	if f.got[2].header.Get("Topic") == topic {
		t.Error("different tags gave the same Topic")
	}
	auth := h.Get("Authorization")
	m := regexp.MustCompile(`^vapid t=([^.]+\.([^.]+)\.[^,]+), k=(\S+)$`).FindStringSubmatch(auth)
	if m == nil {
		t.Fatalf("Authorization %q", auth)
	}
	if m[3] != f.sender.Keys.Public() {
		t.Error("k is not the VAPID public key")
	}
	claims := map[string]any{}
	if err := json.Unmarshal(mustB64(t, m[2]), &claims); err != nil {
		t.Fatal(err)
	}
	if claims["sub"] != "mailto:"+contact {
		t.Errorf("sub = %v", claims["sub"])
	}
	u, _ := url.Parse(f.sub.Endpoint)
	if aud := u.Scheme + "://" + u.Host; claims["aud"] != aud {
		t.Errorf("aud = %v, want %s", claims["aud"], aud)
	}
}

func TestSender_RequestTestPushTTL(t *testing.T) {
	f := newFixture(t, 201)
	f.send(Message{Title: "t"}, Options{Urgency: "normal", Test: true})
	if got := f.got[0].header.Get("TTL"); got != "60" {
		t.Errorf("TTL = %q, want 60", got)
	}
}

func TestSender_PayloadFields(t *testing.T) {
	f := newFixture(t, 201)
	f.send(Message{Title: "T", Body: "B", Path: "/agents/x", Tag: "tag:1", Badge: 3}, Options{Urgency: "normal"})
	p := f.payload(f.got[0])
	want := map[string]any{"title": "T", "body": "B", "path": "/agents/x", "tag": "tag:1", "badge": float64(3)}
	if len(p) != len(want) {
		t.Errorf("payload has keys %v", p)
	}
	for k, v := range want {
		if p[k] != v {
			t.Errorf("%s = %v, want %v", k, p[k], v)
		}
	}
}

func TestSender_PayloadBodyCutOnRuneBoundary(t *testing.T) {
	f := newFixture(t, 201)
	f.send(Message{Body: strings.Repeat("é", 300)}, Options{Urgency: "normal"})
	if got := f.payload(f.got[0])["body"].(string); got != strings.Repeat("é", 200) {
		t.Errorf("body has %d characters, want 200 whole runes", len([]rune(got)))
	}
}

func TestSender_PayloadBodyCutTo100WhenTooBig(t *testing.T) {
	f := newFixture(t, 201)
	// A long title makes 200 body characters overflow the record; 100 fit.
	title := strings.Repeat("x", maxPlaintext-200)
	f.send(Message{Title: title, Body: strings.Repeat("b", 300)}, Options{Urgency: "normal"})
	if got := f.payload(f.got[0])["body"].(string); got != strings.Repeat("b", 100) {
		t.Errorf("body has %d characters, want 100", len(got))
	}
}

func TestSender_RetryNetworkError(t *testing.T) {
	f := newFixture(t, 201)
	f.sub.Endpoint = "http://" + closedAddr(t) + "/x"
	out := f.send(Message{}, Options{Urgency: "normal"})
	if out.Delivered || out.Reason != unreachable {
		t.Fatalf("outcome %+v", out)
	}
	if len(f.slept) != 2 || f.slept[0] != time.Second || f.slept[1] != 5*time.Second {
		t.Errorf("waits %v, want 1s then 5s", f.slept)
	}
	if f.checks != 3 {
		t.Errorf("%d tries, want 3", f.checks)
	}
}

func TestSender_Retry5xxAnd429(t *testing.T) {
	for _, code := range []int{500, 503, 429} {
		t.Run(strconv.Itoa(code), func(t *testing.T) {
			f := newFixture(t, code)
			out := f.send(Message{}, Options{Urgency: "normal"})
			if out.Delivered || out.Gone || out.Reason != "The push service answered "+strconv.Itoa(code)+"." {
				t.Fatalf("outcome %+v", out)
			}
			if len(f.got) != 3 || len(f.slept) != 2 || f.slept[0] != time.Second || f.slept[1] != 5*time.Second {
				t.Errorf("%d requests, waits %v", len(f.got), f.slept)
			}
		})
	}
}

func TestSender_RetrySucceedsOnSecondTry(t *testing.T) {
	f := newFixture(t, 503, 201)
	if out := f.send(Message{}, Options{Urgency: "normal"}); !out.Delivered {
		t.Fatalf("outcome %+v", out)
	}
	if len(f.got) != 2 {
		t.Errorf("%d requests, want 2", len(f.got))
	}
}

func TestSender_RetryAfter(t *testing.T) {
	for _, tc := range []struct {
		header string
		want   []time.Duration
	}{
		{"30", []time.Duration{30 * time.Second, 30 * time.Second}},
		{"2", []time.Duration{2 * time.Second, 2 * time.Second}},
		{"31", []time.Duration{time.Second, 5 * time.Second}},
		{"soon", []time.Duration{time.Second, 5 * time.Second}},
		{"-1", []time.Duration{time.Second, 5 * time.Second}},
	} {
		t.Run(tc.header, func(t *testing.T) {
			f := newFixture(t, 429)
			f.extra = http.Header{"Retry-After": {tc.header}}
			f.send(Message{}, Options{Urgency: "normal"})
			if !slices.Equal(f.slept, tc.want) {
				t.Errorf("waits %v, want %v", f.slept, tc.want)
			}
		})
	}
}

func TestSender_StatusGone(t *testing.T) {
	for _, code := range []int{404, 410} {
		f := newFixture(t, code)
		out := f.send(Message{}, Options{Urgency: "normal"})
		if !out.Gone || out.Delivered || len(f.got) != 1 {
			t.Errorf("%d: outcome %+v after %d requests", code, out, len(f.got))
		}
	}
}

func TestSender_StatusForbiddenKeepsSubscription(t *testing.T) {
	f := newFixture(t, 403)
	out := f.send(Message{}, Options{Urgency: "normal"})
	if out.Gone || out.Delivered || out.Reason != "The push service answered 403." || len(f.got) != 1 {
		t.Errorf("outcome %+v after %d requests", out, len(f.got))
	}
}

func TestSender_StatusOther4xxNotRetried(t *testing.T) {
	f := newFixture(t, 413)
	out := f.send(Message{}, Options{Urgency: "normal"})
	if out.Gone || out.Reason != "The push service answered 413." || len(f.got) != 1 {
		t.Errorf("outcome %+v after %d requests", out, len(f.got))
	}
}

func TestSender_ReasonLeaksNothing(t *testing.T) {
	f := newFixture(t, 500)
	out := f.send(Message{}, Options{Urgency: "normal"})
	for _, secret := range []string{f.sub.Endpoint, "secret-token", contact, f.sender.Keys.Public()} {
		if strings.Contains(out.Reason, secret) {
			t.Errorf("reason %q holds %q", out.Reason, secret)
		}
	}
}

func TestSender_TestPushOneAttemptNoQuietCheck(t *testing.T) {
	f := newFixture(t, 503)
	f.quiet = func() bool { return true }
	out := f.send(Message{}, Options{Urgency: "normal", Test: true})
	if out.Dropped || out.Reason == "" || len(f.got) != 1 || len(f.slept) != 0 || f.checks != 0 {
		t.Errorf("outcome %+v, %d requests, %d waits, %d quiet checks", out, len(f.got), len(f.slept), f.checks)
	}
}

func TestSender_QuietDropsBeforeFirstAttempt(t *testing.T) {
	f := newFixture(t, 201)
	f.quiet = func() bool { return true }
	out := f.send(Message{}, Options{Urgency: "normal"})
	if !out.Dropped || out.Delivered || out.Reason != "" || len(f.got) != 0 {
		t.Errorf("outcome %+v after %d requests", out, len(f.got))
	}
}

func TestSender_QuietCheckedBeforeEveryRetry(t *testing.T) {
	f := newFixture(t, 503)
	f.quiet = func() bool { return len(f.got) >= 1 } // the Rest period starts after try 1
	out := f.send(Message{}, Options{Urgency: "normal"})
	if !out.Dropped || out.Reason != "" || len(f.got) != 1 || f.checks != 2 {
		t.Errorf("outcome %+v, %d requests, %d quiet checks", out, len(f.got), f.checks)
	}
}

func TestSender_NoValidContactFailsBeforeNetwork(t *testing.T) {
	f := newFixture(t, 201)
	for _, c := range []string{"", "not an email"} {
		f.sender.Contact = func() string { return c }
		out := f.send(Message{}, Options{Urgency: "normal"})
		if out.Delivered || out.Reason == "" || len(f.got) != 0 {
			t.Errorf("contact %q: outcome %+v after %d requests", c, out, len(f.got))
		}
	}
}

func TestSender_ContextCancelStopsRetries(t *testing.T) {
	f := newFixture(t, 503)
	ctx, cancel := context.WithCancel(context.Background())
	f.sender.Sleep = func(context.Context, time.Duration) error { cancel(); return ctx.Err() }
	out := f.sender.Send(ctx, f.sub, Message{}, Options{Urgency: "normal"})
	if out.Delivered || out.Reason == "" || len(f.got) != 1 {
		t.Errorf("outcome %+v after %d requests", out, len(f.got))
	}
}

// closedAddr returns a loopback address nothing listens on.
func closedAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	_ = l.Close()
	return addr
}
