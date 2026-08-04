package tickets

import (
	"time"

	tea "charm.land/bubbletea/v2"
)

// autoRefreshInterval is how often the Tickets and Queue tabs poll `.scratch/`
// for ticket status changes made outside this Update loop — another gx
// process, a manual edit, ralph-loop running in a different worktree —
// mirroring implement.go's own poll cadence but decoupled from any run this
// process is itself tracking.
const autoRefreshInterval = 2 * time.Second

// autoRefreshMsg drives the self-perpetuating poll loop each tab starts once
// its own initial load completes (see epicsLoadedMsg/queueEpicsLoadedMsg):
// every tick reloads from disk and reschedules the next tick.
type autoRefreshMsg struct{}

func cmdAutoRefresh() tea.Cmd {
	return tea.Tick(autoRefreshInterval, func(time.Time) tea.Msg {
		return autoRefreshMsg{}
	})
}
