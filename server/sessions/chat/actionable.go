package chat

import (
	"strings"

	"github.com/elentok/gx/grillround"
	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
)

// RunEndNext is the Actionable a stepped session's run-end line raises:
// Continue in a fresh conversation, or just the line when the flow is at a
// final step.
func RunEndNext(text string, ref FlowRef, agent string, final bool) (wire.SessionActionable, bool) {
	line := grillround.RunEndLine.FindString(text)
	if line == "" {
		return wire.SessionActionable{}, false
	}
	a := wire.SessionActionable{Reason: strings.TrimSpace(line), Derived: true}
	if !final {
		a.Action = ContinueAction(ref, agent)
	}
	return a, true
}

// FlowRef names the flow a session runs and the identity it runs on: a
// Design's ticket address, a Triage's project. The zero value is a session
// with no flow.
type FlowRef struct {
	Flow tickets.Flow
	Key  string
}

// ContinueAction starts the flow's next step in a fresh conversation: Codex
// resets with /new, Claude with /clear, then the flow's skill. It is nil for a
// session that runs no flow.
func ContinueAction(ref FlowRef, agent string) []string {
	spec, err := tickets.FlowSpecFor(ref.Flow)
	if err != nil {
		return nil
	}
	reset := "/clear"
	if agent == string(harness.Codex) {
		reset = "/new"
	}
	subject := ref.Key
	if ref.Flow == tickets.FlowDesign {
		subject = TicketEpic(ref.Key)
	}
	return []string{reset, harness.SkillCommand(agent, spec.Skill, spec.ContinueArg(subject))}
}

// TicketEpic is the epic of a ticket address like "project:epic" or
// "project:epic/03".
func TicketEpic(addr string) string {
	_, rest, _ := strings.Cut(addr, ":")
	epic, _, _ := strings.Cut(rest, "/")
	return epic
}

// AcceptsKeys reports whether a session in state can be typed into.
func AcceptsKeys(state string) bool {
	return state == wire.SessionWorking || state == wire.SessionIdle || state == wire.SessionBlocked || state == wire.SessionRateLimited
}
