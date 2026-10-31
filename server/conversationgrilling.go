package server

import (
	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/grillround"
)

// grillingHomes are the harness homes a conversation is found under. The Codex
// home may be unset; a Codex lookup then finds nothing.
func (s *Server) grillingHomes() (grillround.Homes, error) {
	claude, err := s.envHome()
	if err != nil {
		return grillround.Homes{}, err
	}
	codex, _ := s.codexHome()
	return grillround.Homes{Claude: claude, Codex: codex}, nil
}

// recountGrilling counts the Grilling rounds of a conversation of any harness
// onto its record.
func (s *Server) recountGrilling(kind agentrunner.Kind, conversation, dir string) error {
	homes, err := s.grillingHomes()
	if err != nil {
		return err
	}
	return grillround.Recount(s.cfg.ConversationsDir, homes, kind, conversation, dir)
}

// saveConversationGrilling counts the Grilling rounds of the ended Design
// conversation onto its record. Any non-Codex harness reads as Claude.
func (s *Server) saveConversationGrilling(m sessionMeta, conversation string) {
	if err := s.recountGrilling(agentrunner.Kind(m.Harness), conversation, m.Dir); err != nil {
		s.log.Warn("count conversation grilling", "session", m.ID, "conversation", conversation, "err", err)
	}
}
