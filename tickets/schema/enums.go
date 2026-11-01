package schema

import (
	"slices"

	"github.com/elentok/gx/events"
)

// Enums lists every closed value set of the ticket schema, sorted, keyed by
// the frontmatter field. The migration schema guard snapshots it: a change here
// needs a new migration step (see the migrations package).
func Enums() map[string][]string {
	out := map[string][]string{
		"status":           keys(validStatuses),
		"type":             keys(validTypes),
		"iteration_status": keys(validIterationStatuses),
		"category":         {string(CategoryBug), string(CategoryIdea)},
		"pause_reason":     keys(pauseClasses),
		"pause_kind":       events.KindNames(),
	}
	return out
}

func keys[K ~string, V any](m map[K]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, string(k))
	}
	slices.Sort(out)
	return out
}
