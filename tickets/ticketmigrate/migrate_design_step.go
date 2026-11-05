package ticketmigrate

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
	"gopkg.in/yaml.v3"
)

// DesignProbe is what the file-based design step guess reads outside the store:
// the project's trunk checkout and repo root. Either may be empty.
type DesignProbe struct {
	TrunkDir string
	Repo     string
	// BranchExists reports whether the repo has a local branch of that name.
	BranchExists func(repo, branch string) bool
}

// GuessDesignStep is the step a map has reached, judged from its tickets, body
// and files. It checks from the last step back and stops at the first step that
// is done, because the Branch step deletes the spec file from the trunk
// checkout. Only MigrateDesignStep uses it: a stored design_step replaces it.
func GuessDesignStep(m tickets.Epic, byName map[string]tickets.Epic, p DesignProbe) tickets.DesignStep {
	impl, hasImpl := byName[m.Name+tickets.ImplEpicSuffix]
	findings := schema.Section(m.Body, "Review findings")
	hasFindings := schema.HasSection(m.Body, "Review findings")
	open := tickets.HasOpenTicket(m)
	switch {
	case m.Status == string(schema.StatusDone),
		!m.CompletedAt.IsZero() && !hasImpl,
		p.Repo != "" && p.BranchExists != nil && p.BranchExists(p.Repo, m.Name+tickets.ImplEpicSuffix):
		return tickets.StepDone
	case hasImpl && impl.Status != string(schema.StatusDraft):
		return tickets.StepBranch
	case open && !m.DestinationUnconfirmed:
		// A review that sent tickets back reopens the map's tickets.
		return tickets.StepResolveTickets
	case hasFindings && findingsChecked(findings):
		return tickets.StepCreateTickets
	case hasFindings:
		return tickets.StepGrillFindings
	case p.TrunkDir != "" && fileExists(filepath.Join(p.TrunkDir, "docs", "specs", m.Name+".md")):
		return tickets.StepReview
	case !open && schema.Section(m.Body, "Not yet specified") == "" && !m.DestinationUnconfirmed:
		return tickets.StepSpec
	case !m.DestinationUnconfirmed:
		return tickets.StepResolveTickets
	}
	return tickets.StepChart
}

// findingsChecked reports whether every "- [ ]" or "- [x]" line of a findings
// section is checked.
func findingsChecked(section string) bool {
	for line := range strings.SplitSeq(section, "\n") {
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

// MigrateDesignStep writes design_step on every map epic under root that has
// none, from GuessDesignStep. Epics that already have one and loop epics are
// untouched, so a rerun is a no-op.
func MigrateDesignStep(root string, probe DesignProbe) ([]tickets.MigrationChange, error) {
	epics, err := tickets.Load(root)
	if err != nil {
		return nil, err
	}
	byName := make(map[string]tickets.Epic, len(epics))
	for _, e := range epics {
		byName[e.Name] = e
	}
	var changes []tickets.MigrationChange
	for _, e := range epics {
		if !e.IsMap || e.DesignStep != "" {
			continue
		}
		step := GuessDesignStep(e, byName, probe)
		err := tickets.EditEpicFrontmatterInner(e.Path, func(mapping *yaml.Node) (bool, error) {
			if tickets.MappingKeyIndex(mapping, "design_step") >= 0 {
				return false, nil
			}
			mapping.Content = append(mapping.Content,
				&yaml.Node{Kind: yaml.ScalarNode, Value: "design_step"},
				&yaml.Node{Kind: yaml.ScalarNode, Value: string(step)})
			return true, nil
		})
		if err != nil {
			return changes, err
		}
		changes = append(changes, tickets.MigrationChange{
			Path:  filepath.Join(e.Path, "ticket.md"),
			Notes: []string{"design_step set to " + string(step)},
		})
	}
	return changes, nil
}
