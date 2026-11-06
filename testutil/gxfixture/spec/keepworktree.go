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
	// KeepWorktree forces the cleanup a herdr-hosted run does when its agent
	// asks, so the kept worktree is the only thing that carries the file over.
	KeepWorktree = scenario{
		Epics:          keepWorktreeEpic,
		Queue:          []string{project + ":keep-epic"},
		Behaviour:      map[string]string{project + ":keep-epic/01": agentfake.BehaviourWriteAsk},
		DropAskedAgent: true,
	}
	// KeepAgent is the same ask on the unmodified pty runner, which keeps the
	// worktree, pane and session.
	KeepAgent = scenario{
		Epics:     keepWorktreeEpic,
		Queue:     []string{project + ":keep-epic"},
		Behaviour: map[string]string{project + ":keep-epic/01": agentfake.BehaviourWriteAsk},
	}
	// KeepCleanup is keepWorktree with a manual epic merge, so a test can cancel
	// the asking ticket and then merge the epic itself.
	KeepCleanup = scenario{
		Epics:          keepWorktreeEpic,
		Queue:          []string{project + ":keep-epic"},
		Behaviour:      map[string]string{project + ":keep-epic/01": agentfake.BehaviourWriteAsk},
		DropAskedAgent: true,
		ManualMerge:    []string{project},
	}
)
