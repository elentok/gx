package sessions

import (
	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/grillround"
)

// grillingHomes are the harness homes a conversation is found under. The Codex
// home may be unset; a Codex lookup then finds nothing.
func (mg *Manager) grillingHomes() (grillround.Homes, error) {
	claude, err := mg.projects.EnvHome()
	if err != nil {
		return grillround.Homes{}, err
	}
	codex, _ := mg.projects.CodexHome()
	return grillround.Homes{Claude: claude, Codex: codex}, nil
}

// recountGrilling counts the Grilling rounds of a conversation of any harness
// onto its record.
func (mg *Manager) recountGrilling(kind agentrunner.Kind, conversation, dir string) error {
	homes, err := mg.grillingHomes()
	if err != nil {
		return err
	}
	return grillround.Recount(mg.cfg.ConversationsDir, homes, kind, conversation, dir)
}

// saveConversationGrilling counts the Grilling rounds of the ended Design
// conversation onto its record. Any non-Codex harness reads as Claude.
func (mg *Manager) saveConversationGrilling(m Meta, conversation string) {
	if err := mg.recountGrilling(agentrunner.Kind(m.Harness), conversation, m.Dir); err != nil {
		mg.log.Warn("count conversation grilling", "session", m.ID, "conversation", conversation, "err", err)
	}
}
