package suitehealth

import (
	"testing"

	"github.com/elentok/gx/check"
)

func TestSuiteHealth_GuardEntriesHaveExactlyOneMatcher(t *testing.T) {
	t.Parallel()
	for _, g := range Guards {
		if g.Name == "" {
			t.Errorf("entry %+v has no name", g)
		}
		if (g.Kind == "") == (g.Pattern == nil) {
			t.Errorf("%s: need exactly one of Kind and Pattern", g.Name)
		}
		if g.Pattern != nil && !g.Pattern.MatchString(g.Example) {
			t.Errorf("%s: pattern does not match its example %q", g.Name, g.Example)
		}
	}
}

func TestSuiteHealth_GuardListNamesTheExistingGuards(t *testing.T) {
	t.Parallel()
	have := map[string]bool{}
	for _, g := range Guards {
		have[g.Name] = true
	}
	for _, want := range []string{"sizeguard", "sleepguard", "oldwords", "envguard", "playwrightguard", "setstateguard", "skills rules tests", "portable test skills rules test", "gx-only-e2es rules test"} {
		if !have[want] {
			t.Errorf("guard %q missing", want)
		}
	}
}

func TestSuiteHealth_EveryGuardEntryExcludesItsFailure(t *testing.T) {
	t.Parallel()
	for _, g := range Guards {
		kind, name := g.Kind, g.Example
		if kind == "" {
			kind = "test"
		}
		if !Excluded(kind, name) {
			t.Errorf("%s: failure %q of kind %q not excluded", g.Name, name, kind)
		}
		k := fail(name)
		k.Kind = kind
		dir := fixture(t, []check.Record{fullRun("r1", "t1", k)}, nil)
		out, _ := health(t, dir, Options{})
		wantLine(t, out, "1 of 1 first attempts passed")
	}
}
