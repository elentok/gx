package triage

import (
	"os"
	"path/filepath"
	"slices"

	"github.com/elentok/gx/tickets/ticketspec"
)

// DropRows removes every manifest entry whose from: names a listed row (a group
// goes whole), deletes the dropped entries' body files, and closes each gap in
// the other entries' blocked_by with the dropped entry's own blockers. It
// returns the dropped slugs; none dropped leaves the manifest untouched.
func DropRows(manifestPath string, rows []string) ([]string, error) {
	entries, err := ticketspec.LoadEntries(manifestPath)
	if err != nil {
		return nil, err
	}
	dropped := map[string]ticketspec.ManifestEntry{}
	var slugs []string
	for _, e := range entries {
		if slices.ContainsFunc(rows, e.From.Has) {
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

	var kept []ticketspec.ManifestEntry
	for _, e := range entries {
		if _, ok := dropped[e.Slug]; ok {
			continue
		}
		var blocked []string
		for _, b := range e.BlockedBy {
			for _, r := range replace(b) {
				if !slices.Contains(blocked, r) {
					blocked = append(blocked, r)
				}
			}
		}
		e.BlockedBy = blocked
		kept = append(kept, e)
	}
	if err := ticketspec.SaveEntries(manifestPath, kept); err != nil {
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
