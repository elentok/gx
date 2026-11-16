package skills

import (
	"strings"
	"testing"
)

func TestFilesURLNotes(t *testing.T) {
	for _, path := range []string{
		"gx-prototype/UI.md",
		"gx-prototype/SKILL.md",
		"gx-research/SKILL.md",
		"gx-investigate/SKILL.md",
		"gx-design/SKILL.md",
	} {
		t.Run(path, func(t *testing.T) {
			checkAnchors(t, path, "gx files url")
		})
	}
}

// gx-design hands over a spec or mock in three places: the question round, step 4 and step 9.
func TestFilesURLNotesGxDesignHandovers(t *testing.T) {
	if n := strings.Count(readFile(t, "gx-design/SKILL.md"), "gx files url"); n < 3 {
		t.Errorf("gx-design/SKILL.md mentions gx files url %d times, want at least 3", n)
	}
}

// gx-ui-check serves its own fixture mock over http.server; the note does not apply there.
func TestFilesURLNotesAbsentFromUICheck(t *testing.T) {
	for _, path := range []string{"gx-ui-check/SKILL.md", "gx-ui-check/MOCK-CHECK.md"} {
		if strings.Contains(readFile(t, path), "gx files url") {
			t.Errorf("%s must not mention gx files url", path)
		}
	}
}
