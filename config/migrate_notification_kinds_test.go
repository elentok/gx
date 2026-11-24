package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/elentok/gx/notifykind"
)

func upgradedLists(t *testing.T, data []byte) map[string][]string {
	t.Helper()
	var doc struct {
		Channels map[string]map[string]json.RawMessage `json:"notification-channels"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse %s: %v", data, err)
	}
	out := map[string][]string{}
	for ch, m := range doc.Channels {
		if _, ok := m["enabled"]; ok {
			t.Errorf("%s: an enabled switch was written", ch)
		}
		var ids []string
		if err := json.Unmarshal(m["kinds"], &ids); err != nil {
			t.Fatalf("%s kinds: %v", ch, err)
		}
		out[ch] = ids
	}
	return out
}

func TestMigrateNotificationKindsWritesEachChannelsList(t *testing.T) {
	out, changed := MigrateNotificationKinds([]byte("{}"))
	if !changed {
		t.Fatal("unmarked {} is an upgrade")
	}
	lists := upgradedLists(t, out)
	if got, want := lists["push"], []string{"session-alert", "design-question", "paused-ticket"}; !slices.Equal(got, want) {
		t.Errorf("push = %v", got)
	}
	if got, want := lists["desktop"], []string{"session-alert", "design-question"}; !slices.Equal(got, want) {
		t.Errorf("desktop = %v", got)
	}
	if got := lists["web-toast"]; got == nil || len(got) != 0 {
		t.Errorf("web-toast = %#v, want empty list", got)
	}
	for _, ch := range []string{"telegram", "slack"} {
		if len(lists[ch]) != len(notifykind.IDs())-1 {
			t.Errorf("%s has %d ids", ch, len(lists[ch]))
		}
	}
	for ch, ids := range lists {
		if slices.Contains(ids, notifykind.LongTicket) {
			t.Errorf("%s lists long-ticket", ch)
		}
		for _, id := range ids {
			if !notifykind.Valid(id) {
				t.Errorf("%s: %q not in the catalog", ch, id)
			}
		}
	}
	var top map[string]any
	if err := json.Unmarshal(out, &top); err != nil || top[NotificationKindsMarkerKey] != true {
		t.Errorf("marker missing: %s", out)
	}
}

func TestMigrateNotificationKindsKeepsAPresentList(t *testing.T) {
	in := []byte(`{"notification-channels":{"push":{"kinds":["disk"]}}}`)
	out, changed := MigrateNotificationKinds(in)
	if !changed {
		t.Fatal("want an upgrade")
	}
	lists := upgradedLists(t, out)
	if !slices.Equal(lists["push"], []string{"disk"}) {
		t.Errorf("push overwritten: %v", lists["push"])
	}
	if len(lists["desktop"]) != 2 {
		t.Errorf("desktop not written: %v", lists["desktop"])
	}
}

func TestMigrateNotificationKindsLeavesOtherFilesAlone(t *testing.T) {
	for name, in := range map[string]string{
		"marked":       `{"notification-kinds-migrated": true}`,
		"invalid json": `{"a":`,
		"not object":   `[1]`,
		"null":         `null`,
	} {
		out, changed := MigrateNotificationKinds([]byte(in))
		if changed || string(out) != in {
			t.Errorf("%s: changed=%v out=%s", name, changed, out)
		}
	}
}

func useConfigDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	prev := userConfigDirFn
	userConfigDirFn = func() (string, error) { return dir, nil }
	t.Cleanup(func() { userConfigDirFn = prev })
	return filepath.Join(dir, "gx", "config.json")
}

func TestMigrateNotificationKindsLoadWritesOnceAndNewInstallWritesNothing(t *testing.T) {
	path := useConfigDir(t)
	if _, err := Load(); err != nil {
		t.Fatalf("Load without a file: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("a new install wrote the file: %v", err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !slices.Equal(cfg.NotificationChannels.Push.Kinds, []string{"session-alert", "design-question", "paused-ticket"}) {
		t.Errorf("push = %v", cfg.NotificationChannels.Push.Kinds)
	}
	if !cfg.NotificationChannels.Push.Enabled {
		t.Error("the default switch (on) should apply")
	}
	first, _ := os.ReadFile(path)
	if _, err := Load(); err != nil {
		t.Fatal(err)
	}
	second, _ := os.ReadFile(path)
	if string(first) != string(second) {
		t.Errorf("second load changed the file:\n%s\n%s", first, second)
	}
	if string(first) == "{}" {
		t.Error("the file was not rewritten")
	}
}

// Story 39: a marked config keeps the list it holds when the registry default
// moves on. The stored list stands in for an older default: it differs from
// today's.
func TestMigrateNotificationKindsMarkedConfigIgnoresALaterDefault(t *testing.T) {
	path := useConfigDir(t)
	stale := notifykind.DefaultIDs()[1:]
	in, err := json.Marshal(map[string]any{
		NotificationKindsMarkerKey: true,
		"notification-channels": map[string]any{
			"push": map[string]any{"kinds": stale},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, in, 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := cfg.NotificationChannels.Push.Kinds; !slices.Equal(got, stale) || slices.Equal(got, notifykind.DefaultIDs()) {
		t.Errorf("push = %v, want the stored list %v", got, stale)
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(in) {
		t.Errorf("a marked file was rewritten:\n%s\n%s", in, after)
	}
}

func TestMigrateNotificationKindsFailedWriteStillReturnsUpgradeLists(t *testing.T) {
	path := useConfigDir(t)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Dir(path), 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(filepath.Dir(path), 0o755) })
	if f, err := os.CreateTemp(filepath.Dir(path), "probe"); err == nil {
		f.Close()
		t.Skip("directory is still writable (running as root)")
	}
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.NotificationChannels.WebToast.Kinds) != 0 || len(cfg.NotificationChannels.Desktop.Kinds) != 2 {
		t.Errorf("got the new-install lists: %+v", cfg.NotificationChannels)
	}
	if b, _ := os.ReadFile(path); string(b) != "{}" {
		t.Errorf("file changed: %s", b)
	}
}

func TestMigrateNotificationKindsInitStampsMarker(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gx", "config.json")
	if _, err := initAt(func() (string, error) { return path, nil }); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(path)
	if _, err := loadFrom(func() (string, error) { return path, nil }, DataDir); err != nil {
		t.Fatalf("load: %v", err)
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Errorf("Load rewrote a new config:\n%s", after)
	}
	if _, changed := MigrateNotificationKinds(before); changed {
		t.Error("init file is not marked")
	}
}
