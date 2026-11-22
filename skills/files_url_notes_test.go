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

	// Step 4: the note is the orchestrator's, on its own line after the bullets, not in the lead-in.
	text := collapseSpace(readFile(t, "gx-design/SKILL.md"))
	_, step, ok := strings.Cut(text, "**No `docs/specs/<epic>.md`**")
	if !ok {
		t.Fatal("gx-design/SKILL.md lacks step 4")
	}
	leadIn, _, _ := strings.Cut(step, "**Implementer's choice**")
	if strings.Contains(leadIn, "gx files url") {
		t.Error("step 4 lead-in must not carry the gx files url note")
	}
	_, afterBullets, ok := strings.Cut(step, "**Fact that needs a real run")
	if !ok {
		t.Fatal("step 4 lacks the real-run bullet")
	}
	if !strings.Contains(afterBullets, "You, not the spec subagent, hand the spec to the person: give its path as a `gx files url <path>` link") {
		t.Error("step 4 lacks the orchestrator gx files url note after the bullets")
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
