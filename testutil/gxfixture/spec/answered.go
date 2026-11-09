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
