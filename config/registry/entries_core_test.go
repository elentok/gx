package registry

import "testing"

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
	q, b, n, s := SectionExecutionQueue, SectionBudget, SectionNotifications, SectionServer
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
		{key: Key("notifications", "telegram", "bot-token"), typ: TypeSecret, section: n},
		{key: Key("notifications", "telegram", "chat-id"), typ: TypeString, section: n},
		{key: Key("notifications", "slack", "webhook-url"), typ: TypeSecret, section: n},
		{key: Key("server", "tcp-listen"), typ: TypeBool, section: s, restart: true, risky: true},
		{key: Key("server", "tab-env"), typ: TypeStringList, section: s},
		{key: Key("server", "log-retention"), typ: TypeDuration, section: s},
		{key: Key("server", "watch-config"), typ: TypeBool, section: s},
		{key: Key("server", "recently-finished"), typ: TypeNumber, section: SectionWebUI},
		{key: Key("server", "web", "listen"), typ: TypeString, section: SectionWebUI, restart: true, risky: true},
		{key: Key("server", "web", "dev-frontend"), typ: TypeBool, section: SectionWebUI},
		{key: Key("server", "web", "dev-dir"), typ: TypeString, section: SectionWebUI},
		{key: Key("server", "web", "allowed-hosts"), typ: TypeStringList, section: SectionWebUI, risky: true},
		{key: Key("server", "web", "totp"), typ: TypeBool, section: SectionWebUI, risky: true},
		{key: Key("server", "web", "vi-escape"), typ: TypeString, section: SectionWebUI},
		{key: Key("cpu-watchdog", "enabled"), typ: TypeBool, section: SectionCPUWatchdog},
		{key: Key("cpu-watchdog", "threshold-percent"), typ: TypeNumber, section: SectionCPUWatchdog},
		{key: Key("cpu-watchdog", "sustain"), typ: TypeDuration, section: SectionCPUWatchdog},
		{key: Key("cpu-watchdog", "cooldown"), typ: TypeDuration, section: SectionCPUWatchdog},
		{key: Key("watchdog", "max-tree-processes"), typ: TypeNumber, section: SectionProcessWatchdog},
		{key: Key("watchdog", "interval"), typ: TypeDuration, section: SectionProcessWatchdog},
		{key: Key("watchdog", "ulimit-headroom"), typ: TypeNumber, section: SectionProcessWatchdog},
		{key: Key("disk-space", "warn-below"), typ: TypeSize, section: SectionDiskSpace},
		{key: Key("disk-space", "clean-below"), typ: TypeSize, section: SectionDiskSpace},
		{key: Key("disk-space", "cleanup-command"), typ: TypeStringList, section: SectionDiskSpace, projectOv: true},
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
