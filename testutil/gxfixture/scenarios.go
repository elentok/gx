package main

import (
	"maps"
	"net"
	"slices"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil/agentfake"
)

const project = "demo"

type ticket struct {
	id, slug string
	opts     servertest.TicketOpts
}

type epic struct {
	project, name string
	tickets       []ticket
}

type scenario struct {
	// projects have no epics.
	projects []string
	epics    []epic
	// behaviour is the fake worker's per-ticket behaviour, by address.
	behaviour map[string]string
	// queue lists the epics put on the queue once the server is up.
	queue []string
}

var scenarios = map[string]scenario{
	// ready: an epic that is not queued yet.
	"ready": {
		epics: []epic{{project, "ready-epic", []ticket{
			{"01", "first", servertest.TicketOpts{}},
			{"02", "second", servertest.TicketOpts{BlockedBy: []string{"01"}}},
		}}},
	},
	// mixed: tickets in several states, and a worker that asks and fails.
	"mixed": {
		epics: []epic{{project, "mixed-epic", []ticket{
			{"01", "merged", servertest.TicketOpts{Status: "done"}},
			{"02", "open", servertest.TicketOpts{}},
			{"03", "blocked", servertest.TicketOpts{BlockedBy: []string{"02"}}},
			{"04", "drafted", servertest.TicketOpts{Status: "draft"}},
			{"05", "dropped", servertest.TicketOpts{Status: "cancelled"}},
			{"06", "asks", servertest.TicketOpts{}},
			{"07", "fails", servertest.TicketOpts{}},
		}}},
		behaviour: map[string]string{
			project + ":mixed-epic/06": agentfake.BehaviourAsk,
			project + ":mixed-epic/07": agentfake.BehaviourFail,
		},
	},
	// finishing: queued, and its last ticket can finish and merge.
	"finishing": {
		epics: []epic{{project, "finishing-epic", []ticket{
			{"01", "merged", servertest.TicketOpts{Status: "done"}},
			{"02", "last", servertest.TicketOpts{BlockedBy: []string{"01"}}},
		}}},
		queue: []string{project + ":finishing-epic"},
	},
	// edge: a project with no epics, an epic with no tickets, and a ticket
	// blocked by a dependency that is not merged.
	"edge": {
		projects: []string{"bare"},
		epics: []epic{
			{project, "empty-epic", nil},
			{project, "blocked-epic", []ticket{
				{"01", "unmerged", servertest.TicketOpts{}},
				{"02", "waiting", servertest.TicketOpts{BlockedBy: []string{"01"}}},
			}},
		},
	},
}

func scenarioNames() []string {
	return slices.Sorted(maps.Keys(scenarios))
}

// freePort asks the kernel for an unused loopback port.
func freePort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port, nil
}
