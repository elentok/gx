package ralphloop

import (
	"fmt"
	"strings"
	"time"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/tickets/schema"
)

// ModelMismatchError is an implement session whose first assistant turn ran on
// a model other than the one its launch resolved. The wait returns it so the
// run's result handling can stop the session and pause the ticket; the branch
// and its commits stay.
type ModelMismatchError struct {
	Ran, Expected string
}

func (e *ModelMismatchError) Error() string {
	return fmt.Sprintf("session ran on %s, expected %s", e.Ran, e.Expected)
}

// modelFamilies are the Claude tiers a model name is matched on: an alias
// ("sonnet") and a full id ("claude-sonnet-5-5") share one.
var modelFamilies = []string{"opus", "sonnet", "haiku"}

// ModelMatches reports whether ran is the model expected names. Claude names
// match by family, so the alias a tier resolves to equals any version of it.
// Any other name (Codex) matches when either contains the other.
func ModelMatches(ran, expected string) bool {
	ran, expected = strings.ToLower(ran), strings.ToLower(expected)
	for _, f := range modelFamilies {
		if strings.Contains(expected, f) {
			return strings.Contains(ran, f)
		}
	}
	return strings.Contains(ran, expected) || strings.Contains(expected, ran)
}

// modelGuard checks the model of one launch's first assistant turn, once.
type modelGuard struct {
	expected string
	since    time.Time
	done     bool
}

// newModelGuard returns nil when the launch gets no check: a ticket that is not
// an implement ticket, or a reader the wiring lacks. An empty expected model
// (the tier resolved to none) is logged once and skipped.
func newModelGuard(d Deps, p launchAndPromptParams, since time.Time) *modelGuard {
	if d.ReadTurns == nil || !isImplementTicket(p) {
		return nil
	}
	if p.Model == "" {
		p.logAgentEvent(string(events.ModelMismatch), "", "model check skipped: the tier resolves to no model")
		return nil
	}
	return &modelGuard{expected: p.Model, since: since}
}

// clockOf is d's clock, the wall clock when the wiring has none.
func clockOf(d Deps) time.Time {
	if d.Now != nil {
		return d.Now()
	}
	return time.Now()
}

func isImplementTicket(p launchAndPromptParams) bool {
	return IsImplementType(p.TicketData.Type)
}

// IsImplementType reports whether a ticket's type is an implement ticket; an
// unset type counts as one.
func IsImplementType(ticketType string) bool {
	return ticketType == "" || ticketType == string(schema.TypeImplement)
}

// tick compares the launch's first assistant turn with the expected model. It
// returns a *ModelMismatchError the first time they differ; once a turn has
// been read, whatever the verdict, it never reads again. A nil receiver checks
// nothing, and a read that fails or finds no turn yet tries again next tick.
func (g *modelGuard) tick(d Deps, p launchAndPromptParams, sessionID string) error {
	if g == nil || g.done || sessionID == "" {
		return nil
	}
	turns, ok, err := d.ReadTurns(p.SessionCwd, sessionID, g.since)
	if err != nil || !ok || turns.Model == "" {
		return nil
	}
	g.done = true
	if ModelMatches(turns.Model, g.expected) {
		return nil
	}
	p.logAgentEvent(string(events.ModelMismatch), sessionID, fmt.Sprintf("ran %s, expected %s", turns.Model, g.expected))
	return &ModelMismatchError{Ran: turns.Model, Expected: g.expected}
}
