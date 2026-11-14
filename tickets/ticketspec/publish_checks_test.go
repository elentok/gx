package ticketspec

import (
	"slices"
	"strings"
	"testing"
)

const subHeadingBody = "## Test seams\n\n### Sub\n\n```sh\n# a comment\n```\n\n## Where to look\n\n- x\n\n" +
	"## Acceptance criteria\n\n- [ ] one\n\n### Sub\n\n```sh\n# comment\n```\n\n- [ ] two\n- [ ] three\n"

func TestPublishShapeMisses_SubHeadingAndFencedCommentStayInSection(t *testing.T) {
	if got := PublishShapeMisses("implement", subHeadingBody, 0); len(got) != 0 {
		t.Fatalf("misses = %v, want none", got)
	}
}

func TestGroupCriteriaMisses(t *testing.T) {
	canon := func(a string) string { return strings.TrimPrefix(a, "mine:") }
	from := []string{"mine:follow-ups/01", "mine:follow-ups/02"}
	body := func(criteria string) string { return "## What to build\n\nx\n\n## Acceptance criteria\n" + criteria }
	type tc struct {
		from []string
		body string
		want []string
	}
	tests := map[string]tc{
		"all groups present": {from, body("\n### From follow-ups/01\n\n- [ ] a\n\n### From mine:follow-ups/02\n\n- [x] b\n"), nil},
		"one missing": {from, body("\n### From follow-ups/01\n\n- [ ] a\n"),
			[]string{"no `### From mine:follow-ups/02` group under ## Acceptance criteria"}},
		"empty group": {from, body("\n### From follow-ups/01\n\n- [ ] a\n\n### From follow-ups/02\n\nprose only\n\n## Other\n\n- [ ] not here\n"),
			[]string{"`### From mine:follow-ups/02` group has no `- [ ]` item"}},
		"short heading matches canonical member": {from, body("\n### From follow-ups/02\n\n- [ ] b\n\n### From follow-ups/01\n\n- [ ] a\n"), nil},
		"single from is not a group":             {from[:1], body("- [ ] plain\n"), nil},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if got := GroupCriteriaMisses(tc.from, tc.body, canon); !slices.Equal(got, tc.want) {
				t.Errorf("misses = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestBundledCounts_ReadsPastSubHeadingAndFencedComment(t *testing.T) {
	criteria, _ := BundledCounts(subHeadingBody, nil)
	if criteria != 3 {
		t.Fatalf("criteria = %d, want 3", criteria)
	}
}

func verifyRowSlugs(t *testing.T, manifest []byte) []string {
	t.Helper()
	var out []string
	for line := range strings.SplitSeq(string(manifest), "\n") {
		if s, ok := strings.CutPrefix(strings.TrimSpace(strings.TrimPrefix(line, "- ")), "slug: "); ok {
			out = append(out, s)
		}
	}
	return out
}

func vt(slug string) VerifyTicket {
	return VerifyTicket{VerifyEntry: VerifyEntry{Slug: slug, Type: "verify"}}
}

func TestAppendManifest_RerunReplacesVerifyRows(t *testing.T) {
	base := []byte("- slug: first\n  type: implement\n  body: b.md\n")
	two := []VerifyTicket{vt("verify-part-1"), vt("verify-part-2"), vt("verify-collect")}
	once, err := AppendManifest(base, two)
	if err != nil {
		t.Fatal(err)
	}
	twice, err := AppendManifest(once, two)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"first", "verify-part-1", "verify-part-2", "verify-collect"}
	if got := verifyRowSlugs(t, twice); !slices.Equal(got, want) {
		t.Fatalf("slugs after rerun = %v, want %v", got, want)
	}
	repacked, err := AppendManifest(twice, []VerifyTicket{vt("verify")})
	if err != nil {
		t.Fatal(err)
	}
	if got := verifyRowSlugs(t, repacked); !slices.Equal(got, []string{"first", "verify"}) {
		t.Fatalf("slugs after repack = %v", got)
	}
}
