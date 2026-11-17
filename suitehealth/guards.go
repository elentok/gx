package suitehealth

import "regexp"

// Guard is one exclusion: a failure that matches it is dropped from the
// headline and the flip list, because a guard test fails on purpose when code
// breaks a rule. Exactly one of Kind and Pattern is set. Kind excludes a whole
// check kind; Pattern is matched against the stable test name, and Example is
// a name it matches (the table test in guards_test.go runs it).
type Guard struct {
	Name    string
	Kind    string
	Pattern *regexp.Regexp
	Example string
}

// Guards is the ONE exclusion list. Every guard ticket adds its own entry
// here: one line in this list, nothing else. The table tests in
// guards_test.go cover a new entry without a new test.
var Guards = []Guard{
	{Name: "sizeguard", Pattern: regexp.MustCompile(`^github\.com/elentok/gx/sizeguard\.`), Example: "github.com/elentok/gx/sizeguard.TestLimits"},
	{Name: "sleepguard", Pattern: regexp.MustCompile(`^github\.com/elentok/gx/sleepguard\.`), Example: "github.com/elentok/gx/sleepguard.TestNoSleeps"},
	{Name: "oldwords", Pattern: regexp.MustCompile(`^github\.com/elentok/gx/oldwords\.`), Example: "github.com/elentok/gx/oldwords.TestNoOldWords"},
	{Name: "envguard", Pattern: regexp.MustCompile(`^github\.com/elentok/gx/envguard\.`), Example: "github.com/elentok/gx/envguard.TestEnv"},
	{Name: "playwrightguard", Pattern: regexp.MustCompile(`^github\.com/elentok/gx/playwrightguard\.`), Example: "github.com/elentok/gx/playwrightguard.TestMaxFlows"},
	{Name: "setstateguard", Pattern: regexp.MustCompile(`^github\.com/elentok/gx/setstateguard\.`), Example: "github.com/elentok/gx/setstateguard.TestNoRawSetStateCleanup"},
	{Name: "portable test skills rules test", Pattern: regexp.MustCompile(`^github\.com/elentok/gx/skills\.TestPortableTestSkillsHoldNoGxOnlyText$`), Example: "github.com/elentok/gx/skills.TestPortableTestSkillsHoldNoGxOnlyText"},
	{Name: "gx-only-e2es rules test", Pattern: regexp.MustCompile(`^github\.com/elentok/gx/skills\.TestGxOnlyE2esSkillIsWired$`), Example: "github.com/elentok/gx/skills.TestGxOnlyE2esSkillIsWired"},
	{Name: "biome lint", Kind: "lint"},
	// The check itself runs in every Vitest test's afterEach, so a leak is attributed to the leaking
	// test, which no name can match. Only the check's own unit tests are matched by name.
	{Name: "pending-timer check", Pattern: regexp.MustCompile(`^(?:.*/)?src/pendingTimers\.test\.tsx(?: > |$)`), Example: "src/pendingTimers.test.tsx > pending timer check reports a pending timeout with its creation stack"},
	// Judgement call: the skills package also holds tests that are not rules
	// tests, and they are dropped too. Splitting them needs a naming rule the
	// package does not have yet.
	{Name: "skills rules tests", Pattern: regexp.MustCompile(`^github\.com/elentok/gx/skills\.`), Example: "github.com/elentok/gx/skills.TestRules"},
}

// Excluded reports whether a failure of the given check kind and test name
// matches a guard.
func Excluded(kind, name string) bool {
	for _, g := range Guards {
		if g.Kind != "" && g.Kind == kind {
			return true
		}
		if g.Pattern != nil && g.Pattern.MatchString(name) {
			return true
		}
	}
	return false
}
