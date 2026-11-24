package settings_test

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/notifykind"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/settings"
)

func TestSettingsFromConfig_CarriesNotifyChannels(t *testing.T) {
	t.Parallel()
	cfg := config.Default()
	cfg.NotificationChannels.Slack = config.ChannelConfig{Enabled: false, Kinds: []string{"disk"}}
	cfg.NotificationChannels.Push.Kinds = []string{}
	got := settings.FromConfig(cfg).NotifyChannels

	if got.Slack.Enabled() || !slices.Equal(got.Slack.KindIDs(), []string{"disk"}) {
		t.Errorf("slack = %+v", got.Slack)
	}
	if !got.Push.Enabled() || got.Push.Kinds == nil || len(got.Push.KindIDs()) != 0 {
		t.Errorf("push = %+v, want on with an explicit empty list", got.Push)
	}
	for name, ch := range map[string]settings.NotifyChannel{"desktop": got.Desktop, "web-toast": got.WebToast, "telegram": got.Telegram} {
		if !ch.Enabled() || !slices.Equal(ch.KindIDs(), notifykind.DefaultIDs()) {
			t.Errorf("%s = %+v, want the new install defaults", name, ch)
		}
	}
}

func TestSettings_ZeroValueReadsAsNewInstall(t *testing.T) {
	t.Parallel()
	var s settings.Settings
	for name, ch := range map[string]settings.NotifyChannel{
		"desktop": s.NotifyChannels.Desktop, "web-toast": s.NotifyChannels.WebToast, "telegram": s.NotifyChannels.Telegram,
		"slack": s.NotifyChannels.Slack, "push": s.NotifyChannels.Push,
	} {
		ids := ch.KindIDs()
		if !ch.Enabled() || len(ids) != 15 || slices.Contains(ids, notifykind.LongTicket) {
			t.Errorf("%s: enabled=%v kinds=%v, want on with the 15 default ids", name, ch.Enabled(), ids)
		}
	}
}

func TestConfigReload_UnmarkedFileReadsAsUpgrade(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	path := filepath.Join(t.TempDir(), "config.json")
	h := servertest.StartWithStore(t, store, servertest.WithConfigFile(path))
	servertest.WriteConfig(t, path, store, nil)
	if res := reload(t, h); res.Refused {
		t.Fatalf("baseline = %+v", res)
	}

	// Not WriteConfig: it presets the marker, which makes a file a new install.
	if err := os.WriteFile(path, []byte(`{"ticket-store":{"path":`+strconv.Quote(store)+`},"runner-wait":"100ms","server":{"watch-config":false}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	res := reload(t, h)
	if res.Refused {
		t.Fatalf("reload = %+v", res)
	}
	if !slices.Contains(res.Changed, "notification-channels.push.kinds") {
		t.Errorf("reload = %+v, want push kinds changed", res)
	}
	got := h.Server.Settings().Load().NotifyChannels
	if !slices.Equal(got.Push.KindIDs(), []string{notifykind.SessionAlert, notifykind.DesignQuestion, notifykind.PausedTicket}) {
		t.Errorf("push = %v", got.Push.KindIDs())
	}
	if !slices.Equal(got.Desktop.KindIDs(), []string{notifykind.SessionAlert, notifykind.DesignQuestion}) || len(got.WebToast.KindIDs()) != 0 {
		t.Errorf("desktop = %v, web-toast = %v", got.Desktop.KindIDs(), got.WebToast.KindIDs())
	}
	if len(got.Telegram.KindIDs()) != 15 || len(got.Slack.KindIDs()) != 15 {
		t.Errorf("telegram = %v, slack = %v", got.Telegram.KindIDs(), got.Slack.KindIDs())
	}
}

func TestConfigReload_NotifyChannelKeysApply(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	path := filepath.Join(t.TempDir(), "config.json")
	h := servertest.StartWithStore(t, store, servertest.WithConfigFile(path))
	servertest.WriteConfig(t, path, store, nil)
	if res := reload(t, h); res.Refused {
		t.Fatalf("baseline = %+v", res)
	}

	servertest.WriteConfig(t, path, store, map[string]any{"notification-channels": map[string]any{
		"telegram": map[string]any{"enabled": false},
		"push":     map[string]any{"kinds": []string{"disk"}},
	}})
	res := reload(t, h)
	for _, key := range []string{"notification-channels.telegram.enabled", "notification-channels.push.kinds"} {
		if !slices.Contains(res.Changed, key) {
			t.Errorf("reload = %+v, want %s changed", res, key)
		}
	}
}
