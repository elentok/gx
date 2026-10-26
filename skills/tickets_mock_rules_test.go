package skills

import (
	"strings"
	"testing"
)

func TestGxToTicketsMockRules(t *testing.T) {
	text := readFile(t, "gx-to-tickets/SKILL.md")
	for _, want := range []string{
		"## Mock check",
		"Only on an implement ticket that builds part of a stored mock",
		"A UI ticket that builds no mock\npart has no `## Mock check`",
		"- Mock file: `<full path>`",
		"Name the gxfixture scenario for every mock state",
		"add an implement ticket that creates it, and make it block every ticket\nthat checks that state",
		"Mock state check (one mock state at one width: screenshot the mock and the built page, list the differences) | 8K",
		"add 10K to `expected_context_window` per mock state",
		"at most 4 mock states per implement ticket",
		"one state section of the mock file, one name at one width",
		"The first ticket builds the parts and checks the first 4 states",
		"blocked by the first, checks the remaining states and fixes what differs",
		"A story's Mock states line packs like the story's other Verification items",
		"There is no per-part state cap and no per-mock-file rule",
		"a Mock states line is copied word for word too",
		"Copy each Mock states line word\nfor word into the part body",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("gx-to-tickets/SKILL.md lacks %q", want)
		}
	}
}
