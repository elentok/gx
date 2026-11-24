package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/config/registry"
	"github.com/elentok/gx/notifykind"
	"github.com/elentok/gx/osnotify"
	"github.com/elentok/gx/ralphloop/notify"
	"github.com/elentok/gx/server/settings"
	"github.com/elentok/gx/server/settingsapi"
	"github.com/elentok/gx/server/webauth"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/sound"
	"github.com/elentok/gx/subscription"
	"github.com/elentok/gx/testutil/runnerfake"
)

// Part A: a catalog row that one of the four places forgot fails with the id
// and the place.

// noUpgradeList names the catalog ids that sit on no channel's upgrade list on
// purpose, with the reason.
var noUpgradeList = map[string]string{
	notifykind.LongTicket: "new in this release; the spec's \"Lists written by an upgrade\" leaves it off every list",
}

func TestNotificationGuard_EveryCatalogIDIsInRegistryMigrationSnapshotAndRouter(t *testing.T) {
	t.Parallel()
	ids := notifykind.IDs()
	if len(ids) != 16 {
		t.Fatalf("catalog has %d ids, want 16", len(ids))
	}
	channels := notifykind.Channels()
	reg := registry.Default()
	migrated := migratedLists(t)
	snapshot := snapshotKindIDs(t)
	rec := &routeRecorder{}
	chat := notify.NewServerChat(notify.ServerChatConfig{Desktop: desktopRecorder{rec}, Push: pushRecorder{rec}, WebToast: webToastRecorder{rec}})
	for _, id := range ids {
		var missing []string
		for _, ch := range channels {
			e, ok := reg.Lookup(registry.Key("notification-channels", ch.ID, "kinds"))
			if !ok || !slices.Contains(e.Enum, id) {
				missing = append(missing, "config registry kinds entry of "+ch.ID)
			}
		}
		_, optedOut := noUpgradeList[id]
		if !optedOut && !onAnyList(migrated, id) {
			missing = append(missing, "migration table (config/migrate_notification_kinds.go)")
		}
		if optedOut && onAnyList(migrated, id) {
			missing = append(missing, "noUpgradeList says no list, but the migration table has it")
		}
		if !slices.Contains(snapshot, id) {
			missing = append(missing, "settings snapshot (settingsapi.Read)")
		}
		if routed := routeKind(chat, rec, id); routed != 3 {
			missing = append(missing, "router (reached "+string(rune('0'+routed))+" of 3 adapter channels)")
		}
		if len(missing) > 0 {
			t.Errorf("catalog id %q is missing from: %s", id, strings.Join(missing, "; "))
		}
	}
}

// migratedLists runs the upgrade on an empty config and returns each channel's
// kind list.
func migratedLists(t *testing.T) map[string][]string {
	t.Helper()
	out, changed := config.MigrateNotificationKinds([]byte(`{}`))
	if !changed {
		t.Fatal("the migration changed nothing on an empty config")
	}
	var cfg struct {
		Channels map[string]struct {
			Kinds []string `json:"kinds"`
		} `json:"notification-channels"`
	}
	if err := json.Unmarshal(out, &cfg); err != nil {
		t.Fatal(err)
	}
	lists := map[string][]string{}
	for _, ch := range notifykind.Channels() {
		c, ok := cfg.Channels[ch.ID]
		if !ok {
			t.Fatalf("migration table has no entry for channel %s", ch.ID)
		}
		lists[ch.ID] = c.Kinds
	}
	return lists
}

func onAnyList(lists map[string][]string, id string) bool {
	for _, l := range lists {
		if slices.Contains(l, id) {
			return true
		}
	}
	return false
}

