package server

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// mapProbe is what a project's maps share: its trunk checkout and repo root.
// The listing resolves it once per project, not once per map.
type mapProbe struct {
	trunkDir string
	repo     string
}

func (s *Server) mapProbeFor(project string) mapProbe {
	return mapProbe{
		trunkDir: tickets.TrunkCheckout(s.cfg.TicketStore, project),
		repo:     tickets.ProjectRepoRoot(s.cfg.TicketStore, project),
	}
}

// mapProgress is a map's current Progress step and its phase. It checks from
// the last step back and stops at the first step that is done, because the
// Branch step deletes the spec file from the trunk checkout. It reads only the
// trunk checkout and the repo's refs, never a worktree.
func mapProgress(m tickets.Epic, byName map[string]tickets.Epic, p mapProbe) (ProgressStep, ProgressPhase) {
	impl, hasImpl := byName[m.Name+tickets.ImplEpicSuffix]
	switch {
	case m.Status == string(schema.StatusDone),
		!m.CompletedAt.IsZero() && !hasImpl,
		p.repo != "" && tickets.BranchExists(p.repo, m.Name+tickets.ImplEpicSuffix):
		return ProgressDone, ProgressPhaseImplement
	case hasImpl && impl.Status != string(schema.StatusDraft):
		return ProgressBranch, ProgressPhaseReview
	case schema.HasSection(m.Body, "Review findings") && findingsChecked(schema.Section(m.Body, "Review findings")):
		return ProgressCreateTickets, ProgressPhaseReview
	case schema.HasSection(m.Body, "Review findings"):
		return ProgressGrillFindings, ProgressPhaseReview
	case p.trunkDir != "" && fileExists(filepath.Join(p.trunkDir, "docs", "specs", m.Name+".md")):
		return ProgressReview, ProgressPhaseReview
	case !hasOpenTicket(m) && schema.Section(m.Body, "Not yet specified") == "" && !m.DestinationUnconfirmed:
		return ProgressSpec, ProgressPhaseReview
	case !m.DestinationUnconfirmed:
		return ProgressResolveTickets, ProgressPhaseMap
	}
	return ProgressChart, ProgressPhaseMap
}

// hasOpenTicket reports whether any of m's tickets is open or claimed.
func hasOpenTicket(m tickets.Epic) bool {
	for _, t := range m.Tickets {
		if t.Status == string(schema.StatusOpen) || t.Status == string(schema.StatusClaimed) {
			return true
		}
	}
	return false
}

// findingsChecked reports whether every "- [ ]" or "- [x]" line of a findings
// section is checked.
func findingsChecked(section string) bool {
	for _, line := range strings.Split(section, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "- [ ]") {
			return false
		}
	}
	return true
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
