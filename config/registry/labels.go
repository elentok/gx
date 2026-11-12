package registry

// labels names the rows whose key's last segment reads badly on its own, such
// as "enabled" or "model". A row without an entry here is named from its key.
var labels = map[string]string{
	"notifications.telegram.bot-token": "Telegram bot token",
	"notifications.telegram.chat-id":   "Telegram chat ID",
	"notifications.slack.webhook-url":  "Slack webhook URL",
	"server.tcp-listen":                "Listen on TCP",
	"server.tab-env":                   "Tab environment variables",
	"server.watch-config":              "Reload config on save",
	"server.recently-finished":         "Recently finished list size",
	"server.web.listen":                "Listen address",
	"server.web.dev-frontend":          "Dev mode",
	"server.web.dev-dir":               "Dev checkout path",
	"server.web.totp":                  "Require one-time code",
	"server.web.vi-escape":             "Vi escape sequence",
	"cpu-watchdog.enabled":             "Watch CPU use",
	"cpu-watchdog.threshold-percent":   "CPU threshold (% of one core)",
	"cpu-watchdog.sustain":             "Sustain time",
	"watchdog.max-tree-processes":      "Max processes per agent",
	"watchdog.interval":                "Check interval",
	"watchdog.ulimit-headroom":         "Process headroom",
	"agents.default-harness":           "Default agent",
	"agents.restore-sessions-on-start": "Restore sessions on start",
	"agents.banners.enabled":           "OS banners",
	"agents.sounds.enabled":            "Sounds",
	"agents.sounds.done":               "Done sound",
	"agents.sounds.attention":          "Attention sound",
	"agents.claude-args":               "Extra Claude arguments",
	"agents.fix-agent-remote-control":  "Remote control for fix agents",
	"agent-runner":                     "Agent runner",
	"runner-wait":                      "Runner wait",
	"skills.ci-fix":                    "CI-fix skills",
	"skills.pr-text":                   "PR text skill",
	"pr.ci-fix-attempts-per-check":     "CI-fix attempts per check",
	"pr.ci-fix-attempts-per-pr":        "CI-fix attempts per PR",
	"pr.no-checks-grace":               "No-checks grace",
	"recovery.enabled":                 "Recovery enabled",
	"recovery.disabled":                "Disabled recovery steps",
	"recovery.follow-ups":              "Follow-ups target",
	"recovery.notify-hold":             "Notify hold",
	"ticket-store.commit-debounce":     "Commit debounce (seconds)",
	"ticket-store.push-remote":         "Push remote",
}

// agentLabel names an agents.<claude|codex>[.light|.strongest].<model|effort> row.
func agentLabel(path KeyPath) string {
	if len(path) < 3 || path[0] != "agents" || (path[1] != "claude" && path[1] != "codex") {
		return ""
	}
	name := map[string]string{"claude": "Claude", "codex": "Codex"}[path[1]]
	for _, tier := range path[2 : len(path)-1] {
		name += " " + tier
	}
	return name + " " + path[len(path)-1]
}

func labelFor(e Entry) string {
	if e.Label != "" {
		return e.Label
	}
	if l := agentLabel(e.Key); l != "" {
		return l
	}
	return labels[e.Key.String()]
}
