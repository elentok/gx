package skills

import (
	"strings"
	"testing"
)

// TestTrackerDocumentsEpicBlockedBy pins the structural anchors of the
// epic-level blocked_by section, so the tracker doc keeps telling the Waiting
// story.
func TestTrackerDocumentsEpicBlockedBy(t *testing.T) {
	raw := collapseSpace(readFile(t, "gx-local-tracker.md"))
	for _, want := range []string{
		"### Epic-level blocked_by",
		"gx tickets epic set",
		"--blocked-by",
		"Waiting",
		"names only tickets of its own epic",
	} {
		if !strings.Contains(raw, want) {
			t.Errorf("gx-local-tracker.md does not contain %q", want)
		}
	}
}
