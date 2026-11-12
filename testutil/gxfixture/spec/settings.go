package spec

import "github.com/elentok/gx/server/servertest"

// NotesNotifications is the hand-written notifications block in the `notes`
// project's project.json; the Settings page must leave it byte for byte.
const NotesNotifications = `"notifications": { "telegram":{"bot-token":"notes-token",  "chat-id":"42"} }`

// SettingsScenario is the Settings page fixture: a config with a non-default
// value in every section (a risky key left at default, Telegram set, Slack
// not), three projects, and one running and one waiting agent so the
// default-harness switch notice shows counts. demo holds the overrides, notes
// a hand-written notifications block, plain nothing.
func SettingsScenario() scenario {
	// The seed appends "checks":{} unless NoCheckConfig, which would duplicate
	// demo's key, so every project spells its own.
	const noChecks = `,"checks":{}`
	return scenario{
		Epics: []epic{
			{project, "running-epic", []ticket{{"01", "running", servertest.TicketOpts{}}}},
			{project, "waiting-epic", []ticket{{"01", "waiting", servertest.TicketOpts{}}}},
			{"notes", "notes-epic", nil},
			{"plain", "plain-epic", nil},
		},
		// Codex puts the fake codex on PATH so the default harness can be saved as
		// codex; --hide-codex brings back the host without it.
		Codex:         true,
		NoCheckConfig: true,
		ProjectExtra: map[string]string{
			project: `,"max-agents":1,"evolve":false,` +
				`"execution-queue":{"max-agents-per-epic":2},` +
				`"agents":{"sounds":{"enabled":false}},` +
				`"disk-space":{"cleanup-command":["true"]},` +
				`"checks":{"test":{"full":"true","limits":{"slow":{"go":"5s","vitest":"7s"}}}}`,
			"notes": `,` + NotesNotifications + noChecks,
			"plain": noChecks,
		},
		OwnRepos: []string{"notes", "plain"},
		Queue:    []string{project + ":running-epic", project + ":waiting-epic"},
		Hold:     true,
		Config: []string{
			"execution-queue.max-released-agents=3",
			"budget.soft-limit=50",
			"budget.hard-limit=80",
			"notifications.telegram.bot-token=fixture-bot-token",
			"notifications.telegram.chat-id=\"12345\"",
			"server.recently-finished=7",
			"agents.sounds.enabled=false",
			"recovery.notify-hold=30m",
			"ticket-store.commit-debounce=5",
		},
		// agent-runner is startup-only, so changing it leaves a pending restart.
		Steps: [][]string{
			{"server", "settings-set", "--set", "agent-runner=headless"},
		},
	}
}
