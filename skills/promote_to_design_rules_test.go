package skills

import (
	"maps"
	"regexp"
	"strings"
	"testing"
)

// fixedLines maps each fixed first-line sentence (found in gx-handoff.md by
// its first word) to the files that must carry it. Ticket 13 adds
// gx-fork/SKILL.md to the fork row.
var fixedLines = []struct {
	prefix string
	files  []string
}{
	{"Promoted", []string{"gx-handoff.md", "gx-promote-to-design/SKILL.md", "gx-wayfinder/SKILL.md"}},
	{"Forked", []string{"gx-handoff.md", "gx-wayfinder/SKILL.md"}},
}

func TestForkedOutAnchors(t *testing.T) {
	wf := collapseSpace(readFile(t, "gx-wayfinder/SKILL.md"))
	for _, want := range []string{"## Forked out", "](../gx-handoff.md)"} {
		if !strings.Contains(wf, want) {
			t.Errorf("gx-wayfinder/SKILL.md lacks %q", want)
		}
	}
	if !strings.Contains(collapseSpace(readFile(t, "gx-design/SKILL.md")), "## Forked out") {
		t.Error("gx-design/SKILL.md rubric lacks ## Forked out")
	}
}

func fixedLineRe(prefix string) *regexp.Regexp {
	return regexp.MustCompile("(?m)^`(" + prefix + "[^`]+)`$")
}

// fixedLineGaps reports the files that lack the sentence gx-handoff.md holds
// for prefix.
func fixedLineGaps(prefix string, files []string, text map[string]string) []string {
	m := fixedLineRe(prefix).FindStringSubmatch(text["gx-handoff.md"])
	if m == nil {
		return []string{"gx-handoff.md has no " + prefix + " line"}
	}
	var gaps []string
	for _, f := range files {
		if !strings.Contains(collapseSpace(text[f]), "`"+collapseSpace(m[1])+"`") {
			gaps = append(gaps, f+" lacks "+m[1])
		}
	}
	return gaps
}

func readFixedLineFiles(t *testing.T) map[string]string {
	text := map[string]string{}
	for _, row := range fixedLines {
		for _, f := range row.files {
			text[f] = readFile(t, f)
		}
	}
	return text
}

func TestFixedLinesCarriedByTheirFiles(t *testing.T) {
	text := readFixedLineFiles(t)
	for _, row := range fixedLines {
		for _, gap := range fixedLineGaps(row.prefix, row.files, text) {
			t.Errorf("drift: %s", gap)
		}
	}
}

func TestFixedLineDriftIsCaught(t *testing.T) {
	text := readFixedLineFiles(t)
	m := fixedLineRe("Promoted").FindStringSubmatch(text["gx-handoff.md"])
	if m == nil {
		t.Fatal("gx-handoff.md lacks the Promote line")
	}
	row := fixedLines[0]
	for _, f := range row.files[1:] {
		drifted := maps.Clone(text)
		drifted[f] = strings.ReplaceAll(text[f], m[1], m[1]+" Changed.")
		if len(fixedLineGaps(row.prefix, row.files, drifted)) == 0 {
			t.Errorf("check passes when the line differs in %s", f)
		}
	}
	drifted := maps.Clone(text)
	drifted["gx-handoff.md"] = strings.ReplaceAll(text["gx-handoff.md"], m[1], m[1]+" Changed.")
	if len(fixedLineGaps(row.prefix, row.files, drifted)) == 0 {
		t.Error("check passes when the line differs in gx-handoff.md")
	}
}

func TestFixedHandoffHeadingOrder(t *testing.T) {
	doc := readFile(t, "gx-handoff.md")
	kinds := []struct {
		heading  string
		sections []string
	}{
		{"## Design fork brief", []string{"## Goal", "## Decisions", "## Open questions", "## Handled elsewhere", "## Branch and changed files", "## Source"}},
		{"## Chat fork brief", []string{"## Goal", "## Decisions are settled", "## Open questions", "## Working folder", "## Start by", "## Handled elsewhere", "## Branch and changed files", "## Source"}},
		{"## Promote brief", []string{"## Goal", "## Decisions", "## Open questions", "## Branch and changed files", "## Source chat"}},
	}
	ends := []string{"## Chat fork brief", "## Promote brief", "## Section rules"}
	for i, k := range kinds {
		start := strings.Index(doc, k.heading+"\n")
		if start < 0 {
			t.Errorf("gx-handoff.md lacks %q", k.heading)
			continue
		}
		body := doc[start+len(k.heading):]
		if end := strings.Index(body, "\n"+ends[i]+"\n"); end >= 0 {
			body = body[:end]
		}
		pos := 0
		for _, h := range k.sections {
			idx := strings.Index(body[pos:], "\n"+h+"\n")
			if idx < 0 {
				t.Errorf("%s: %q missing or out of order", k.heading, h)
				break
			}
			pos += idx + 1 + len(h)
		}
		if k.heading == "## Promote brief" && strings.Contains(body, "\n## Handled elsewhere\n") {
			t.Errorf("%s lists Handled elsewhere", k.heading)
		}
	}
	flat := collapseSpace(doc)
	for _, want := range []string{"Handled elsewhere", "Working folder", "Start by", "## Source chat"} {
		if !strings.Contains(flat, want) {
			t.Errorf("gx-handoff.md lacks %q", want)
		}
	}
}

func TestGxStartDesignSkill(t *testing.T) {
	text := collapseSpace(readFile(t, "gx-start-design/SKILL.md"))
	for _, want := range []string{
		"gx tickets epic next-name",
		"\"Start this Design?\"",
		"8 to 14",
		"fewer than 8 open tickets proposes all of them",
		"numbered tickets",
		"print the card again **once**",
		"gx tickets start-design <project:backlog> --map <name> --title",
		"gx server web-url --json",
		"/#/d/<project>:<map>",
		"[gx-grilling](../gx-grilling/SKILL.md)",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("gx-start-design/SKILL.md lacks %q", want)
		}
	}
}

func TestGxPromoteToDesignSkill(t *testing.T) {
	text := collapseSpace(readFile(t, "gx-promote-to-design/SKILL.md"))
	for _, want := range []string{
		"gx agent actionable",
		"gx designs open <slug> --idea - --project <project>",
		"gx server web-url --json",
		"gx agent transcript --json",
		"## Failure handling",
		"## The brief",
		"[gx-handoff](../gx-handoff.md)",
		"`base_url`",
		"[gx-grilling](../gx-grilling/SKILL.md)",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("gx-promote-to-design/SKILL.md lacks %q", want)
		}
	}
	if strings.Contains(text, "Handled elsewhere") {
		t.Error("gx-promote-to-design/SKILL.md names Handled elsewhere; a Promote brief leaves it out")
	}
	if !strings.Contains(collapseSpace(readFile(t, "gx-wayfinder/SKILL.md")), "[gx-promote-to-design](../gx-promote-to-design/SKILL.md)") {
		t.Error("gx-wayfinder/SKILL.md does not link the promote skill")
	}
}
