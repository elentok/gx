package registry

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/elentok/gx/notifykind"
)

func TestCoreEntries(t *testing.T) {
	r := New(coreEntries()...)
	type want struct {
		key        KeyPath
		typ        ValueType
		section    Section
		projectOv  bool
		projectKey string
		restart    bool
		risky      bool
	}
	q, b, n, s := SectionExecutionQueue, SectionBudget, SectionDesktop, SectionServer
	cases := []want{
		{key: Key("execution-queue", "max-agents"), typ: TypeNumber, section: q, projectKey: "max-agents"},
		{key: Key("execution-queue", "max-agents-per-epic"), typ: TypeNumber, section: q, projectOv: true},
		{key: Key("execution-queue", "max-one-off-agents"), typ: TypeNumber, section: q, projectOv: true},
		{key: Key("execution-queue", "waiting-slot-grace"), typ: TypeDuration, section: q},
		{key: Key("execution-queue", "max-released-agents"), typ: TypeNumber, section: q},
		{key: Key("execution-queue", "stalled-nudge-delay"), typ: TypeDuration, section: q},
		{key: Key("budget", "soft-limit"), typ: TypeNumber, section: b},
		{key: Key("budget", "hard-limit"), typ: TypeNumber, section: b},
		{key: Key("budget", "long-ticket-turns"), typ: TypeNumber, section: b},
		{key: Key("subscription", "suppress-extra-usage-warning"), typ: TypeBool, section: b},
		{key: Key("agents", "banners", "enabled"), typ: TypeBool, section: n},
		{key: Key("agents", "sounds", "enabled"), typ: TypeBool, section: n},
		{key: Key("agents", "sounds", "done"), typ: TypeString, section: n},
		{key: Key("agents", "sounds", "attention"), typ: TypeString, section: n},
		{key: Key("notifications", "telegram", "bot-token"), typ: TypeSecret, section: SectionTelegram},
		{key: Key("notifications", "telegram", "chat-id"), typ: TypeString, section: SectionTelegram},
		{key: Key("notifications", "slack", "webhook-url"), typ: TypeSecret, section: SectionSlack},
		{key: Key("server", "tab-env"), typ: TypeStringList, section: s},
		{key: Key("server", "log-retention"), typ: TypeDuration, section: s},
		{key: Key("server", "watch-config"), typ: TypeBool, section: s},
		{key: Key("server", "recently-finished"), typ: TypeNumber, section: SectionDisplayDev},
		{key: Key("server", "web", "listen"), typ: TypeString, section: SectionAccess, restart: true, risky: true},
		{key: Key("server", "web", "dev-frontend"), typ: TypeBool, section: SectionDisplayDev},
		{key: Key("server", "web", "dev-dir"), typ: TypeString, section: SectionDisplayDev},
		{key: Key("server", "web", "allowed-hosts"), typ: TypeStringList, section: SectionAccess, risky: true},
		{key: Key("server", "web", "files", "roots"), typ: TypeStringList, section: SectionAccess, risky: true},
		{key: Key("server", "web", "totp"), typ: TypeBool, section: SectionAccess, risky: true},
		{key: Key("server", "web", "vi-escape"), typ: TypeString, section: SectionKeybindings},
		{key: Key("server", "web", "leader-key-mac"), typ: TypeString, section: SectionKeybindings},
		{key: Key("server", "web", "leader-key"), typ: TypeString, section: SectionKeybindings},
		{key: Key("cpu-watchdog", "enabled"), typ: TypeBool, section: SectionWatchdogs},
		{key: Key("cpu-watchdog", "threshold-percent"), typ: TypeNumber, section: SectionWatchdogs},
		{key: Key("cpu-watchdog", "sustain"), typ: TypeDuration, section: SectionWatchdogs},
		{key: Key("cpu-watchdog", "cooldown"), typ: TypeDuration, section: SectionWatchdogs},
		{key: Key("watchdog", "max-tree-processes"), typ: TypeNumber, section: SectionWatchdogs},
		{key: Key("watchdog", "hard-max-tree-processes"), typ: TypeNumber, section: SectionWatchdogs},
		{key: Key("watchdog", "interval"), typ: TypeDuration, section: SectionWatchdogs},
		{key: Key("watchdog", "ulimit-headroom"), typ: TypeNumber, section: SectionWatchdogs},
		{key: Key("disk-space", "warn-below"), typ: TypeSize, section: SectionDiskSpace},
		{key: Key("disk-space", "clean-below"), typ: TypeSize, section: SectionDiskSpace},
		{key: Key("disk-space", "cleanup-command"), typ: TypeStringList, section: SectionDiskSpace, projectOv: true},
		{key: Key("mental-health", "enabled"), typ: TypeBool, section: SectionMentalHealth},
		{key: Key("mental-health", "bedtime", "enabled"), typ: TypeBool, section: SectionMentalHealth},
		{key: Key("mental-health", "bedtime", "start"), typ: TypeString, section: SectionMentalHealth},
		{key: Key("mental-health", "bedtime", "end"), typ: TypeString, section: SectionMentalHealth},
		{key: Key("mental-health", "bedtime", "message"), typ: TypeString, section: SectionMentalHealth},
		{key: Key("mental-health", "break", "enabled"), typ: TypeBool, section: SectionMentalHealth},
		{key: Key("mental-health", "break", "work-minutes"), typ: TypeNumber, section: SectionMentalHealth},
		{key: Key("mental-health", "break", "break-minutes"), typ: TypeNumber, section: SectionMentalHealth},
		{key: Key("mental-health", "break", "message"), typ: TypeString, section: SectionMentalHealth},
		{key: Key("server", "web", "push-contact-email"), typ: TypeSecret, section: SectionPush},
		{key: Key("notification-channels", "desktop", "enabled"), typ: TypeBool, section: SectionDesktop},
		{key: Key("notification-channels", "desktop", "kinds"), typ: TypeStringList, section: SectionDesktop},
		{key: Key("notification-channels", "web-toast", "enabled"), typ: TypeBool, section: SectionWebToast},
		{key: Key("notification-channels", "web-toast", "kinds"), typ: TypeStringList, section: SectionWebToast},
		{key: Key("notification-channels", "telegram", "enabled"), typ: TypeBool, section: SectionTelegram},
		{key: Key("notification-channels", "telegram", "kinds"), typ: TypeStringList, section: SectionTelegram},
		{key: Key("notification-channels", "slack", "enabled"), typ: TypeBool, section: SectionSlack},
		{key: Key("notification-channels", "slack", "kinds"), typ: TypeStringList, section: SectionSlack},
		{key: Key("notification-channels", "push", "enabled"), typ: TypeBool, section: SectionPush},
		{key: Key("notification-channels", "push", "kinds"), typ: TypeStringList, section: SectionPush},
	}
	if got := len(r.All()); got != len(cases) {
		t.Fatalf("entries = %d, want %d", got, len(cases))
	}
	for _, c := range cases {
		e, ok := r.Lookup(c.key)
		if !ok {
			t.Errorf("%v: missing", c.key)
			continue
		}
		if e.Type != c.typ || e.Section != c.section || e.ProjectOverridable != c.projectOv ||
			e.ProjectKey.String() != c.projectKey || e.Restart != c.restart || e.Risky != c.risky {
			t.Errorf("%v: got %+v", c.key, e)
		}
		if e.Description == "" || e.ProjectOnly {
			t.Errorf("%v: description empty or project-only", c.key)
		}
		if e.Risky != (e.ConfirmText != "") {
			t.Errorf("%v: risky and confirm text disagree", c.key)
		}
	}
}

