package main

import (
	"github.com/elentok/gx/server/servertest"
)

var readyEpic = []epic{{project, "ready-epic", []ticket{{"01", "first", servertest.TicketOpts{}}}}}

var (
	// rejectedFirstPrompt is a server-launched session whose agent rejects the
	// slash first prompt as an unknown command, so the session shows a failure.
	rejectedFirstPrompt = scenario{
		unknownSkill: true,
		sessions:     []sessionSpec{{project: project, prompt: "/gx-implement demo:ready-epic/01", rejected: true}},
	}
	// readyDelayed is a queued ticket whose agent shows its ready signal late;
	// the launch prompt waits for it, well inside the wait cap.
	readyDelayed = scenario{
		epics:      readyEpic,
		queue:      []string{project + ":ready-epic"},
		readyDelay: "2s",
	}
	// readyNever is a queued ticket whose agent never shows the ready signal;
	// the launch prompt goes out when the shortened wait runs out.
	readyNever = scenario{
		epics:      readyEpic,
		queue:      []string{project + ":ready-epic"},
		readyDelay: "1h",
		readyWait:  "3s",
	}
)
