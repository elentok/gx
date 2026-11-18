package playwrightguard

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// rulesSkill is the skill the messages point at. One constant, so a rename
// edits one place.
const rulesSkill = "the gx-only-e2es skill"

var (
	fixedSleep       = regexp.MustCompile(`\bwaitForTimeout\(`)
	retriesAny       = regexp.MustCompile(`(?i)retries`)
	retriesSet       = regexp.MustCompile(`\bretries\s*:`)
	retriesZero      = regexp.MustCompile(`\bretries\s*:\s*0\s*,?\s*(//.*)?$`)
	configureRetries = regexp.MustCompile(`(?s)describe\.configure\(\s*\{[^}]*retries`)
)

func isComment(line string) bool {
	t := strings.TrimSpace(line)
	return strings.HasPrefix(t, "//") || strings.HasPrefix(t, "*") || strings.HasPrefix(t, "/*") || strings.HasPrefix(t, "#")
}

// sleepProblems returns one problem per waitForTimeout call outside comments.
func sleepProblems(path, src string) []string {
	var problems []string
	for i, line := range strings.Split(src, "\n") {
		if isComment(line) || !fixedSleep.MatchString(line) {
			continue
		}
		problems = append(problems, fmt.Sprintf(
			"%s:%d: %q is a fixed sleep. Wait on the state instead: await expect(...), expect.poll, or a positive signal from the server. Rules: %s.",
			path, i+1, strings.TrimSpace(line), rulesSkill))
	}
	return problems
}

// configRetryProblems requires exactly one retries setting and that it is the literal 0.
func configRetryProblems(path, src string) []string {
	var set []string
	for i, line := range strings.Split(src, "\n") {
		if !isComment(line) && retriesSet.MatchString(line) {
			set = append(set, fmt.Sprintf("%d:%s", i+1, strings.TrimSpace(line)))
			if !retriesZero.MatchString(strings.TrimSpace(line)) {
				return []string{fmt.Sprintf("%s:%d: %q must be the literal `retries: 0`, not env-dependent. A pass on retry never counts as a fix. Rules: %s.", path, i+1, strings.TrimSpace(line), rulesSkill)}
			}
		}
	}
	if len(set) != 1 {
		return []string{fmt.Sprintf("%s: want exactly one `retries: 0` setting, found %d. Rules: %s.", path, len(set), rulesSkill)}
	}
	return nil
}

// retryUseProblems flags any non-comment mention of retries in src.
func retryUseProblems(path, src string) []string {
	var problems []string
	for i, line := range strings.Split(src, "\n") {
		if !isComment(line) && retriesAny.MatchString(line) {
			problems = append(problems, fmt.Sprintf("%s:%d: %q turns retries on a required-CI path. Retries stay 0; fix the flow instead. Rules: %s.", path, i+1, strings.TrimSpace(line), rulesSkill))
		}
	}
	return problems
}

// block returns the lines from the first line matching start up to (not
// including) the first later line for which end reports true.
func block(src string, start *regexp.Regexp, end func(string) bool) string {
	lines := strings.Split(src, "\n")
	for i, l := range lines {
		if !start.MatchString(l) {
			continue
		}
		j := i + 1
		for j < len(lines) && !end(lines[j]) {
			j++
		}
		return strings.Join(lines[i:j], "\n")
	}
	return ""
}

func indentOf(l string) int { return len(l) - len(strings.TrimLeft(l, " ")) }

func makeTarget(src string) string {
	return block(src, regexp.MustCompile(`^web-e2e:`), func(l string) bool {
		return l != "" && !strings.HasPrefix(l, "\t")
	})
}

func ciJob(src string) string {
	return block(src, regexp.MustCompile(`^\s{2}web-e2e:\s*$`), func(l string) bool {
		return strings.TrimSpace(l) != "" && indentOf(l) <= 2
	})
}

