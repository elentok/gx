// Package playwrightguard holds the repo guard for Playwright state reads: a
// locator read (evaluate, evaluateAll) must follow an expect on the same
// locator, so the read never races the page (skills/gx-playwright-e2es).
package playwrightguard

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// readCall captures the receiver expression of a locator read. getComputedStyle
// only runs inside an evaluate callback, so the evaluate call covers it.
var readCall = regexp.MustCompile(`([\w.]+(?:\([^()]*\))?(?:\.[\w]+\([^()]*\))*)\.evaluate(?:All)?\(`)

var (
	identifier = regexp.MustCompile(`^\w+`)
	// scopeStart marks the start of the enclosing test or helper.
	scopeStart = regexp.MustCompile(`^(?:\s*test(?:\.\w+)?\(|(?:export )?(?:async )?function\b|(?:export )?const \w+ = )`)
)

// baseName returns the first identifier of a receiver expression, such as
// "composer" for `composer(page).getByText("x")`.
func baseName(receiver string) string {
	return identifier.FindString(receiver)
}

// isHandle reports whether recv was assigned from evaluateHandle or newPage
// earlier in its test. A JSHandle or a Page is not a locator and has no
// web-first assertion for this read.
func isHandle(lines []string, i int, recv string) bool {
	assign := regexp.MustCompile(`\b` + regexp.QuoteMeta(recv) + `\s*=\s*await\b.*\.(?:evaluateHandle|newPage)\(`)
	for j := i - 1; j >= 0; j-- {
		if assign.MatchString(lines[j]) {
			return true
		}
		if scopeStart.MatchString(lines[j]) {
			return false
		}
	}
	return false
}

// violations returns one problem per locator read in src that has no expect
// on the same locator earlier in its test or helper. A read inside
// expect.poll retries on its own and passes. page.evaluate is a page-level
// write or read, not a locator read, and is not checked.
func violations(path, src string) []string {
	lines := strings.Split(src, "\n")
	var problems []string
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "//") {
			continue
		}
		m := readCall.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		recv := baseName(m[1])
		if recv == "" || m[1] == "page" || recv == "window" || recv == "document" {
			continue
		}
		if insidePoll(lines, i) || hasExpectOn(lines, i, recv) || isHandle(lines, i, recv) {
			continue
		}
		problems = append(problems, fmt.Sprintf(
			"%s:%d: %q is read before any expect on it. Add `await expect(%s)...` (toBeVisible, toHaveCount, toHaveCSS) before the read, or wrap the read in expect.poll.",
			path, i+1, strings.TrimSpace(line), recv))
	}
	return problems
}

func insidePoll(lines []string, i int) bool {
	if strings.Contains(lines[i], "poll(") {
		return true
	}
	return i > 0 && strings.HasSuffix(strings.TrimSpace(lines[i-1]), "poll(")
}

func hasExpectOn(lines []string, i int, recv string) bool {
	word := regexp.MustCompile(`\b` + regexp.QuoteMeta(recv) + `\b`)
	// The read's own line never counts: `expect(await x.evaluate(...))` is the read.
	for j := i - 1; j >= 0; j-- {
		line := lines[j]
		if strings.Contains(line, "expect(") && word.MatchString(line) {
			return true
		}
		if scopeStart.MatchString(line) {
			return false
		}
	}
	return false
}

func TestRepoFlowsReadStateAfterExpect(t *testing.T) {
	files, err := filepath.Glob("../webui/e2e/*.ts")
	if err != nil || len(files) == 0 {
		t.Fatalf("no flow files found: %v", err)
	}
	for _, path := range files {
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range violations(filepath.Base(path), string(src)) {
			t.Error(p)
		}
	}
}

func TestFixtures(t *testing.T) {
	cases := []struct {
		file string
		want int // number of problems
	}{
		{"testdata/violating.e2e.ts", 3},
		{"testdata/compliant.e2e.ts", 0},
	}
	for _, c := range cases {
		t.Run(c.file, func(t *testing.T) {
			src, err := os.ReadFile(c.file)
			if err != nil {
				t.Fatal(err)
			}
			got := violations(c.file, string(src))
			if len(got) != c.want {
				t.Fatalf("got %d problems, want %d:\n%s", len(got), c.want, strings.Join(got, "\n"))
			}
		})
	}
}
