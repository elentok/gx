// Package ticketwrite holds the single-ticket writes: pausing a ticket and
// announcing the pause, the recovery hold, the model-mismatch pause and the
// retro stamp.
package ticketwrite

import (
	"fmt"
	"log/slog"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/server/broker"
	"github.com/elentok/gx/server/index"
	"github.com/elentok/gx/server/projects"
	"github.com/elentok/gx/server/runs"
	"github.com/elentok/gx/tickets"
)

// FollowUp is what a caller supplies to file a follow-up: the title and the
// text of each section, plus the key that makes two filings the same problem.
type FollowUp struct {
	Slug, Title, Source, Problem, SuggestedFix, Key string
}

// Host is what the writer needs from the root.
type Host interface {
	// HoldPauseForRecovery starts the pause's recovery and reports whether it
	// kept the pause's chat message back.
	HoldPauseForRecovery(addr tickets.Address, ticketPath string, kind events.Kind, reason string) bool
	// WriteFollowUp files the follow-up, or notes a repeat on the open one.
	WriteFollowUp(from tickets.Address, f FollowUp) (string, error)
	// FindTicket resolves a ticket address to its project dir and ticket file.
	FindTicket(address string) (projectDir, ticketPath string, ok bool, err error)
	// HerdrUnavailable reports whether herdr is down.
	HerdrUnavailable() bool
	// StopRun stops the run's session.
	StopRun(run runs.Run) error
	// TakeTierOverride returns the ticket's one-time tier and clears it.
	TakeTierOverride(ticketPath string) ralphloop.Tier
}

// Deps is what New needs.
type Deps struct {
	Log      *slog.Logger
	Index    *index.Index
	Events   *broker.Broker
	Projects *projects.Registry
	Chat     *ralphloop.ServerChat
	Host     Host
}

// Writer makes the single-ticket writes.
type Writer struct {
	d    Deps
	fold PauseFold
}

// New returns a Writer. The pause fold's cap flushes the digest to chat.
func New(d Deps) *Writer {
	w := &Writer{d: d}
	w.fold.OnCap = w.FlushPauseFold
	return w
}

// FlushPauseFold sends the held herdr-outage pauses as one digest.
func (w *Writer) FlushPauseFold() {
	w.d.Chat.PauseDigest(w.fold.Flush())
}

// holdPause queues the pause line for the digest.
func (w *Writer) holdPause(addr tickets.Address, status string, kind events.Kind, reason string) {
	w.fold.Hold(fmt.Sprintf("[%s] %s/%s — %s (%s): %s", addr.Project, addr.Epic, addr.ID, status, kind, reason))
}