// snapshotKindIDs is the catalog a global settings read carries.
func snapshotKindIDs(t *testing.T) []string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	w := &settingsapi.Writer{Registry: registry.Default(), GlobalPath: path}
	resp, err := w.Read(settingsapi.Target{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, k := range resp.NotificationKinds {
		ids = append(ids, k.ID)
	}
	return ids
}

// routeRecorder logs the kinds each adapter channel receives.
type routeRecorder struct{ kinds []string }

type (
	desktopRecorder  struct{ r *routeRecorder }
	pushRecorder     struct{ r *routeRecorder }
	webToastRecorder struct{ r *routeRecorder }
)

func (d desktopRecorder) Show(n notify.DesktopNote) { d.r.kinds = append(d.r.kinds, "desktop:"+n.Kind) }
func (p pushRecorder) Send(n notify.PushNote)       { p.r.kinds = append(p.r.kinds, "push:"+n.Kind) }
func (w webToastRecorder) Show(n notify.WebToastNote) {
	w.r.kinds = append(w.r.kinds, "web-toast:"+n.Kind)
}

// routeKind hands a note of kind id to the router and counts the adapter
// channels that received it.
func routeKind(chat *notify.ServerChat, rec *routeRecorder, id string) int {
	rec.kinds = nil
	chat.Desktop(notify.DesktopNote{Kind: id})
	chat.Push(notify.PushNote{Kind: id})
	chat.WebToast(notify.WebToastNote{Kind: id})
	n := 0
	for _, k := range rec.kinds {
		if strings.HasSuffix(k, ":"+id) {
			n++
		}
	}
	return n
}

// Part B: no call site calls a transport directly.

// transportCall matches a direct call of a transport's send: the OS notifier's
// Notify, the sound player's Play, the web push dispatcher's Push and PushNow,
// the sender's Send, the chat transports' sendSync and a Web toast publish.
var transportCall = regexp.MustCompile(
	`\.Notify\([^,]+, (note|osnotify\.Notification\{)` +
		`|\.Play\((ctx|context\.)` +
		`|\.PushNow\(|\bd\.Push\(` +
		`|\.Send\((d\.)?ctx,` +
		`|\.sendSync\(` +
		`|Type: wire\.EventNotificationToast`)

// transportCalls returns the non-comment lines of src that call a transport.
func transportCalls(src string) []string {
	var hits []string
	for line := range strings.SplitSeq(src, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "//") && transportCall.MatchString(line) {
			hits = append(hits, trimmed)
		}
	}
	return hits
}

func TestNotificationGuard_ScannerReportsADirectCallAndSkipsComments(t *testing.T) {
	t.Parallel()
	src := "// n.Notify(ctx, note) is the doc example\n" +
		"func f() {\n" +
		"\terr := n.Notifier.Notify(ctx, note)\n" +
		"\tplayer.Play(ctx, cfg, name)\n" +
		"\tp.d.PushNow(ev)\n" +
		"\tout := d.Send(d.ctx, sub, msg, opts)\n" +
		"\tt.sendSync(ctx, text)\n" +
		"\ts.events.PublishEvent(wire.Event{Type: wire.EventNotificationToast})\n" +
		"\tc.Notify(\"text\")\n" +
		"}\n"
	got := transportCalls(src)
	if len(got) != 6 {
		t.Fatalf("scanner found %d direct calls, want 6: %q", len(got), got)
	}
	if hits := transportCalls("\t// p.d.PushNow(ev)\n"); len(hits) != 0 {
		t.Fatalf("scanner reported a comment line: %q", hits)
	}
}

