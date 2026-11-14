package ticketspec

import (
	"slices"
	"strings"
	"testing"
)

func story(n int, group string, flows int, states ...string) PlanStory {
	return PlanStory{Story: n, Group: group, WebFlows: flows, MockStates: states}
}

func slugs(es []VerifyEntry) []string {
	var out []string
	for _, e := range es {
		out = append(out, e.Slug)
	}
	return out
}

func TestVerifyPlan_PackSmallStoriesFitOnePart(t *testing.T) {
	t.Parallel()
	es := PackVerifyPlan(VerifyPlan{Stories: []PlanStory{story(1, "", 1), story(2, "", 1), story(3, "", 1)}})
	if got := slugs(es); !slices.Equal(got, []string{"verify"}) {
		t.Fatalf("slugs = %v, want [verify]", got)
	}
	if want := VerifyPartBaseTokens + 3*WebFlowTokens; es[0].ExpectedContextWindow != want {
		t.Errorf("window = %d, want %d", es[0].ExpectedContextWindow, want)
	}
	if len(es[0].BlockedBy) != 0 || !slices.Equal(es[0].Stories, []int{1, 2, 3}) {
		t.Errorf("entry = %+v", es[0])
	}
}

func TestVerifyPlan_PackOverLimitGivesPartsAndCollect(t *testing.T) {
	t.Parallel()
	es := PackVerifyPlan(VerifyPlan{Stories: []PlanStory{story(1, "", 7), story(2, "", 1)}})
	if got := slugs(es); !slices.Equal(got, []string{"verify-part-1", "verify-part-2", "verify-collect"}) {
		t.Fatalf("slugs = %v", got)
	}
	c := es[2]
	if !slices.Equal(c.BlockedBy, []string{"verify-part-1", "verify-part-2"}) || c.ExpectedContextWindow != VerifyCollectTokens {
		t.Errorf("collect = %+v", c)
	}
	for _, p := range es[:2] {
		if len(p.BlockedBy) != 0 || p.ExpectedContextWindow == 0 {
			t.Errorf("part = %+v", p)
		}
	}
}

func TestVerifyPlan_PackSharedGroupStaysTogether(t *testing.T) {
	t.Parallel()
	es := PackVerifyPlan(VerifyPlan{Stories: []PlanStory{
		story(1, "a", 3), story(2, "b", 3), story(3, "a", 3),
	}})
	if !slices.Equal(es[0].Stories, []int{1, 3}) || !slices.Equal(es[1].Stories, []int{2}) {
		t.Errorf("parts = %v / %v", es[0].Stories, es[1].Stories)
	}
}

func TestVerifyPlan_PackGroupOverLimitSplits(t *testing.T) {
	t.Parallel()
	es := PackVerifyPlan(VerifyPlan{Stories: []PlanStory{story(1, "a", 5), story(2, "a", 5)}})
	if got := slugs(es); !slices.Equal(got, []string{"verify-part-1", "verify-part-2", "verify-collect"}) {
		t.Fatalf("slugs = %v", got)
	}
	if !slices.Equal(es[0].Stories, []int{1}) || !slices.Equal(es[1].Stories, []int{2}) {
		t.Errorf("stories = %v / %v", es[0].Stories, es[1].Stories)
	}
}

func TestVerifyPlan_PackSingleStoryOverLimitOwnPartWithNote(t *testing.T) {
	t.Parallel()
	es := PackVerifyPlan(VerifyPlan{Stories: []PlanStory{story(1, "", 1), story(2, "", 9)}})
	if es[0].Note != "" {
		t.Errorf("part 1 note = %q", es[0].Note)
	}
	if !slices.Equal(es[1].Stories, []int{2}) || !strings.Contains(es[1].Note, "over budget") {
		t.Errorf("part 2 = %+v", es[1])
	}
}

func TestVerifyPlan_PackSharedMockStateCountsOnce(t *testing.T) {
	t.Parallel()
	es := PackVerifyPlan(VerifyPlan{Stories: []PlanStory{story(1, "", 0, "s"), story(2, "", 0, "s", "t")}})
	if want := VerifyPartBaseTokens + 2*MockStateTokens; es[0].ExpectedContextWindow != want {
		t.Errorf("window = %d, want %d", es[0].ExpectedContextWindow, want)
	}
}

