package spec

import (
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil/agentfake"
)

// DeadRunTicket is the one ticket of the dead-run scenarios.
const DeadRunTicket = project + ":dead-run-epic/01"

// deadRun is one queued ticket whose agent ends its turn on a live background
// task, so the ticket stays claimed until a flow kills the server and the agent
// together (the migrate server script, plus the agent's holder). The agent
// writes work into its worktree when behaviour says so.
func deadRun(behaviour string) scenario {
	sc := recoveryEpic("dead-run-epic", servertest.TicketOpts{}, false)
	sc.Behaviour = map[string]string{DeadRunTicket: behaviour}
	sc.Migrate = true
	return sc
}

var (
	// DeadRunEmpty: the run left nothing in its worktree, so a restart reopens the ticket.
	DeadRunEmpty = deadRun(agentfake.BehaviourBackgroundTask)
	// DeadRunWithWork: the run left an uncommitted file, so a restart pauses the ticket.
	DeadRunWithWork = deadRun(agentfake.BehaviourBackgroundDirty)
)
