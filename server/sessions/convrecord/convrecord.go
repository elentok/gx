// Package convrecord keeps a session's conversation on the conversation record:
// the record itself, its token and cost totals, and its Grilling round counts.
package convrecord

import (
	"log/slog"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/conversations"
	"github.com/elentok/gx/grillround"
	"github.com/elentok/gx/ralphloop/agent"
	"github.com/elentok/gx/ralphloop/metrics"
)

// Host is what the Recorder needs from the root: the harness homes and whether
// a ticket is on a map epic.
type Host interface {
	EnvHome() (string, error)
	CodexHome() (string, error)
	OnMapEpic(address string) bool
}

// Session is the part of a session's meta that a conversation record needs.
type Session struct {
	ID             string
	Dir            string
	Harness        string
	Ticket         string
	ConversationID string
	Design         bool
}

// Recorder writes conversation records under Dir.
type Recorder struct {
	Dir  string
	Log  *slog.Logger
	Host Host
}

// Record keeps the session's current conversation on the record, counted toward
// its current Session ticket. A session with no ticket records nothing.
func (r *Recorder) Record(s Session) {
	if s.Ticket == "" || s.ConversationID == "" {
		return
	}
	err := conversations.Save(r.Dir, conversations.Record{
		ID: s.ConversationID, Dir: s.Dir, Harness: s.Harness, Ticket: s.Ticket, Design: s.Design, RecordedAt: time.Now(),
	})
	if err != nil {
		r.Log.Warn("record conversation", "session", s.ID, "err", err)
	}
}

// SaveTotals saves the tokens and cost of the ended conversation on its record,
// so stamps never re-read a transcript that may have aged out. An unreadable
// transcript leaves the record without totals.
func (r *Recorder) SaveTotals(s Session, conversation string) {
	if s.Ticket == "" || conversation == "" {
		return
	}
	agent := agent.AgentFor(s.Harness)
	home, err := r.Host.EnvHome()
	if err != nil {
		r.Log.Warn("read conversation totals", "session", s.ID, "conversation", conversation, "err", err)
		return
	}
	codexHome, _ := r.Host.CodexHome()
	tokens, cost, err := metrics.SessionsTotals(metrics.SessionHomes{Claude: home, Codex: codexHome}, []metrics.SessionRef{{Agent: agent, Cwd: s.Dir, SessionID: conversation}})
	if err != nil {
		r.Log.Warn("read conversation totals", "session", s.ID, "conversation", conversation, "err", err)
		return
	}
	if err := conversations.SaveTotals(r.Dir, conversation, conversations.Totals{TotalTokens: int(tokens), Cost: cost}); err != nil {
		r.Log.Warn("save conversation totals", "session", s.ID, "conversation", conversation, "err", err)
	}
	if s.Design {
		r.saveGrilling(s, conversation)
	}
}

// homes are the harness homes a conversation is found under. The Codex home may
// be unset; a Codex lookup then finds nothing.
func (r *Recorder) homes() (grillround.Homes, error) {
	claude, err := r.Host.EnvHome()
	if err != nil {
		return grillround.Homes{}, err
	}
	codex, _ := r.Host.CodexHome()
	return grillround.Homes{Claude: claude, Codex: codex}, nil
}

// recount counts the Grilling rounds of a conversation of any harness onto its
// record.
func (r *Recorder) recount(kind agentrunner.Kind, conversation, dir string) error {
	homes, err := r.homes()
	if err != nil {
		return err
	}
	return grillround.Recount(r.Dir, homes, kind, conversation, dir)
}

// saveGrilling counts the Grilling rounds of the ended Design conversation onto
// its record. Any non-Codex harness reads as Claude.
func (r *Recorder) saveGrilling(s Session, conversation string) {
	if err := r.recount(agentrunner.Kind(s.Harness), conversation, s.Dir); err != nil {
		r.Log.Warn("count conversation grilling", "session", s.ID, "conversation", conversation, "err", err)
	}
}

// SweepDesignCounts counts every Design conversation record, of either harness,
// that has no counts yet, so a session that never ended cleanly (a crash, or an
// exit while the server was down) still gets them. Records saved before the
// Design flag existed are Design when their ticket is on a map epic. A record
// whose transcript or rollout is missing stays uncounted; a later end recounts
// and replaces.
func (r *Recorder) SweepDesignCounts() {
	recs, err := conversations.List(r.Dir)
	if err != nil {
		r.Log.Warn("sweep conversation counts", "err", err)
		return
	}
	for _, rec := range recs {
		if rec.Grilling != nil {
			continue
		}
		if !rec.Design && !(rec.Ticket != "" && r.Host.OnMapEpic(rec.Ticket)) {
			continue
		}
		if err := r.recount(agentrunner.Kind(rec.Harness), rec.ID, rec.Dir); err != nil {
			r.Log.Warn("sweep conversation counts", "conversation", rec.ID, "err", err)
		}
	}
}
