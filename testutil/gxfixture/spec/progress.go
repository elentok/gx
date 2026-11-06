package spec

import "github.com/elentok/gx/server/servertest"

// RunningTicketTick is chat-capture with one epic whose first two tickets run at
// once on different harnesses (Claude, then Codex) and are held until the hold
// file is removed. The third waits for a free slot, so it only finishes after
// the release. The Progress interval is a few seconds.
func RunningTicketTick() scenario {
	one := func(id, slug string, opts servertest.TicketOpts) ticket { return ticket{id, slug, opts} }
	return scenario{
		Codex:       true,
		ChatCapture: true,
		Hold:        true,
		Epics: []epic{{project, "tick-epic", []ticket{
			one("01", "claude-runner", servertest.TicketOpts{}),
			one("02", "codex-runner", servertest.TicketOpts{Agent: "codex"}),
			one("03", "finisher", servertest.TicketOpts{}),
		}}},
		ProjectExtra: map[string]string{project: `,"max-agents":2`},
		Queue:        []string{project + ":tick-epic"},
		Env:          []string{"GX_CHAT_PROGRESS_INTERVAL=3s"},
	}
}
