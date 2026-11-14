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

// gx-ui-check serves its own fixture mock over http.server; the note does not apply there.
func TestFilesURLNotesAbsentFromUICheck(t *testing.T) {
	for _, path := range []string{"gx-ui-check/SKILL.md", "gx-ui-check/MOCK-CHECK.md"} {
		if strings.Contains(readFile(t, path), "gx files url") {
			t.Errorf("%s must not mention gx files url", path)
		}
	}
}
