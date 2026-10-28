package tickets

import (
	"bytes"
	"fmt"
	"os"

	"github.com/elentok/gx/fsutil"
	"github.com/elentok/gx/tickets/schema"
	"gopkg.in/yaml.v3"
)

// SiblingKey normalises a ticket token ("03", "3", "04A") so two spellings of
// the same sibling compare equal.
func SiblingKey(token string) string {
	return siblingKey(splitBlockedByToken(token))
}

// RewriteFrontmatterRefs returns raw with fn applied to every blocked_by entry
// and to base, and whether any value changed. Other keys, their order and the
// body are kept; raw is returned untouched when nothing changed.
func RewriteFrontmatterRefs(raw string, fn func(ref string) string) (string, bool, error) {
	fm, body, ok := schema.SplitFrontmatter(raw)
	if !ok {
		return raw, false, nil
	}
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(fm), &doc); err != nil {
		return raw, false, err
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return raw, false, nil
	}
	mapping := doc.Content[0]
	changed := false
	apply := func(n *yaml.Node) {
		if n.Kind != yaml.ScalarNode {
			return
		}
		if next := fn(n.Value); next != n.Value {
			n.Value, changed = next, true
		}
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		switch val := mapping.Content[i+1]; mapping.Content[i].Value {
		case "base":
			apply(val)
		case "blocked_by":
			for _, item := range val.Content {
				apply(item)
			}
		}
	}
	if !changed {
		return raw, false, nil
	}
	var out bytes.Buffer
	enc := yaml.NewEncoder(&out)
	enc.SetIndent(2)
	if err := enc.Encode(mapping); err != nil {
		return raw, false, err
	}
	return "---\n" + out.String() + "---\n" + body, true, nil
}

// RewriteRefsInFile applies RewriteFrontmatterRefs to the file at path, under
// its ticket lock, and reports whether it wrote.
func RewriteRefsInFile(path string, fn func(ref string) string) (bool, error) {
	unlock, err := schema.LockTicket(path)
	if err != nil {
		return false, err
	}
	defer unlock()
	raw, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	next, changed, err := RewriteFrontmatterRefs(string(raw), fn)
	if err != nil {
		return false, fmt.Errorf("rewriting refs in %s: %w", path, err)
	}
	if !changed {
		return false, nil
	}
	return true, fsutil.WriteFileAtomic(path, []byte(next))
}

// ValidateRefs is ValidateProject's cross-ticket half, run on epics already in
// memory (all of project): every epic's and every live ticket's blocked_by
// ref must resolve, and none may close a wait cycle. Done and
// cancelled tickets are skipped, as in ValidateProject. archived is the
// project's ArchivedEpicNames: a blocker in one of them is not dangling.
func ValidateRefs(project string, epics []Epic, archived map[string]bool) []error {
	g := newProjectGraph(project, epics, archived)
	var errs []error
	for _, e := range epics {
		errs = append(errs, g.epicBlockedByErrors(e)...)
		for _, t := range e.Tickets {
			if t.IsTerminal() {
				continue
			}
			for _, err := range append(g.checkBlockedBy(e.Name, t), g.cycleErrors(e.Name, t)...) {
				errs = append(errs, fmt.Errorf("epic %s: %w", e.Name, err))
			}
		}
	}
	return errs
}
