package server

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/herdr"
	"github.com/elentok/gx/ralphloop"
)

// EventTicketNudged is emitted after text was typed into an iteration's pane.
const EventTicketNudged = "ticket-nudged"

// Refusal reasons of the nudge verb.
const (
	ReasonTextRequired     = "text-required"
	ReasonIterationNotLive = "iteration-not-live"
	ReasonRateLimited      = "rate-limited"
)

const (
	nudgeLimit  = 3
	nudgeWindow = time.Minute
)

// nudgeLimiter allows nudgeLimit nudges per iteration in any nudgeWindow, so a
// looping caller cannot bury a pane in text. The zero value is ready to use.
type nudgeLimiter struct {
	mu   sync.Mutex
	sent map[string][]time.Time
}

// allow records a nudge for address and reports whether it is within the limit.
func (l *nudgeLimiter) allow(address string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.sent == nil {
		l.sent = map[string][]time.Time{}
	}
	recent := l.sent[address][:0]
	for _, t := range l.sent[address] {
		if now.Sub(t) < nudgeWindow {
			recent = append(recent, t)
		}
	}
	if len(recent) >= nudgeLimit {
		l.sent[address] = recent
		return false
	}
	l.sent[address] = append(recent, now)
	return true
}

// ticketNudge types req.Text into the live pane of a ticket's iteration. It is
// the only way to nudge a pane: every nudge is rate-limited, written to the
// epic's run log and streamed.
func (s *Server) ticketNudge(req QueueRequest) (QueueResult, error) {
	text := strings.TrimSpace(req.Text)
	if text == "" {
		return refusal(ReasonTextRequired, "text to type into the pane is required"), nil
	}
	return s.resolvedWrite(req, func(ref ticketRef) (QueueResult, error) {
		address := ref.addr.String()
		if s.herdr.isUnavailable() {
			return refusal(ReasonHerdrUnavailable, "herdr is unavailable"), nil
		}
		it, live := s.liveIteration(address)
		if !live {
			return refusal(ReasonIterationNotLive, "no live iteration for "+address), nil
		}
		now := time.Now()
		if !s.nudges.allow(address, now) {
			return refusal(ReasonRateLimited, fmt.Sprintf("more than %d nudges to %s within %s", nudgeLimit, address, nudgeWindow)), nil
		}
		label, _, _ := ralphloop.IterationIdentity(ref.addr.Epic, ref.addr.ID, "")
		if _, err := herdr.AgentPrompt(herdr.AgentPromptOptions{Target: label, Text: text}); err != nil {
			return QueueResult{}, fmt.Errorf("nudge %s: %w", address, err)
		}
		err := ralphloop.AppendEvent(ref.projectDir, ref.addr.Epic, ralphloop.Event{
			Time: now, Type: string(events.Nudged), Ticket: ref.addr.ID, Address: address,
			Pane: it.Pane, Text: text,
		})
		if err != nil {
			return QueueResult{}, fmt.Errorf("logging nudge of %s: %w", address, err)
		}
		s.events.publish(EventTicketNudged, address)
		return QueueResult{}, nil
	})
}
