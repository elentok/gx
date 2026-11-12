package ticketspec

import (
	"os"
	"path/filepath"
	"slices"

	"gopkg.in/yaml.v3"
)

// DropEntries removes every manifest entry that match selects, deletes the
// dropped entries' body files, and replaces each dropped slug in the other
// entries' blocked_by with the dropped entry's own blockers. It edits the YAML
// node tree, so comments on the kept entries survive. It returns the dropped
// slugs; none dropped leaves the manifest untouched.
func DropEntries(manifestPath string, match func(ManifestEntry) bool) ([]string, error) {
	entries, err := LoadEntries(manifestPath)
	if err != nil {
		return nil, err
	}
	dropped := map[string]ManifestEntry{}
	var slugs []string
	for _, e := range entries {
		if match(e) {
			dropped[e.Slug] = e
			slugs = append(slugs, e.Slug)
		}
	}
	if len(slugs) == 0 {
		return nil, nil
	}

	// replace expands a dropped slug into its blockers, recursively, so a
	// chain of dropped entries closes over the whole gap.
	var replace func(slug string) []string
	replace = func(slug string) []string {
		d, ok := dropped[slug]
		if !ok {
			return []string{slug}
		}
		var out []string
		for _, b := range d.BlockedBy {
			out = append(out, replace(b)...)
		}
		return out
	}

	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, err
	}
	out, _, err := dropRows(data, func(slug, _ string) bool { _, ok := dropped[slug]; return ok })
	if err != nil {
		return nil, err
	}
	if out == nil {
		out = []byte("[]\n")
	} else if out, err = rewriteBlockedBy(out, replace); err != nil {
		return nil, err
	}
	if err := os.WriteFile(manifestPath, out, 0o644); err != nil {
		return nil, err
	}
	dir := filepath.Dir(manifestPath)
	for _, s := range slugs {
		if f := dropped[s].BodyFile; f != "" {
			if err := os.Remove(filepath.Join(dir, f)); err != nil && !os.IsNotExist(err) {
				return nil, err
			}
		}
	}
	return slugs, nil
}

// rewriteBlockedBy replaces every blocked_by slug of manifest with replace(slug),
// without duplicates, leaving the rest of the document as it is.
func rewriteBlockedBy(manifest []byte, replace func(slug string) []string) ([]byte, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(manifest, &doc); err != nil {
		return nil, err
	}
	if len(doc.Content) == 0 || doc.Content[0].Kind != yaml.SequenceNode {
		return manifest, nil
	}
	for _, item := range doc.Content[0].Content {
		if item.Kind != yaml.MappingNode {
			continue
		}
		for i := 0; i+1 < len(item.Content); i += 2 {
			list := item.Content[i+1]
			if item.Content[i].Value != "blocked_by" || list.Kind != yaml.SequenceNode {
				continue
			}
			var blocked []string
			for _, b := range list.Content {
				for _, r := range replace(b.Value) {
					if !slices.Contains(blocked, r) {
						blocked = append(blocked, r)
					}
				}
			}
			if len(blocked) == 0 {
				item.Content = slices.Delete(item.Content, i, i+2)
				break
			}
			kept := make([]*yaml.Node, 0, len(blocked))
			for _, s := range blocked {
				kept = append(kept, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: s})
			}
			list.Content = kept
		}
	}
	return yaml.Marshal(&doc)
}
