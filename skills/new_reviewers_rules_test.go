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
		"gx-security-review",
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

// The upstream command runs shell lines; the skill reads the review files instead.
func TestSecurityReviewReadsReviewFiles(t *testing.T) {
	path := "gx-security-review/SKILL.md"
	text := readFile(t, path)
	for line := range strings.SplitSeq(text, "\n") {
		if strings.HasPrefix(line, "!") {
			t.Errorf("%s has a line starting with !: %q", path, line)
		}
	}
	if !strings.Contains(normalizeSpace(text), "shared review files") {
		t.Errorf("%s does not mention the shared review files", path)
	}
}

// The verify-ticket rule lives once, in the Quality brief of gx-code-review.
func TestPRTestAnalyzerHoldsNoVerifyTicketRule(t *testing.T) {
	path := "gx-pr-test-analyzer/SKILL.md"
	if strings.Contains(normalizeSpace(readFile(t, path)), "open verify ticket") {
		t.Errorf("%s restates the open-verify-ticket rule", path)
	}
}
