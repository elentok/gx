package skills

import (
	"strings"
	"testing"
)

// Later tickets add their reviewer skills to this table.
func TestNewReviewersRules(t *testing.T) {
	for _, name := range []string{
		"gx-code-reviewer",
		"gx-silent-failure-hunter",
		"gx-type-design-analyzer",
	} {
		t.Run(name, func(t *testing.T) {
			path := name + "/SKILL.md"
			checkAnchors(t, path,
				"## 1. Skip when",
				"nothing to review",
				"(../gx-code-review/SKILL.md)",
			)
			text := normalizeSpace(readFile(t, path))
			for _, cmd := range []string{"git diff", "git log -p", "git status"} {
				if strings.Contains(text, cmd) {
					t.Errorf("%s names the command %q", path, cmd)
				}
			}
		})
	}
}
