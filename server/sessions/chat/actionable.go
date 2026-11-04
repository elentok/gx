package chat

import (
	"strings"

	"github.com/elentok/gx/grillround"
	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/server/wire"
)

// RunEndNext is the Actionable a gx-design run-end line raises: Continue in a
// fresh conversation, or just the line when the design is at its final step.
func RunEndNext(text, ticket, agent string, final bool) (wire.SessionActionable, bool) {
	line := grillround.RunEndLine.FindString(text)
	if line == "" {
		return wire.SessionActionable{}, false
	}
	a := wire.SessionActionable{Reason: strings.TrimSpace(line), Derived: true}
	if !final {
		a.Action = DesignContinueAction(ticket, agent)
	}
	return a, true
}

// DesignContinueAction starts the next gx-design phase in a fresh conversation:
// Codex resets with /new, Claude with /clear.
func DesignContinueAction(ticket, agent string) []string {
	reset := "/clear"
	if agent == string(harness.Codex) {
		reset = "/new"
	}
	return []string{reset, harness.SkillCommand(agent, "gx-design", TicketEpic(ticket))}
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
