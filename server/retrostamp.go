package server

import (
	"path/filepath"
	"strings"
	"time"

	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// stampFinished is the one hook every done and cancel path calls once the
// ticket's status is written.
func (s *Server) stampFinished(addr tickets.Address, ticketPath string, cancelled bool) {
	s.stampFriction(addr, ticketPath)
	s.stampRetro(addr, ticketPath, cancelled)
}

// stampFriction writes the ticket's Friction summary from its own log and the
// epic's run log. A missing log counts as empty.
func (s *Server) stampFriction(addr tickets.Address, ticketPath string) {
	dir, err := s.projectDir(addr.Project)
	if err != nil {
		s.log.Warn("stamp friction", "ticket", addr.String(), "err", err)
		return
	}
	runEvents, _, err := ralphloop.ReadEvents(dir, addr.Epic)
	if err != nil {
		s.log.Warn("stamp friction", "ticket", addr.String(), "err", err)
		return
	}
	log, err := tickets.ReadLog(ticketPath)
	if err != nil {
		s.log.Warn("stamp friction", "ticket", addr.String(), "err", err)
		return
	}
	// Replaces any earlier summary, so a ticket done again after a new claim gets a fresh one.
	friction := ralphloop.Friction(addr.ID, runEvents, log)
	if err := schema.UpdateTicket(ticketPath, func(t *schema.Ticket) { t.Friction = &friction }); err != nil {
		s.log.Warn("stamp friction", "ticket", addr.String(), "err", err)
	}
}

// retroKeyPrefix is how RetroSubmit's dedupe key names the epic a retro covers.
const retroKeyPrefix = "evolve:"

// stampRetro records on the retro'd epic what became of its evolve one-off: the
// server stamps, never the skill. A done retro (zero findings included) sets
// evolve_ticket, overwriting a --force re-run's predecessor; a cancelled one
// sets evolve_skipped so `gx evolve all` stops offering the epic. Both set
// evolved_at. Tickets that are not retros are ignored.
func (s *Server) stampRetro(addr tickets.Address, ticketPath string, cancelled bool) {
	tk, err := schema.ParseTicket(ticketPath)
	if err != nil || tk.Type != schema.TypeEvolve {
		return
	}
	target, ok := strings.CutPrefix(tk.Unique, retroKeyPrefix)
	if !ok {
		return
	}
	project, epic, ok := strings.Cut(target, "/")
	if !ok {
		return
	}
	dir, err := s.projectDir(project)
	if err != nil {
		s.log.Warn("stamp retro", "ticket", addr.String(), "err", err)
		return
	}
	epicPath := filepath.Join(dir, epic)
	now := time.Now().UTC()
	if cancelled {
		// A cancel after a finished retro keeps the stamp that retro wrote.
		err = tickets.StampEpicFields(epicPath, map[string]any{"evolved_at": now, "evolve_skipped": true})
	} else {
		err = tickets.OverwriteEpicFields(epicPath, map[string]any{"evolved_at": now, "evolve_ticket": addr.String()})
	}
	if err != nil {
		s.log.Warn("stamp retro", "ticket", addr.String(), "err", err)
	}
}
