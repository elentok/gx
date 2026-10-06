package tickets

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// claimEventType is the run-log event written when a ticket is claimed.
const claimEventType = "iteration-started"

// ProjectWarnings reports the base-derivation problems that are worth a look
// but never fail validation. Every ticket here is a leaf of its epic, so the
// derivation rules for roots (an epic's own blocked_by) are not checked.
// Terminal and commitless tickets are skipped: they derive no base.
func ProjectWarnings(projectDir string) ([]string, error) {
	epics, err := Load(projectDir)
	if err != nil {
		return nil, err
	}
	graph := newProjectGraph(ProjectName(projectDir), epics)
	var warnings []string
	for _, epic := range epics {
		lastClaim := lastClaimBases(filepath.Join(epic.Path, "run-log.jsonl"))
		for _, t := range epic.Tickets {
			if t.IsTerminal() || t.Commitless {
				continue
			}
			warnings = append(warnings, graph.baseWarnings(epic.Name, t)...)
			if claimed, ok := lastClaim[t.Identifier]; ok && t.ResolvedBase != "" && claimed != t.ResolvedBase {
				warnings = append(warnings, fmt.Sprintf("%s: ticket %s: resolved_base %s differs from the last claim (%s)",
					t.Path, t.DisplayNumber(), t.ResolvedBase, claimed))
			}
		}
	}
	return warnings, nil
}

// baseWarnings flags the blocker shapes that leave a ticket without an
// unambiguous derived base.
func (g *projectGraph) baseWarnings(epic string, t Ticket) []string {
	var warnings []string
	unlanded := 0
	for _, ref := range t.BlockedBy {
		key, err := g.resolve(epic, ref)
		if err != nil {
			continue // reported as an error by ValidateProject
		}
		blocker := g.tickets[key]
		if blocker.IsTerminal() || blocker.Commitless {
			continue
		}
		unlanded++
		if !strings.HasPrefix(key, epic+"/") {
			warnings = append(warnings, fmt.Sprintf("%s: ticket %s: blocker %q is in another epic, so no base is derived",
				t.Path, t.DisplayNumber(), ref))
		}
	}
	if unlanded >= 2 && t.Base == "" {
		warnings = append(warnings, fmt.Sprintf("%s: ticket %s: %d unlanded commitful blockers and no base: set base: to pick one",
			t.Path, t.DisplayNumber(), unlanded))
	}
	return warnings
}

// lastClaimBases maps each ticket in the run log to the resolved base of its
// latest claim event. A missing or unreadable log has no claims.
func lastClaimBases(path string) map[string]string {
	bases := map[string]string{}
	f, err := os.Open(path)
	if err != nil {
		return bases
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(nil, 1<<20)
	for sc.Scan() {
		var ev struct {
			Type         string `json:"type"`
			Ticket       string `json:"ticket"`
			ResolvedBase string `json:"resolved_base"`
		}
		if json.Unmarshal(sc.Bytes(), &ev) != nil || ev.Type != claimEventType || ev.ResolvedBase == "" {
			continue
		}
		bases[ev.Ticket] = ev.ResolvedBase
	}
	_ = sc.Err() // a torn log only means fewer claims to compare against
	return bases
}