func TestNotificationGuard_NoCallSiteCallsATransportDirectly(t *testing.T) {
	t.Parallel()
	root := ".."
	// file -> why it may call a transport. The server's own wiring files are the
	// adapters and the construction seam; the CLI fallback is the one exception.
	allowed := map[string]string{
		"server/server.go":                       "the construction seam",
		"server/desktop.go":                      "the Desktop and Web toast adapters",
		"server/sessionwire.go":                  "the Push adapter's hand-off to the dispatcher",
		"server/quiet.go":                        "the decorators that apply the Desktop switch and the Rest period to the notifier and player",
		"server/webpush/dispatcher.go":           "the Push transport itself: the dispatcher calls its sender",
		"ralphloop/notify/telegram_transport.go": "the Telegram transport and its CLI test-send helper",
		"ralphloop/notify/slack_transport.go":    "the Slack transport and its CLI test-send helper",
		"ralphloop/notify/chat_eventsink.go":     "the Telegram and Slack adapter: the sink sends through its transport after the router's gates",
		"ralphloop/notify/notify.go":             "the one exception: the CLI's direct fallback (SendMessage, SendTestBatch, gateCLISend)",
		"server/settings/holder.go":              "config-reload banner, a system message; the server wraps its notifier in the Desktop switch",
		"server/health/stormwatch.go":            "process-storm banner, a system message; the server wraps its notifier in the Desktop switch",
		"server/notification_guard_test.go":      "this guard",
	}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", "webui", ".scratch":
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		hits := transportCalls(string(src))
		if reason, ok := allowed[rel]; ok {
			if reason == "" {
				t.Errorf("%s is allowed without a reason", rel)
			}
			return nil
		}
		for _, h := range hits {
			t.Errorf("%s calls a transport directly, hand the router a notification instead: %s", rel, h)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

type bannerLog struct {
	mu   sync.Mutex
	sent []osnotify.Notification
}

func (l *bannerLog) Notify(_ context.Context, n osnotify.Notification) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.sent = append(l.sent, n)
	return nil
}

func (l *bannerLog) all() []osnotify.Notification {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]osnotify.Notification(nil), l.sent...)
}

// desktopServer is a server whose banners and sounds are recorded and whose
// Desktop channel is ch.
// defaultOn is a channel as a new install has it: on, with the default kinds.
func defaultOn() settings.NotifyChannel {
	return settings.NotifyChannel{Enabled: true, Kinds: notifykind.DefaultIDs()}
}

func allOn() settings.NotifyChannels {
	return settings.NotifyChannels{Desktop: defaultOn(), WebToast: defaultOn(), Telegram: defaultOn(), Slack: defaultOn(), Push: defaultOn()}
}

func desktopServer(t *testing.T, ch settings.NotifyChannel) (*Server, *bannerLog, *recPlayer) {
	t.Helper()
	stateDir, err := os.MkdirTemp("", "gx") // short path: unix socket limit
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(stateDir) })
	banners, player := &bannerLog{}, &recPlayer{}
	sounds := config.AgentsSoundsConfig{Enabled: true}
	s, err := New(Config{Runner: runnerfake.NewRunner(), StateDir: stateDir, TicketStore: t.TempDir(), Notifier: banners, Player: player,
		Settings:        Settings{BudgetSoftLimit: 10, BudgetHardLimit: 20, AgentsBanners: true, AgentsSounds: sounds, NotifyChannels: settings.NotifyChannels{Desktop: ch}},
		ExtraUsageCheck: func() subscription.State { return subscription.StateDisabled }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.chat.Close() })
	return s, banners, player
}

func overSoftLimit(s *Server) {
	now := time.Date(2026, 10, 6, 12, 0, 0, 0, time.Local)
	s.budget.SeedNotes(now)
	s.budget.RecordSpendAt("a", 11, now)
	s.budget.NotifyAt(now)
}

func TestDesktop_ACheckedNonSessionKindIsABannerWithNoSound(t *testing.T) {
	t.Parallel()
	s, banners, player := desktopServer(t, settings.NotifyChannel{Enabled: true, Kinds: []string{notifykind.BudgetAndUsage}})
	overSoftLimit(s)
	got := banners.all()
	if len(got) != 1 || !strings.Contains(got[0].Title, "Budget soft limit reached") {
		t.Fatalf("banners = %+v, want one with the chat headline as title", got)
	}
	if player.n.Load() != 0 {
		t.Errorf("sounds = %d, want none for a non-session kind", player.n.Load())
	}
}

func TestDesktop_AnUncheckedKindIsNoBannerAndNoSound(t *testing.T) {
	t.Parallel()
	s, banners, player := desktopServer(t, settings.NotifyChannel{Enabled: true, Kinds: []string{notifykind.SessionAlert}})
	overSoftLimit(s)
	if got := banners.all(); len(got) != 0 || player.n.Load() != 0 {
		t.Errorf("banners = %+v, sounds = %d, want none", got, player.n.Load())
	}
}