func TestVerifyPlan_PartEstimateIsBasePlusStories(t *testing.T) {
	t.Parallel()
	s := PlanStory{Story: 1, WebFlows: 1, CLIChecks: 2, GoTests: 3, SkillQuotes: 4, MockStates: []string{"m"}}
	want := VerifyPartBaseTokens + WebFlowTokens + 2*CLICheckTokens + 3*GoTestRunTokens + 4*SkillQuoteTokens + MockStateTokens
	if got := PackVerifyPlan(VerifyPlan{Stories: []PlanStory{s}})[0].ExpectedContextWindow; got != want {
		t.Errorf("window = %d, want %d", got, want)
	}
}

func TestVerifyPlan_ParseRejectsBadPlans(t *testing.T) {
	t.Parallel()
	for name, src := range map[string]string{
		"empty":         "stories: []\n",
		"not yaml":      ":\n  - [",
		"unknown field": "stories:\n  - story: 1\n    bogus: 2\n",
		"no number":     "stories:\n  - web_flows: 1\n",
		"duplicate":     "stories:\n  - story: 1\n  - story: 1\n",
		"negative":      "stories:\n  - story: 1\n    web_flows: -1\n",
	} {
		if _, err := ParseVerifyPlan([]byte(src)); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
	if _, err := ParseVerifyPlan([]byte("stories:\n  - story: 1\n    group: g\n    web_flows: 2\n    mock_states: [a]\n")); err != nil {
		t.Errorf("good plan: %v", err)
	}
}

const groupedSpec = "## User Stories\n\n1. One\n2. Two\n3. Three\n4. Four\n\n## Verification\n\nAlpha (stories 1 to 3)\n\n- check a\n- check b\n\nBeta (stories 4 to 4)\n\n- check c\n"

func buildBody(t *testing.T, spec string, nums ...int) (string, error) {
	t.Helper()
	var plan VerifyPlan
	for _, n := range nums {
		plan.Stories = append(plan.Stories, story(n, "", 0))
	}
	ts, err := BuildVerifyTickets(plan, spec, "docs/specs/x.md")
	if err != nil {
		return "", err
	}
	return ts[0].Body, nil
}

func TestVerifyBodies_GroupHeadingGivesWholeBlock(t *testing.T) {
	t.Parallel()
	body, err := buildBody(t, groupedSpec, 1, 4)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Alpha (stories 1 to 3)\n\n- check a\n- check b", "Beta (stories 4 to 4)\n\n- check c"} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q in:\n%s", want, body)
		}
	}
	if strings.Contains(strings.SplitN(body, "## Story 4", 2)[0], "check c") {
		t.Errorf("next group leaked into story 1:\n%s", body)
	}
}

func TestSpecGroupBlock_MultiRangeHeading(t *testing.T) {
	t.Parallel()
	section := "Skill size (stories 29 to 31)\n\n- size\n\n" +
		"Triage (stories 32 to 43, 55 to 57, including 36a and 39b)\n\n- triage\n\n" +
		"Last (stories 60 to 60)\n\n- last\n"
	for _, n := range []int{32, 43, 55, 57} {
		got, ok := SpecGroupBlock(section, n)
		if !ok || !strings.HasPrefix(got, "Triage (") || !strings.HasSuffix(got, "- triage") {
			t.Errorf("story %d: got %q, %v", n, got, ok)
		}
	}
	for _, n := range []int{44, 54, 58} {
		if got, ok := SpecGroupBlock(section, n); ok {
			t.Errorf("story %d in a gap got block %q", n, got)
		}
	}
	got, _ := SpecGroupBlock(section, 30)
	if strings.Contains(got, "triage") || !strings.HasSuffix(got, "- size") {
		t.Errorf("previous group runs past the multi-range heading: %q", got)
	}
	if got, ok := SpecVerificationEntry(section, 56); !ok || !strings.Contains(got, "- triage") {
		t.Errorf("entry for 56: %q, %v", got, ok)
	}
}

func TestVerifyBodies_NumberedItemBeatsGroup(t *testing.T) {
	t.Parallel()
	spec := strings.Replace(groupedSpec, "## Verification\n\n", "## Verification\n\n2. numbered two\n\n", 1)
	body, err := buildBody(t, spec, 2)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body, "numbered two") || strings.Contains(body, "check a") {
		t.Errorf("numbered item should win:\n%s", body)
	}
}

func TestVerifyBodies_StoryOutsideGroupsFails(t *testing.T) {
	t.Parallel()
	spec := strings.Replace(groupedSpec, "(stories 4 to 4)", "(stories 9 to 9)", 1)
	_, err := buildBody(t, spec, 4)
	if err == nil || !strings.Contains(err.Error(), "story 4 has no entry in ## Verification") {
		t.Fatalf("err = %v", err)
	}
}
