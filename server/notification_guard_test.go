package server

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/config/registry"
	"github.com/elentok/gx/notifykind"
	"github.com/elentok/gx/ralphloop/notify"
	"github.com/elentok/gx/server/settingsapi"
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
