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

// AnswerFinished is a ticket whose agent reports finished with no commit, so it
// pauses zero-commit with a question, and commits once the hold is released.
// Releasing it before the answer makes the answer meet a late finish: the
// "adopted" outcome.
var AnswerFinished = scenario{
	Epics:     []epic{{project, "finished-answer-epic", []ticket{{"01", "stops", servertest.TicketOpts{}}}}},
	Queue:     []string{project + ":finished-answer-epic"},
	Behaviour: map[string]string{project + ":finished-answer-epic/01": agentfake.BehaviourEmptyFinish},
	Hold:      true,
}
