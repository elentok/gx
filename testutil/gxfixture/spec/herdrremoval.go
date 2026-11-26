package spec

import "github.com/elentok/gx/server/servertest"

const (
	// HerdrLeftoverEmptyTicket is the ticket of the herdr-leftover-empty scenario.
	HerdrLeftoverEmptyTicket = project + ":herdr-empty-epic/01"
	// HerdrLeftoverWorkTicket is the ticket of the herdr-leftover-work scenario.
	HerdrLeftoverWorkTicket = project + ":herdr-work-epic/01"
)

// herdrLeftover is one queued ticket claimed by a run recorded on the removed
// herdr runner, with a clean worktree or one commit.
func herdrLeftover(epicName, addr string, commit bool) scenario {
	sc := recoveryEpic(epicName, servertest.TicketOpts{}, false)
	sc.LeftoverRuns = []LeftoverRun{{Address: addr, Runner: "herdr", Commit: commit}}
	// queue add refuses an epic with no open ticket, so storeseed.SeedLeftoverRuns queues it on disk.
	sc.Queue = nil
	return sc
}

var (
	// HerdrConfig: the config says agent-runner: herdr. The server reads it as
	// auto, logs one warning, and runs the queued ticket on the PTY runner.
	HerdrConfig = scenario{
		Epics:  []epic{{project, "herdr-config-epic", []ticket{{"01", "first", servertest.TicketOpts{}}}}},
		Queue:  []string{project + ":herdr-config-epic"},
		Config: []string{"agent-runner=herdr"},
	}
	// HerdrLeftoverEmpty: a herdr run left an empty worktree, so the start reopens the ticket.
	HerdrLeftoverEmpty = herdrLeftover("herdr-empty-epic", HerdrLeftoverEmptyTicket, false)
	// HerdrLeftoverWork: a herdr run left a commit, so the start pauses the ticket as handle-mismatch.
	HerdrLeftoverWork = herdrLeftover("herdr-work-epic", HerdrLeftoverWorkTicket, true)
	// AgentPaneBusy: a ticket paused with the removed agent_pane_busy reason and
	// the start-up migration still pending, which rewrites it to iteration-error.
	AgentPaneBusy = scenario{
		Epics:   []epic{{project, "pane-busy-epic", []ticket{{"01", "first", servertest.TicketOpts{PauseReason: "agent_pane_busy"}}}}},
		Migrate: true,
	}
	// OldRunLog: a run log with event types and a failure kind the code no longer defines.
	OldRunLog = scenario{
		Epics: []epic{{project, "old-log-epic", []ticket{{"01", "first", servertest.TicketOpts{}}}}},
		RunLog: map[string][]RunLogLine{project + ":old-log-epic": {
			{Type: "herdr-unavailable"},
			{Type: "herdr-available"},
			{Type: "herdr-agent-lost"},
			{Type: "failure", Kind: "agent_pane_busy", Reason: "agent pane busy"},
		}},
	}
)
