package tickets

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// claimEventType is the run-log event written when a ticket is claimed.
const claimEventType = "iteration-started"

// Warning is a non-fatal finding about the ticket file at Path.
type Warning struct {
	Path string
	Text string
}

func (w Warning) String() string { return w.Path + ": " + w.Text }

// ProjectWarnings reports the base-derivation problems that are worth a look
// but never fail validation. Every ticket here is a leaf of its epic, so the
// derivation rules for roots (an epic's own blocked_by) are not checked.
// Terminal and commitless tickets are skipped: they derive no base.
func ProjectWarnings(projectDir string) ([]Warning, error) {
	epics, err := Load(projectDir)
	if err != nil {
		return nil, err
	}
	graph := newProjectGraph(ProjectName(projectDir), epics)
	var warnings []Warning
	for _, epic := range epics {
		lastClaim := lastClaimBases(filepath.Join(epic.Path, "run-log.jsonl"))
		for _, t := range epic.Tickets {
			if t.IsTerminal() {
				continue
			}
			warnings = append(warnings, graph.mentionWarnings(epic.Name, t)...)
			if t.Commitless {
				continue
			}
			warnings = append(warnings, graph.baseWarnings(epic.Name, t)...)
			if claimed, ok := lastClaim[t.Identifier]; ok && t.ResolvedBase != "" && claimed != t.ResolvedBase {
				warnings = append(warnings, Warning{t.Path, fmt.Sprintf("ticket %s: resolved_base %s differs from the last claim (%s)",
					t.DisplayNumber(), t.ResolvedBase, claimed)})
			}
		}
	}
	return warnings, nil
}

// baseWarnings flags the blocker shapes that leave a ticket without an
// unambiguous derived base.
func (g *projectGraph) baseWarnings(epic string, t Ticket) []Warning {
	var warnings []Warning
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
			warnings = append(warnings, Warning{t.Path, fmt.Sprintf("ticket %s: blocker %q is in another epic, so no base is derived",
				t.DisplayNumber(), ref)})
		}
	}
	if unlanded >= 2 && t.Base == "" {
		warnings = append(warnings, Warning{t.Path, fmt.Sprintf("ticket %s: %d unlanded commitful blockers and no base: set base: to pick one",
			t.DisplayNumber(), unlanded)})
	}
	return warnings
}

// mentionRe matches "ticket N" and "#N" in a ticket body; a bare N is too
// often not a ticket.
var mentionRe = regexp.MustCompile(`(?i)(?:\bticket\s+#?|#)(\d+[a-z]?)\b`)

// mentionWarnings flags live sibling tickets the body names that t does not
// wait on, directly or transitively (parent edges and fork subtrees included):
// the scheduler may then run t before the ticket it relies on lands.
func (g *projectGraph) mentionWarnings(epic string, t Ticket) []Warning {
	self := nodeKey(epic, t)
	var covered map[string]bool
	seen := map[string]bool{}
	var warnings []Warning
	for _, m := range mentionRe.FindAllStringSubmatch(t.Body, -1) {
		key, err := g.resolve(epic, m[1])
		if err != nil || key == self || seen[key] || g.tickets[key].IsTerminal() {
			continue
		}
		seen[key] = true
		if covered == nil {
			covered = g.waitClosure(self)
		}
		if !covered[key] {
			warnings = append(warnings, Warning{t.Path, fmt.Sprintf("ticket %s: body mentions ticket %s, which is not in blocked_by or parent",
				t.DisplayNumber(), m[1])})
		}
	}
	return warnings
}

// waitClosure is every node key reachable from start through g.wait.
func (g *projectGraph) waitClosure(start string) map[string]bool {
	reached := map[string]bool{}
	stack := []string{start}
	for len(stack) > 0 {
		key := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for _, next := range g.wait[key] {
			if !reached[next] {
				reached[next] = true
				stack = append(stack, next)
			}
		}
	}
	return reached
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
