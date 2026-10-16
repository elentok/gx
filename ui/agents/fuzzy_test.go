package agents

import (
	"slices"
	"testing"

	"github.com/elentok/gx/ui"
)

func TestFuzzyScore_EveryTermMustMatchInOrder(t *testing.T) {
	for _, tc := range []struct {
		query, text string
		ok          bool
	}{
		{"", "anything", true},
		{"agt", "agents-tab", true},
		{"AGT", "agents-tab", true},
		{"tga", "agents-tab", false},
		{"gx tab", "gx ~/dev/gx/agents-tab", true},
		{"gx zz", "gx ~/dev/gx/agents-tab", false},
	} {
		if _, _, ok := fuzzyScore(tc.query, tc.text); ok != tc.ok {
			t.Errorf("fuzzyScore(%q, %q) ok = %v, want %v", tc.query, tc.text, ok, tc.ok)
		}
	}
}

func TestFuzzyScore_PrefersConsecutiveAndWordStartMatches(t *testing.T) {
	score := func(q, text string) int {
		s, _, ok := fuzzyScore(q, text)
		if !ok {
			t.Fatalf("%q does not match %q", q, text)
		}
		return s
	}
	if a, b := score("tab", "/r/tab"), score("tab", "/r/t-x-a-x-b"); a <= b {
		t.Errorf("consecutive %d <= scattered %d", a, b)
	}
	if a, b := score("wt", "/r/wt1"), score("wt", "/r/xwt1"); a <= b {
		t.Errorf("word start %d <= mid-word %d", a, b)
	}
}

func TestFuzzyScore_PicksTheBestPlaceToMatch(t *testing.T) {
	_, pos, _ := fuzzyScore("ab", "a-x-b-ab")
	if !slices.Equal(pos, []int{6, 7}) {
		t.Fatalf("pos = %v, want the consecutive ab at 6,7", pos)
	}
}

func TestHighlightRunes_StylesTheMatchedRuns(t *testing.T) {
	got := highlightRunes("ab-cd", []int{0, 1, 4})
	want := ui.StyleFilterMatch.Render("ab") + "-c" + ui.StyleFilterMatch.Render("d")
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if highlightRunes("ab", nil) != "ab" {
		t.Fatal("no positions should leave the text as is")
	}
}
