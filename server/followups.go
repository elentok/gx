package server

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// DefaultFollowUps is the epic that receives follow-up drafts when
// recovery.follow-ups is unset.
const DefaultFollowUps = "gx:follow-ups"

// Proposal classes an investigate ticket's ## Result names. Only the first two
// are filed: human-only is already a person's job.
const (
	proposalCatalogEntry    = "catalog-entry"
	proposalOrchestratorFix = "orchestrator-fix"
	proposalHumanOnly       = "human-only"
)

var proposalClasses = []string{proposalCatalogEntry, proposalOrchestratorFix, proposalHumanOnly}

// proposalClass returns the one proposal class a ## Result names after its
// Proposal label. Zero or several distinct classes return "": a malformed
// report files nothing.
func proposalClass(result string) string {
	lower := strings.ToLower(result)
	at := strings.Index(lower, "proposal")
	if at < 0 {
		return ""
	}
	found := ""
	for _, c := range proposalClasses {
		if strings.Contains(lower[at:], c) {
			if found != "" {
				return ""
			}
			found = c
		}
	}
	return found
}

// fileFollowUp files a draft research ticket in the follow-ups epic for a landed
// investigate ticket whose report proposes a catalog entry or an orchestrator fix.
func (s *Server) fileFollowUp(addr tickets.Address, ticketPath string) {
	tk, err := schema.ParseTicket(ticketPath)
	if err != nil || tk.Type != schema.TypeInvestigate {
		return
	}
	raw, err := os.ReadFile(ticketPath)
	if err != nil {
		s.log.Warn("read investigate report", "ticket", addr.String(), "err", err)
		return
	}
	result := strings.TrimSpace(schema.Section(schema.ParseBody(string(raw)), "Result"))
	class := proposalClass(result)
	if class != proposalCatalogEntry && class != proposalOrchestratorFix {
		return
	}
	path, err := s.writeFollowUp(addr, class, result)
	if err != nil {
		s.log.Warn("file follow-up", "ticket", addr.String(), "err", err)
		return
	}
	s.log.Info("filed follow-up", "ticket", addr.String(), "path", path)
}

func (s *Server) writeFollowUp(from tickets.Address, class, result string) (string, error) {
	target := s.cfg.FollowUps
	if target == "" {
		target = DefaultFollowUps
	}
	project, epic, ok := strings.Cut(target, ":")
	if !ok {
		project, epic = from.Project, target
	}
	dir, err := s.projectDir(project)
	if err != nil {
		return "", err
	}
	epicPath := filepath.Join(dir, epic)
	if err := ensureDraftEpic(epicPath, epic); err != nil {
		return "", err
	}
	path, _, err := writeNewTicket(epicPath, "", class, func(id string) (schema.Ticket, string) {
		body := fmt.Sprintf(
			"\n# %s — Follow up on %s (%s)\n\n## What to build\n\nInvestigate %s proposed a %s. Its report:\n\n%s\n\n## Acceptance criteria\n\n- [ ] Proposal assessed and either built or dropped\n",
			id, from, class, from, class, result,
		)
		return schema.Ticket{ID: schema.TicketID(id), Status: schema.StatusDraft, Type: schema.TypeResearch}, body
	})
	return path, err
}

// ensureDraftEpic creates a missing epic with status draft, so nothing in it is
// scheduled until a person promotes it.
func ensureDraftEpic(epicPath, name string) error {
	if _, err := os.Stat(epicPath); err == nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Join(epicPath, "issues"), 0o755); err != nil {
		return err
	}
	body := fmt.Sprintf("---\nstatus: draft\n---\n\n# %s\n", name)
	return os.WriteFile(filepath.Join(epicPath, "ticket.md"), []byte(body), 0o644)
}
