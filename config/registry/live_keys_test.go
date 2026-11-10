package registry

import (
	"strings"
	"testing"
)

// liveKeys are the page keys that something outside config/ reads (audited in
// settings-page-impl tickets 22, 23 and 54c). Every other page key must be
// flagged Restart, so the page never says "Saved" for a value the running
// server ignores. A key nothing reads belongs in Exclusions(), not here.
var liveKeys = strings.Fields(`
execution-queue.max-agents execution-queue.max-agents-per-epic
execution-queue.max-one-off-agents execution-queue.waiting-slot-grace
execution-queue.max-released-agents execution-queue.stalled-nudge-delay
budget.soft-limit budget.hard-limit budget.long-ticket-turns
subscription.suppress-extra-usage-warning
notifications.telegram.bot-token notifications.telegram.chat-id notifications.slack.webhook-url
server.tab-env server.log-retention server.watch-config server.recently-finished
server.web.allowed-hosts server.web.totp server.web.dev-frontend server.web.dev-dir
cpu-watchdog.enabled cpu-watchdog.threshold-percent cpu-watchdog.sustain cpu-watchdog.cooldown
watchdog.max-tree-processes watchdog.interval watchdog.ulimit-headroom
disk-space.warn-below disk-space.clean-below disk-space.cleanup-command
agents.default-harness
agents.claude.model agents.claude.effort agents.claude.light.model agents.claude.light.effort
agents.claude.strongest.model agents.claude.strongest.effort
agents.codex.model agents.codex.effort agents.codex.light.model agents.codex.light.effort
agents.codex.strongest.model agents.codex.strongest.effort
agents.restore-sessions-on-start agents.banners.enabled agents.sounds.enabled
agents.sounds.done agents.sounds.attention agents.claude-args agents.fix-agent-remote-control
evolve recovery.enabled recovery.disabled recovery.follow-ups recovery.notify-hold
ticket-store.commit-debounce ticket-store.push-remote
skills.ci-fix skills.pr-text
pr.ci-fix-attempts-per-check pr.ci-fix-attempts-per-pr pr.no-checks-grace
`)

func TestPageKeysAreLiveOrRestart(t *testing.T) {
	live := map[string]bool{}
	for _, k := range liveKeys {
		live[k] = true
	}
	for _, e := range Default().All() {
		if e.Restart || e.ProjectOnly || live[e.Key.String()] {
			continue
		}
		t.Errorf("%s: page key with no Restart flag and no listed reader outside config/", e.Key)
	}
}
