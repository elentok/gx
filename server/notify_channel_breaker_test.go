package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/elentok/gx/notifykind"
	"github.com/elentok/gx/ralphloop/notify"
	"github.com/elentok/gx/server/settings"
	"github.com/elentok/gx/server/webauth"
)

// threeDevices is one login's subscription repeated for three devices.
type threeDevices struct{ oneTarget }

func (d threeDevices) PushTargets() []webauth.PushTarget {
	var out []webauth.PushTarget
	for _, id := range []string{"a", "b", "c"} {
		t := d.PushTarget
		t.LoginID = id
		out = append(out, t)
	}
	return out
}

// breakerRig is a push server whose banners are recorded, with Desktop and
// Push on a fixed kind list, a Slack webhook and the state file at gate.
type breakerRig struct {
	s       *Server
	banners *recNotifier
	hits    *atomic.Int32
	posts   *atomic.Int32
}

func newBreakerRig(t *testing.T, now func() time.Time, tweak ...func(*Config)) breakerRig {
	t.Helper()
	r := breakerRig{banners: &recNotifier{}, posts: new(atomic.Int32)}
	hook := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { r.posts.Add(1) }))
	t.Cleanup(hook.Close)
	kinds := []string{notifykind.BudgetAndUsage}
	r.s, r.hits, _ = pushServer(t, now, append([]func(*Config){func(c *Config) {
		c.Notifier = r.banners
		c.Player = &recPlayer{}
		c.Settings.NotifyChannels = settings.NotifyChannels{
			Desktop: settings.NotifyChannel{Enabled: true, Kinds: kinds}, Push: settings.NotifyChannel{Enabled: true, Kinds: kinds}, WebToast: settings.NotifyChannel{Enabled: true, Kinds: kinds}, Slack: defaultOn()}
		c.Settings.Chat = notify.ServerChatConfig{SlackWebhookURL: hook.URL, FlushInterval: 20 * time.Millisecond}
	}}, tweak...)...)
	t.Cleanup(func() { r.s.chat.Close() })
	return r
}

func (r breakerRig) desktop(i int) {
	r.s.chat.Desktop(notify.DesktopNote{Kind: notifykind.BudgetAndUsage, Title: fmt.Sprint("banner ", i)})
}

func (r breakerRig) series(t *testing.T, channel string) notify.TransportState {
	t.Helper()
	st, err := notify.LoadNotificationState(r.s.cfg.Chat.GateStatePath)
	if err != nil {
		t.Fatal(err)
	}
	return st.Transports[channel]
}

func eventually(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		<-time.After(5 * time.Millisecond)
	}
}

func TestChannelBreaker_DesktopFloodLeavesPushAndSlackSending(t *testing.T) {
	t.Parallel()
	r := newBreakerRig(t, time.Now)
	for i := range 25 {
		r.desktop(i)
	}
	// 20 banners, then the trip notice, then nothing more.
	if got := r.banners.n.Load(); got != 21 {
		t.Fatalf("banners = %d, want 20 plus one notice", got)
	}
	if ts := r.series(t, "desktop"); !ts.Muted || len(ts.Trips) != 1 {
		t.Fatalf("desktop state = %+v", ts)
	}

	r.s.chat.Push(notify.PushNote{Kind: notifykind.BudgetAndUsage, Title: "push"})
	eventually(t, "a push", func() bool { return r.hits.Load() == 1 })
	r.s.chat.SessionAlert("p", nil, "name", "done", "body", true)
	eventually(t, "a Slack post", func() bool { return r.posts.Load() == 1 })
	if ts := r.series(t, "push"); ts.Muted || len(ts.Sends) != 1 {
		t.Fatalf("push state = %+v", ts)
	}
}

func TestChannelBreaker_OnePushToThreeDevicesIsOneSend(t *testing.T) {
	t.Parallel()
	r := newBreakerRig(t, time.Now)
	r.s.push.Subs = threeDevices{r.s.push.Subs.(oneTarget)}
	r.s.chat.Push(notify.PushNote{Kind: notifykind.BudgetAndUsage, Title: "push"})
	eventually(t, "three pushes", func() bool { return r.hits.Load() == 3 })
	if got := len(r.series(t, "push").Sends); got != 1 {
		t.Fatalf("push sends = %d, want 1", got)
	}

	// A toast to two open pages is one send as well.
	for range 2 {
		_, cancel, err := r.s.events.Subscribe(r.s.events.CurrentSeq())
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(cancel)
	}
	r.s.chat.WebToast(notify.WebToastNote{Kind: notifykind.BudgetAndUsage, Title: "toast"})
	if got := len(r.series(t, "web-toast").Sends); got != 1 {
		t.Fatalf("web toast sends = %d, want 1", got)
	}
}

func TestChannelBreaker_RestPeriodAndUncheckedKindAddNoAttempt(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 1, 5, 23, 0, 0, 0, time.Local)
	r := newBreakerRig(t, func() time.Time { return now })
	if _, err := r.s.rest.Tick(); err != nil {
		t.Fatal(err)
	}
	if !r.s.rest.Quiet() {
		t.Fatal("bedtime should be quiet")
	}
	r.desktop(0)
	r.s.chat.Desktop(notify.DesktopNote{Kind: notifykind.Disk, Title: "unchecked"})
	r.s.chat.WebToast(notify.WebToastNote{Kind: notifykind.Disk, Title: "unchecked"})
	r.s.chat.Push(notify.PushNote{Kind: notifykind.BudgetAndUsage, Title: "push"})
	<-time.After(100 * time.Millisecond) // longer than the merge hold: a push would have been counted by now
	for _, ch := range []string{"desktop", "web-toast", "push"} {
		if ts := r.series(t, ch); len(ts.Sends) != 0 {
			t.Errorf("%s recorded %d attempts, want none", ch, len(ts.Sends))
		}
	}
}

func TestChannelBreaker_ClearsWithTheShortKnob(t *testing.T) {
	t.Parallel()
	const clear = 300 * time.Millisecond
	r := newBreakerRig(t, time.Now, func(c *Config) { c.ChannelBreakerClear = clear })
	for i := range 22 {
		r.desktop(i)
	}
	if got := r.banners.n.Load(); got != 21 {
		t.Fatalf("banners = %d, want 20 plus one notice", got)
	}
	// Attempts while tripped keep it tripped past one clear window.
	for i := range 4 {
		<-time.After(clear * 2 / 3)
		r.desktop(100 + i)
	}
	if got := r.banners.n.Load(); got != 21 {
		t.Fatalf("banners = %d while tripped, want 21", got)
	}
	<-time.After(clear + 50*time.Millisecond)
	r.desktop(200)
	if got := r.banners.n.Load(); got != 22 {
		t.Fatalf("banners = %d after the quiet window, want 22", got)
	}
}

func TestChannelBreaker_NoticeShowsTheConfiguredClearTime(t *testing.T) {
	t.Parallel()
	r := newBreakerRig(t, time.Now, func(c *Config) { c.ChannelBreakerClear = 90 * time.Second })
	if got := breakerNotice(r.s, "Desktop"); !strings.Contains(got, "1m30s") || strings.Contains(got, "5 minutes") {
		t.Fatalf("notice = %q, want the configured 1m30s", got)
	}
	d := newBreakerRig(t, time.Now)
	if got := breakerNotice(d.s, "Desktop"); !strings.Contains(got, "5m0s") {
		t.Fatalf("default notice = %q, want 5m0s", got)
	}
}