func TestDesktop_SwitchOffSendsNothing(t *testing.T) {
	t.Parallel()
	s, banners, player := desktopServer(t, settings.NotifyChannel{})
	overSoftLimit(s)
	(&desktopAdapter{s: s, webURL: func() string { return "" }}).Show(notify.DesktopNote{Kind: notifykind.SessionAlert, Session: true, Sound: sound.Done})
	// The decorators drop the system banners and sounds that bypass the adapter.
	_ = s.cfg.Notifier.Notify(context.Background(), osnotify.Notification{Title: "storm"})
	s.cfg.Player.Play(context.Background(), config.AgentsSoundsConfig{Enabled: true}, sound.Done)
	if got := banners.all(); len(got) != 0 || player.n.Load() != 0 {
		t.Errorf("banners = %+v, sounds = %d, want none", got, player.n.Load())
	}
}

func TestDesktop_OnlySessionKindsPlayASoundAndEveryTargetCarriesItsLink(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		note     notify.DesktopNote
		wantLink string
		sounds   int
	}{
		{"ticket", notify.DesktopNote{Kind: notifykind.PausedTicket, Target: notify.Target{Ticket: "p:e/01"}}, "http://web/#/t/p:e/01", 0},
		{"epic", notify.DesktopNote{Kind: notifykind.PausedTicket, Target: notify.Target{Epic: "p:e"}}, "http://web/#/e/p:e", 0},
		{"design", notify.DesktopNote{Kind: notifykind.PausedTicket, Target: notify.Target{Design: "p:m"}}, "http://web/#/d/p:m", 0},
		{"session", notify.DesktopNote{Kind: notifykind.PausedTicket, Target: notify.Target{Session: "abc"}}, "http://web/#/agents/abc", 0},
		{"none", notify.DesktopNote{Kind: notifykind.Disk}, "http://web/#/", 0},
		{"session banner keeps its own link", notify.DesktopNote{Kind: notifykind.SessionAlert, Session: true, Link: "L", Sound: sound.Done}, "L", 1},
		{"non-session kind never plays", notify.DesktopNote{Kind: notifykind.Disk, Sound: sound.Done}, "http://web/#/", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, banners, player := desktopServer(t, defaultOn())
			a := &desktopAdapter{s: s, webURL: func() string { return "http://web" }}
			a.Show(tc.note)
			got := banners.all()
			if len(got) != 1 || got[0].Link != tc.wantLink {
				t.Errorf("banners = %+v, want one with Link %q", got, tc.wantLink)
			}
			if int(player.n.Load()) != tc.sounds {
				t.Errorf("sounds = %d, want %d", player.n.Load(), tc.sounds)
			}
		})
	}
}

func TestDesktop_NoWebListenerMeansNoLink(t *testing.T) {
	t.Parallel()
	s, banners, _ := desktopServer(t, defaultOn())
	(&desktopAdapter{s: s, webURL: func() string { return "" }}).Show(notify.DesktopNote{Kind: notifykind.Disk, Target: notify.Target{Epic: "p:e"}})
	if got := banners.all(); len(got) != 1 || got[0].Link != "" {
		t.Errorf("banners = %+v, want one with no Link", got)
	}
}

func webToastServer(t *testing.T, ch settings.NotifyChannel) *Server {
	t.Helper()
	stateDir, err := os.MkdirTemp("", "gx") // short path: unix socket limit
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(stateDir) })
	s, err := New(Config{Runner: runnerfake.NewRunner(), StateDir: stateDir, TicketStore: t.TempDir(),
		Settings: Settings{NotifyChannels: settings.NotifyChannels{WebToast: ch}}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.chat.Close() })
	return s
}

func sendServerStarted(s *Server) {
	s.chat.Notice(notify.ServerNotice{Kind: NoticeServerStarted, Emoji: "🚀", Title: "server started", Detail: "Build x"})
}

