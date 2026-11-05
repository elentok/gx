package ticketmove

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"github.com/elentok/gx/fsutil"
	"github.com/elentok/gx/projectlock"
	"github.com/elentok/gx/tickets/schema"
	"gopkg.in/yaml.v3"
)

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
// the project's shared migration lock and its ticket lock, and reports whether
// it wrote. path is a ticket or an epic's ticket.md.
func RewriteRefsInFile(path string, fn func(ref string) string) (bool, error) {
	var unlockProject func()
	var err error
	if filepath.Base(path) == "ticket.md" {
		unlockProject, err = projectlock.SharedEpic(filepath.Dir(path))
	} else {
		unlockProject, err = projectlock.Shared(path)
	}
	if err != nil {
		return false, err
	}
	defer unlockProject()
	return rewriteRefsInFileInner(path, fn)
}

// rewriteRefsInFileInner is RewriteRefsInFile without the migration lock, for
// callers that already hold it (ApplyMove, ResumeMove).
func rewriteRefsInFileInner(path string, fn func(ref string) string) (bool, error) {
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