func TestNotificationKindEntriesEnumIsTheCatalog(t *testing.T) {
	for _, ch := range []string{"desktop", "web-toast", "telegram", "slack", "push"} {
		e, ok := Default().Lookup(Key("notification-channels", ch, "kinds"))
		if !ok || !slices.Equal(e.Enum, notifykind.IDs()) {
			t.Errorf("%s: kinds entry missing or its Enum differs from the catalog ids: %v", ch, e.Enum)
		}
	}
}

func TestPushContactEmailEntry(t *testing.T) {
	e, ok := Default().Lookup(Key("server", "web", "push-contact-email"))
	if !ok || !e.WriteOnly() {
		t.Fatalf("entry missing or not write-only: %v", ok)
	}
	for raw, ok := range map[string]bool{
		`""`: true, `"me@example.com"`: true,
		`"Me <me@example.com>"`: false, `"a@example.com, b@example.com"`: false, `"me@example.com "`: false,
		`"mailto:me@example.com"`: false, `"nope"`: false, `5`: false,
	} {
		if err := CheckValue(e, json.RawMessage(raw), nil); (err == nil) != ok {
			t.Errorf("%s: err = %v, want ok=%v", raw, err, ok)
		}
	}
}

func TestLeaderKeyEntries(t *testing.T) {
	r := Default()
	for _, key := range []string{"leader-key-mac", "leader-key"} {
		e, ok := r.Lookup(Key("server", "web", key))
		if !ok {
			t.Fatalf("%s: missing", key)
		}
		if !strings.Contains(e.Description, "Press it to see the chords") {
			t.Errorf("%s: description %q lacks the chords hint", key, e.Description)
		}
		for raw, ok := range map[string]bool{
			`"cmd+e"`: true, `"alt+e"`: true, `"ctrl+shift+j"`: true, `"alt+7"`: true, `""`: true,
			`"shift+e"`: false, `"e"`: false, `"cmd+E"`: false, `"cmd+"`: false, `"cmd+ee"`: false, `"meta+e"`: false, `5`: false,
		} {
			err := CheckValue(e, json.RawMessage(raw), nil)
			if (err == nil) != ok {
				t.Errorf("%s %s: err = %v, want ok=%v", key, raw, err, ok)
			}
		}
	}
	e, _ := r.Lookup(Key("server", "web", "leader-key"))
	if err := CheckValue(e, json.RawMessage(`"shift+e"`), nil); err == nil || !strings.HasSuffix(err.Error(), ".") {
		t.Errorf("refusal should be a short sentence, got %v", err)
	}
}
