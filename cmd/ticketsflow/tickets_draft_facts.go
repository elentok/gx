package ticketsflow

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/elentok/gx/migrations"
	"github.com/elentok/gx/tickets/ticketspec"
)

var (
	vitestEnvRe = regexp.MustCompile(`@vitest-environment\s+(\S+)`)
	makeRuleRe  = regexp.MustCompile(`(?m)^([A-Za-z0-9_.-]+)\s*:`)
	testFileRe  = regexp.MustCompile(`(_test\.go|\.test\.tsx?|\.e2e\.ts)$`)
	flowTitleRe = regexp.MustCompile(`test\(\s*["'` + "`" + `]([^"'` + "`" + `]+)`)
)

// Only mechanical facts are printed here. Each is a lookup, never a verdict.

// draftFacts fills each ticket's SeamFacts, Consumers and FieldTests and
// returns one miss per Makefile target, helper or Playwright pattern that does
// not check out.
func draftFacts(plan *ticketspec.DraftPlan, root string, finder *callerFinder, rgOK bool) []string {
	var misses []string
	var targets map[string]bool
	for i := range plan.Tickets {
		t := &plan.Tickets[i]
		for _, f := range t.TestFiles {
			if env, ok := vitestEnv(root, f); ok {
				t.SeamFacts = append(t.SeamFacts, fmt.Sprintf("test environment of `%s`: %s", f, env))
			}
		}
		for _, name := range t.MakeTargets {
			if targets == nil {
				targets = makeTargets(root)
			}
			if targets[name] {
				t.SeamFacts = append(t.SeamFacts, fmt.Sprintf("make target `%s` exists in Makefile", name))
			} else {
				misses = append(misses, fmt.Sprintf("%s: make target `%s` is not in Makefile", t.Slug, name))
			}
		}
		if t.Migrations {
			names := migrations.Default.Names()
			if len(names) == 0 {
				t.SeamFacts = append(t.SeamFacts, "highest migration number in use: none registered")
			} else {
				t.SeamFacts = append(t.SeamFacts, fmt.Sprintf("highest migration number in use: %d (step %s)", len(names), names[len(names)-1]))
			}
		}
		for _, p := range t.Playwright {
			line, ok := playwrightFact(root, p)
			t.SeamFacts = append(t.SeamFacts, line)
			if !ok {
				misses = append(misses, t.Slug+": "+line)
			}
		}
		if !rgOK {
			continue
		}
		for _, h := range t.Helpers {
			if line, ok := finder.helperFact(h); ok {
				t.SeamFacts = append(t.SeamFacts, line)
			} else {
				misses = append(misses, fmt.Sprintf("%s: helper `%s`: no such declaration", t.Slug, h))
			}
		}
		if len(t.ChangedFields) > 0 {
			t.Consumers = map[string]string{}
			t.FieldTests = map[string]string{}
			for _, f := range t.ChangedFields {
				t.Consumers[f], t.FieldTests[f] = finder.fieldConsumers(f)
			}
		}
	}
	return misses
}

// vitestEnv is the environment a TypeScript test file declares in its first
// lines, or "default" when it declares none.
func vitestEnv(root, file string) (string, bool) {
	if !strings.HasSuffix(file, ".ts") && !strings.HasSuffix(file, ".tsx") || strings.HasSuffix(file, ".e2e.ts") {
		return "", false
	}
	data, err := os.ReadFile(filepath.Join(root, file))
	if err != nil {
		return "", false
	}
	lines := strings.SplitN(string(data), "\n", 11)
	for _, l := range lines[:min(len(lines), 10)] {
		if m := vitestEnvRe.FindStringSubmatch(l); m != nil {
			return m[1], true
		}
	}
	return "default (no @vitest-environment line)", true
}

func makeTargets(root string) map[string]bool {
	out := map[string]bool{}
	data, err := os.ReadFile(filepath.Join(root, "Makefile"))
	if err != nil {
		return out
	}
	for _, m := range makeRuleRe.FindAllStringSubmatch(string(data), -1) {
		out[m[1]] = true
	}
	return out
}

// playwrightFact matches the -g pattern against the full title Playwright
// builds, `<project> › <file> › <title>`, for the file's flows.
func playwrightFact(root string, p ticketspec.PlaywrightCheck) (string, bool) {
	label := fmt.Sprintf("playwright -g `%s` on %s", p.Grep, p.File)
	re, err := regexp.Compile(p.Grep)
	if err != nil {
		return label + ": not a valid pattern: " + err.Error(), false
	}
	data, err := os.ReadFile(filepath.Join(root, "webui", "e2e", p.File))
	if err != nil {
		return label + ": file not found under webui/e2e", false
	}
	var matched []string
	for _, m := range flowTitleRe.FindAllStringSubmatch(string(data), -1) {
		title := m[1]
		if len(p.Flows) > 0 && !flowListed(title, p.Flows) {
			continue
		}
		if re.MatchString("chromium › " + p.File + " › " + title) {
			matched = append(matched, title)
		}
	}
	if len(matched) == 0 {
		return label + ": finds no tests (Playwright matches the full title, which starts with the project and file, so a ^ anchor never matches)", false
	}
	return fmt.Sprintf("%s: finds %d test(s): %s", label, len(matched), strings.Join(matched, "; ")), true
}

func flowListed(title string, flows []int) bool {
	for _, f := range flows {
		if strings.HasPrefix(title, fmt.Sprintf("%d. ", f)) {
			return true
		}
	}
	return false
}

// helperFact prints the declaration line of a cited helper and the comment
// above it, which is where its prerequisites and return value are written.
func (f *callerFinder) helperFact(symbol string) (string, bool) {
	name := symbol[strings.LastIndex(symbol, ".")+1:]
	pos, ok := f.goDefinition(name)
	if !ok {
		return "", false
	}
	parts := strings.Split(pos, ":")
	var line int
	if _, err := fmt.Sscanf(parts[1], "%d", &line); err != nil {
		return "", false
	}
	data, err := os.ReadFile(filepath.Join(f.root, parts[0]))
	if err != nil {
		return "", false
	}
	src := strings.Split(string(data), "\n")
	var doc []string
	for j := line - 2; j >= 0 && strings.HasPrefix(strings.TrimSpace(src[j]), "//"); j-- {
		doc = append([]string{strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(src[j]), "//"))}, doc...)
	}
	text := "none"
	if len(doc) > 0 {
		text = strings.Join(doc, " ")
	}
	decl := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(src[line-1]), "{"))
	return fmt.Sprintf("helper `%s` at %s:%d: `%s`; comment: %s", symbol, parts[0], line, decl, text), true
}

// fieldConsumers lists every line that mentions the field name, and the
// lines that sit in test files.
func (f *callerFinder) fieldConsumers(field string) (all, tests string) {
	name := field[strings.LastIndex(field, ".")+1:]
	var other, inTests []string
	for _, l := range f.rgLines(name) {
		if testFileRe.MatchString(strings.SplitN(l, ":", 2)[0]) {
			inTests = append(inTests, l)
		} else {
			other = append(other, l)
		}
	}
	return "rg, not verified: " + f.summarize(other), "rg, not verified: " + f.summarize(inTests)
}
