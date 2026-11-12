package triage

import (
	"slices"

	"github.com/elentok/gx/tickets/ticketspec"
)

// DropRows removes every manifest entry whose from: names a listed row (a group
// goes whole), deletes the dropped entries' body files, and closes each gap in
// the other entries' blocked_by with the dropped entry's own blockers. It
// returns the dropped slugs; none dropped leaves the manifest untouched.
//
// canon turns an address into its canonical form (nil leaves addresses as
// written), so a short `from:` matches a full `--row` address.
func DropRows(manifestPath string, rows []string, canon func(string) string) ([]string, error) {
	if canon == nil {
		canon = func(a string) string { return a }
	}
	want := make([]string, len(rows))
	for i, r := range rows {
		want[i] = canon(r)
	}
	return ticketspec.DropEntries(manifestPath, func(e ticketspec.ManifestEntry) bool {
		return slices.ContainsFunc(e.From, func(a string) bool { return slices.Contains(want, canon(a)) })
	})
}
