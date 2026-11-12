package spec

import (
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil/agentfake"
)

// AnswerQueued is a ticket that asked and whose agent is gone (the cleanup a
// herdr-hosted run does), so answering it reopens it for gx to claim again:
// the "queued" outcome. The relaunch sees the uncommitted file and finishes.
var AnswerQueued = scenario{
	Epics:          []epic{{project, "queued-answer-epic", []ticket{{"01", "asks", servertest.TicketOpts{}}}}},
	Queue:          []string{project + ":queued-answer-epic"},
	Behaviour:      map[string]string{project + ":queued-answer-epic/01": agentfake.BehaviourWriteAsk},
	DropAskedAgent: true,
}

// AnswerFinished is a ticket that asked and whose agent is gone. A resume
// relaunches it, and the new agent reports finished with no commit, so the
// ticket pauses zero-commit, then commits once the hold is released. An
// answer from the form of the first ask, sent after the release, meets that
// late finish: the "adopted" outcome.
var AnswerFinished = scenario{
	Epics:          []epic{{project, "finished-answer-epic", []ticket{{"01", "stops", servertest.TicketOpts{}}}}},
	Queue:          []string{project + ":finished-answer-epic"},
	Behaviour:      map[string]string{project + ":finished-answer-epic/01": agentfake.BehaviourAskEmptyFinish},
	DropAskedAgent: true,
	Hold:           true,
	// The rescan would otherwise adopt the late finish before the answer arrives.
	HoldLateFinishScan: true,
}