func TestWebToast_CheckedKindReachesEveryOpenPageAndNoLaterPage(t *testing.T) {
	t.Parallel()
	s := webToastServer(t, settings.NotifyChannel{Enabled: true, Kinds: []string{notifykind.ServerStarted}})
	var chans []<-chan wire.Event
	for range 2 {
		ch, cancel, err := s.events.Subscribe(s.events.CurrentSeq())
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(cancel)
		chans = append(chans, ch)
	}
	sendServerStarted(s)
	for i, ch := range chans {
		var ev wire.Event
		select {
		case ev = <-ch:
		default:
			t.Fatalf("subscriber %d got no event", i)
		}
		if ev.Type != wire.EventNotificationToast || ev.Title == "" || ev.Body == "" || ev.Path != "/" || ev.Address != "" {
			t.Fatalf("subscriber %d event = %+v", i, ev)
		}
		raw, err := json.Marshal(ev)
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]any
		if err := json.Unmarshal(raw, &fields); err != nil {
			t.Fatal(err)
		}
		if _, ok := fields["address"]; ok {
			t.Fatalf("event JSON has an address: %s", raw)
		}
		for _, k := range []string{"title", "body", "path"} {
			if _, ok := fields[k]; !ok {
				t.Fatalf("event JSON lacks %q: %s", k, raw)
			}
		}
	}
	late, cancel, err := s.events.Subscribe(s.events.CurrentSeq())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cancel)
	select {
	case ev := <-late:
		t.Fatalf("a later subscriber got %+v", ev)
	default:
	}
}

func TestWebToast_SwitchOffOrKindUncheckedPublishesNothing(t *testing.T) {
	t.Parallel()
	cases := map[string]settings.NotifyChannel{
		"switch off":     {Enabled: false, Kinds: []string{notifykind.ServerStarted}},
		"kind unchecked": {Enabled: true, Kinds: []string{notifykind.Disk}},
	}
	for name, ch := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			s := webToastServer(t, ch)
			sub, cancel, err := s.events.Subscribe(s.events.CurrentSeq())
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(cancel)
			sendServerStarted(s)
			select {
			case ev := <-sub:
				t.Fatalf("got %+v", ev)
			default:
			}
		})
	}
}

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

// Story 13: a person's manual mute of Slack stops Slack only.
func TestChannelBreaker_ManualSlackMuteLeavesTheOtherChannelsSending(t *testing.T) {
	t.Parallel()
	r := newBreakerRig(t, time.Now)
	r.s.chat.SessionAlert("p", nil, "name", "done", "body", true)
	eventually(t, "the first Slack post", func() bool { return r.posts.Load() == 1 })

	// The Slack key is the destination's; the other series are named by channel.
	channels := map[string]bool{"desktop": true, "web-toast": true, "push": true}
	if err := notify.UpdateNotificationState(r.s.cfg.Chat.GateStatePath, func(s *notify.NotificationState) {
		for key, ts := range s.Transports {
			if !channels[key] {
				ts.Muted, ts.Reason = true, "manual-disable"
				s.Transports[key] = ts
			}
		}
	}); err != nil {
		t.Fatal(err)
	}

	r.s.chat.SessionAlert("p", nil, "name", "done again", "body", true)
	r.desktop(1)
	r.s.chat.Push(notify.PushNote{Kind: notifykind.BudgetAndUsage, Title: "push"})
	_, cancel, err := r.s.events.Subscribe(r.s.events.CurrentSeq())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cancel)
	r.s.chat.WebToast(notify.WebToastNote{Kind: notifykind.BudgetAndUsage, Title: "toast"})

	eventually(t, "a push", func() bool { return r.hits.Load() == 1 })
	if got := r.banners.n.Load(); got != 1 {
		t.Errorf("banners = %d, want 1", got)
	}
	if got := len(r.series(t, "web-toast").Sends); got != 1 {
		t.Errorf("web toast sends = %d, want 1", got)
	}
	<-time.After(100 * time.Millisecond) // longer than the flush interval: a Slack post would have gone out by now
	if got := r.posts.Load(); got != 1 {
		t.Errorf("slack posts = %d, want 1 (muted)", got)
	}
}
