package spec

import (
	"strconv"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil/agentfake"
)

// The storm scenarios stage a process tree larger than the watchdog's
// threshold: the agent forks agentfake.StormDescendants processes while the
// threshold is StormThreshold, counted every StormInterval. A flow counts the
// leftover sleepers (fixturetest.Running.Sleepers) against a baseline taken
// before the storm.

// StormThreshold is the watchdog.max-tree-processes of the storm scenarios.
const StormThreshold = agentfake.StormDescendants / 2

// StormInterval is the watchdog.interval of the storm scenarios.
const StormInterval = "200ms"

var stormConfig = []string{
	"watchdog.max-tree-processes=" + strconv.Itoa(StormThreshold),
	"watchdog.interval=" + StormInterval,
}

// StormTicket is the ticket of the process-storm scenario.
const StormTicket = project + ":storm-epic/01"

// ProcessStorm is one queued ticket whose agent forks past the threshold: the
// watchdog kills it and the ticket pauses process-storm.
var ProcessStorm = scenario{
	Epics:     []epic{{project, "storm-epic", []ticket{{"01", "first", servertest.TicketOpts{}}}}},
	Queue:     []string{project + ":storm-epic"},
	Behaviour: map[string]string{StormTicket: agentfake.BehaviourStorm},
	Config:    stormConfig,
	// The trees are real, so the watchdog must read the real process table.
	RealProcesses: true,
}

// StormIdleTicket is the ticket of the session-storm scenario, which no
// session touches.
const StormIdleTicket = project + ":storm-session-epic/01"

// SessionStorm is an interactive session that forks past the threshold: the
// watchdog kills it, the session shows Exited, and the unqueued ticket
// stays as it was.
var SessionStorm = scenario{
	Epics: []epic{{project, "storm-session-epic", []ticket{{"01", "first", servertest.TicketOpts{}}}}},
	Chat:  true,
	Sessions: []sessionSpec{{
		Project: project,
		Prompt:  "hello",
		Control: &agentfake.Control{Action: agentfake.ActionDescendants, Count: agentfake.StormDescendants, Depth: 1},
	}},
	Config:        stormConfig,
	RealProcesses: true,
}

// OutsideStormCount is the size of the tree the outside-storm scenario starts.
const OutsideStormCount = agentfake.StormDescendants

// OutsideStorm is a tree above the threshold that gx did not launch: the
// watchdog covers agents only, so it stays alive.
var OutsideStorm = scenario{OutsideTree: OutsideStormCount, RealProcesses: true, Config: stormConfig}
