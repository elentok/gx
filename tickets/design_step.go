package tickets

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/elentok/gx/tickets/schema"
	"gopkg.in/yaml.v3"
)

// DesignSteps are the values of a map epic's design_step, in order. They match
// the Progress step wire values the server sends.
var DesignSteps = []string{"chart", "resolve-tickets", "spec", "review", "grill-findings", "create-tickets", "branch", "done"}

// CheckDesignStep refuses a name that is not one of DesignSteps.
func CheckDesignStep(step string) error {
	if !slices.Contains(DesignSteps, step) {
		return fmt.Errorf("design step %q; want one of %s", step, strings.Join(DesignSteps, ", "))
	}
	return nil
}

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
func GuessDesignStep(m Epic, byName map[string]Epic, p DesignProbe) string {
	impl, hasImpl := byName[m.Name+ImplEpicSuffix]
	findings := schema.Section(m.Body, "Review findings")
	hasFindings := schema.HasSection(m.Body, "Review findings")
	open := hasOpenTicket(m)
	switch {
	case m.Status == string(schema.StatusDone),
		!m.CompletedAt.IsZero() && !hasImpl,
		p.Repo != "" && p.BranchExists != nil && p.BranchExists(p.Repo, m.Name+ImplEpicSuffix):
		return "done"
	case hasImpl && impl.Status != string(schema.StatusDraft):
		return "branch"
	case open && !m.DestinationUnconfirmed:
		// A review that sent tickets back reopens the map's tickets.
		return "resolve-tickets"
	case hasFindings && findingsChecked(findings):
		return "create-tickets"
	case hasFindings:
		return "grill-findings"
	case p.TrunkDir != "" && fileExists(filepath.Join(p.TrunkDir, "docs", "specs", m.Name+".md")):
		return "review"
	case !open && schema.Section(m.Body, "Not yet specified") == "" && !m.DestinationUnconfirmed:
		return "spec"
	case !m.DestinationUnconfirmed:
		return "resolve-tickets"
	}
	return "chart"
}

// hasOpenTicket reports whether any of m's tickets is open or claimed.
func hasOpenTicket(m Epic) bool {
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
func MigrateDesignStep(root string, probe DesignProbe) ([]MigrationChange, error) {
	epics, err := Load(root)
	if err != nil {
		return nil, err
	}
	byName := make(map[string]Epic, len(epics))
	for _, e := range epics {
		byName[e.Name] = e
	}
	var changes []MigrationChange
	for _, e := range epics {
		if !e.IsMap || e.DesignStep != "" {
			continue
		}
		step := GuessDesignStep(e, byName, probe)
		err := editEpicFrontmatter(e.Path, func(mapping *yaml.Node) (bool, error) {
			if mappingKeyIndex(mapping, "design_step") >= 0 {
				return false, nil
			}
			return true, setMappingValue(mapping, "design_step", step)
		})
		if err != nil {
			return changes, err
		}
		changes = append(changes, MigrationChange{
			Path:  filepath.Join(e.Path, "ticket.md"),
			Notes: []string{"design_step set to " + step},
		})
	}
	return changes, nil
}
