package spec

import (
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil/agentfake"
)

// keepWorktreeEpic is queued with a first ticket that leaves an uncommitted
// file and asks, and an independent second ticket that can start while the
// first waits.
var keepWorktreeEpic = []epic{{project, "keep-epic", []ticket{
	{"01", "asks", servertest.TicketOpts{}},
	{"02", "second", servertest.TicketOpts{}},
}}}

var (
	// KeepWorktree has the agent exit when it asks, so the kept worktree is the
	// only thing that carries the file over.
	KeepWorktree = scenario{
		Epics:              keepWorktreeEpic,
		Queue:              []string{project + ":keep-epic"},
		Behaviour:          map[string]string{project + ":keep-epic/01": agentfake.BehaviourWriteAsk},
		AgentExitsAfterAsk: true,
	}
	// KeepAgent is the same ask with the agent staying up, which keeps the
	// worktree, pane and session.
	KeepAgent = scenario{
		Epics:     keepWorktreeEpic,
		Queue:     []string{project + ":keep-epic"},
		Behaviour: map[string]string{project + ":keep-epic/01": agentfake.BehaviourWriteAsk},
	}
	// KeepCleanup is keepWorktree with a manual epic merge, so a test can cancel
	// the asking ticket and then merge the epic itself.
	KeepCleanup = scenario{
		Epics:              keepWorktreeEpic,
		Queue:              []string{project + ":keep-epic"},
		Behaviour:          map[string]string{project + ":keep-epic/01": agentfake.BehaviourWriteAsk},
		AgentExitsAfterAsk: true,
		ManualMerge:        []string{project},
	}
)
