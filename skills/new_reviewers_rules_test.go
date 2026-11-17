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
		"gx-comment-analyzer",
		"gx-pr-test-analyzer",
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

// The verify-ticket rule lives once, in the Quality brief of gx-code-review.
func TestPRTestAnalyzerHoldsNoVerifyTicketRule(t *testing.T) {
	path := "gx-pr-test-analyzer/SKILL.md"
	if strings.Contains(normalizeSpace(readFile(t, path)), "open verify ticket") {
		t.Errorf("%s restates the open-verify-ticket rule", path)
	}
}