func e2eScript(src string) string {
	return block(src, regexp.MustCompile(`"e2e"\s*:`), func(string) bool { return true })
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestFlowsHaveNoFixedSleeps(t *testing.T) {
	files, err := filepath.Glob("../webui/e2e/*.ts")
	if err != nil || len(files) == 0 {
		t.Fatalf("no flow files found: %v", err)
	}
	for _, path := range files {
		for _, p := range sleepProblems(filepath.Base(path), read(t, path)) {
			t.Error(p)
		}
	}
}

func TestPlaywrightRetriesStayZero(t *testing.T) {
	for _, p := range configRetryProblems("webui/e2e/playwright.config.ts", read(t, "../webui/e2e/playwright.config.ts")) {
		t.Error(p)
	}
	paths := []struct{ name, src string }{
		{"Makefile (web-e2e)", makeTarget(read(t, "../Makefile"))},
		{".github/workflows/ci.yml (web-e2e)", ciJob(read(t, "../.github/workflows/ci.yml"))},
		{"webui/package.json (e2e)", e2eScript(read(t, "../webui/package.json"))},
		{"brief/playwright.go", read(t, "../brief/playwright.go")},
	}
	for _, p := range paths[:3] {
		if p.src == "" {
			t.Errorf("%s: section not found, so the retries rule cannot check it", p.name)
		}
	}
	for _, p := range paths {
		for _, m := range retryUseProblems(p.name, p.src) {
			t.Error(m)
		}
	}
	flows, _ := filepath.Glob("../webui/e2e/*.ts")
	for _, f := range flows {
		if configureRetries.MatchString(read(t, f)) {
			t.Errorf("%s: test.describe.configure sets retries. Retries stay 0. Rules: %s.", filepath.Base(f), rulesSkill)
		}
	}
}

func TestFixedSleepFixtures(t *testing.T) {
	cases := []struct {
		file string
		want int
	}{
		{"testdata/fixedsleep_violating.e2e.ts", 2},
		{"testdata/fixedsleep_compliant.e2e.ts", 0},
	}
	for _, c := range cases {
		t.Run(c.file, func(t *testing.T) {
			got := sleepProblems(c.file, read(t, c.file))
			if len(got) != c.want {
				t.Fatalf("got %d problems, want %d:\n%s", len(got), c.want, strings.Join(got, "\n"))
			}
		})
	}

	t.Run("retries config", func(t *testing.T) {
		for src, want := range map[string]int{
			"  retries: 0,\n":                      0,
			"  // retries: 2\n  retries: 0,\n":     0,
			"  retries: process.env.CI ? 2 : 0,\n": 1,
			"  retries: 2,\n":                      1,
			"  workers: 1,\n":                      1,
			"  retries: 0,\n  retries: 0,\n":       1,
		} {
			if got := len(configRetryProblems("c.ts", src)); got != want {
				t.Errorf("%q: got %d problems, want %d", src, got, want)
			}
		}
	})

	t.Run("retries on CI paths", func(t *testing.T) {
		mk := "web-e2e: web-build\n\t$(GX) brief playwright --all --retries=1\n\nother:\n\t--retries=3\n"
		if got := makeTarget(mk); len(retryUseProblems("Makefile", got)) != 1 {
			t.Errorf("makefile target %q should hold one problem", got)
		}
		ci := "jobs:\n  web-e2e:\n    steps:\n      - run: make web-e2e\n  other:\n    retries: 2\n"
		if got := ciJob(ci); len(retryUseProblems("ci.yml", got)) != 0 || !strings.Contains(got, "make web-e2e") || strings.Contains(got, "other") {
			t.Errorf("ci job slice wrong: %q", got)
		}
		pkg := "\"e2e\": \"playwright test --retries=1\",\n\"x\": \"y\"\n"
		if got := e2eScript(pkg); len(retryUseProblems("package.json", got)) != 1 {
			t.Errorf("package script %q should hold one problem", got)
		}
		if !configureRetries.MatchString("test.describe.configure({\n  mode: 'serial',\n  retries: 2,\n});") {
			t.Error("describe.configure retries not matched")
		}
	})
}
