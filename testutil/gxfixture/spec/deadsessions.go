package spec

import "github.com/elentok/gx/tickets"

// The dead-session scenarios stage a server start whose saved sessions have no
// live holder: each session's fake exits with code 1, so the session shows
// Exited (crashed) and nothing of it is left running. A flow counts the
// holder and claude processes (fixturetest.Running.ProcessCounts) to see that a
// start launches none.

// DeadSessionCount is how many sessions the many-dead scenario seeds.
const DeadSessionCount = 40

// DeadManyScenario is DeadSessionCount saved sessions, all dead.
func DeadManyScenario() scenario {
	return scenario{DeadSessions: DeadSessionCount}
}

// DeadQuestionScenario is one dead session that raised a pending question and
// still has Unread.
func DeadQuestionScenario() scenario {
	return scenario{Chat: true, Sessions: []sessionSpec{
		{Project: project, Prompt: "hello", Actionable: "needs a decision", Crash: true},
	}}
}

// DeadDesignScenario is one dead design session, linked to a map.
func DeadDesignScenario() scenario {
	const mapAddr = "design-epic"
	return scenario{
		Epics:       []epic{{project, mapAddr, nil}},
		DesignSteps: map[string]string{project + ":" + mapAddr: string(tickets.StepResolveTickets)},
		Chat:        true,
		Sessions: []sessionSpec{
			{Project: project, Prompt: "hello", Link: project + ":" + mapAddr, Crash: true},
		},
	}
}
