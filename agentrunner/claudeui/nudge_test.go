package claudeui

import (
	"errors"
	"slices"
	"testing"
	"time"
)

// fakePrompter is an in-memory Prompter. starts scripts WaitStart's answers in
// order (false once exhausted); screens scripts Screen's answers in order (the
// last one repeats). Every WaitStart advances clock by the timeout it was given.
type fakePrompter struct {
	clock     time.Time
	starts    []bool
	screens   []string
	calls     []string
	timeouts  []time.Duration
	decisions []NudgeDecision
}

func (f *fakePrompter) Submit(text string) error {
	f.calls = append(f.calls, "submit:"+text)
	return nil
}

func (f *fakePrompter) Clear() error {
	f.calls = append(f.calls, "clear")
	return nil
}

func (f *fakePrompter) report(d NudgeDecision) { f.decisions = append(f.decisions, d) }

func (f *fakePrompter) SendEnter() error {
	f.calls = append(f.calls, "enter")
	return nil
}

func (f *fakePrompter) WaitStart(timeout time.Duration) (bool, error) {
	f.calls = append(f.calls, "wait")
	f.timeouts = append(f.timeouts, timeout)
	f.clock = f.clock.Add(timeout)
	if len(f.starts) == 0 {
		return false, nil
	}
	started := f.starts[0]
	f.starts = f.starts[1:]
	return started, nil
}

func (f *fakePrompter) Screen() (string, error) {
	s := f.screens[0]
	if len(f.screens) > 1 {
		f.screens = f.screens[1:]
	}
	return s, nil
}

func (f *fakePrompter) now() time.Time { return f.clock }

func assertCalls(t *testing.T, got []string, want ...string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("calls = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("calls = %v, want %v", got, want)
		}
	}
}

func TestPromptWithNudge_StartSeenAtOnce(t *testing.T) {
	f := &fakePrompter{starts: []bool{true}, screens: []string{"a"}}
	if err := PromptWithNudge(f, "hi", DefaultNudgeCaps, time.Time{}, f.now, f.report); err != nil {
		t.Fatalf("PromptWithNudge() error = %v", err)
	}
	assertCalls(t, f.calls, "submit:hi", "wait")
	assertDecisions(t, f.decisions)
}

func TestPromptWithNudge_ChangedScreen_NudgesWithEnter(t *testing.T) {
	f := &fakePrompter{starts: []bool{false, true}, screens: []string{"before", "typed"}}
	if err := PromptWithNudge(f, "hi", DefaultNudgeCaps, time.Time{}, f.now, f.report); err != nil {
		t.Fatalf("PromptWithNudge() error = %v", err)
	}
	assertCalls(t, f.calls, "submit:hi", "wait", "enter", "wait")
	assertDecisions(t, f.decisions, NudgeEnter)
}

func TestPromptWithNudge_UnchangedScreen_ClearsThenRetypes(t *testing.T) {
	f := &fakePrompter{starts: []bool{false, true}, screens: []string{"empty"}}
	if err := PromptWithNudge(f, "hi", DefaultNudgeCaps, time.Time{}, f.now, f.report); err != nil {
		t.Fatalf("PromptWithNudge() error = %v", err)
	}
	assertCalls(t, f.calls, "submit:hi", "wait", "clear", "clear", "clear", "submit:hi", "wait")
	assertDecisions(t, f.decisions, NudgeRetype)
}

func TestPromptWithNudge_RetypeCap_ReturnsStuckSubmission(t *testing.T) {
	f := &fakePrompter{screens: []string{"empty"}}
	err := PromptWithNudge(f, "hi", DefaultNudgeCaps, time.Time{}, f.now, f.report)
	if !errors.Is(err, ErrStuckSubmission) {
		t.Fatalf("PromptWithNudge() error = %v, want ErrStuckSubmission", err)
	}
	assertCalls(t, f.calls, "submit:hi", "wait", "clear", "clear", "clear", "submit:hi", "wait", "clear", "clear", "clear", "submit:hi", "wait")
	assertDecisions(t, f.decisions, NudgeRetype, NudgeRetype, NudgeStuck)
}

// checkingPrompter adds an input check to fakePrompter; calls it records are
// "clear" and "check".
type checkingPrompter struct {
	*fakePrompter
	empty, known bool
}

func (c *checkingPrompter) InputEmpty() (bool, bool, error) {
	c.calls = append(c.calls, "check")
	return c.empty, c.known, nil
}

func clears(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = "clear"
	}
	return out
}

func TestPromptWithNudge_RetypeClearsWholeInput(t *testing.T) {
	// 3 lines: 2*3-1 presses plus two spare.
	text := "a\nb\nc"
	f := &fakePrompter{starts: []bool{false, true}, screens: []string{"empty"}}
	if err := PromptWithNudge(f, text, DefaultNudgeCaps, time.Time{}, f.now, f.report); err != nil {
		t.Fatalf("PromptWithNudge() error = %v", err)
	}
	want := append([]string{"submit:" + text, "wait"}, clears(7)...)
	want = append(want, "submit:"+text, "wait")
	assertCalls(t, f.calls, want...)
}

func TestPromptWithNudge_RetypeChecksInputAfterClear(t *testing.T) {
	f := &checkingPrompter{fakePrompter: &fakePrompter{starts: []bool{false, true}, screens: []string{"empty"}}, empty: true, known: true}
	if err := PromptWithNudge(f, "hi", DefaultNudgeCaps, time.Time{}, f.now, f.report); err != nil {
		t.Fatalf("PromptWithNudge() error = %v", err)
	}
	want := append([]string{"submit:hi", "wait"}, clears(3)...)
	want = append(want, "check", "submit:hi", "wait")
	assertCalls(t, f.calls, want...)
}

func TestPromptWithNudge_NotEmptyAfterClear_IsStuck(t *testing.T) {
	f := &checkingPrompter{fakePrompter: &fakePrompter{screens: []string{"empty"}}, known: true}
	err := PromptWithNudge(f, "hi", DefaultNudgeCaps, time.Time{}, f.now, f.report)
	if !errors.Is(err, ErrStuckSubmission) {
		t.Fatalf("PromptWithNudge() error = %v, want ErrStuckSubmission", err)
	}
	want := append([]string{"submit:hi", "wait"}, clears(3)...)
	want = append(want, "check")
	assertCalls(t, f.calls, want...)
	assertDecisions(t, f.decisions, NudgeRetype, NudgeStuck)
}

func TestPromptWithNudge_UnstyledRead_CountAloneDecides(t *testing.T) {
	f := &checkingPrompter{fakePrompter: &fakePrompter{starts: []bool{false, true}, screens: []string{"empty"}}}
	if err := PromptWithNudge(f, "hi", DefaultNudgeCaps, time.Time{}, f.now, f.report); err != nil {
		t.Fatalf("PromptWithNudge() error = %v", err)
	}
	assertDecisions(t, f.decisions, NudgeRetype)
}

func TestPromptWithNudge_NeverPressesCtrlC(t *testing.T) {
	f := &checkingPrompter{fakePrompter: &fakePrompter{screens: []string{"empty"}}, known: true}
	_ = PromptWithNudge(f, "a\nb", DefaultNudgeCaps, time.Time{}, f.now, f.report)
	for _, c := range f.calls {
		if c != "submit:a\nb" && c != "wait" && c != "clear" && c != "check" {
			t.Errorf("unexpected call %q: only typing, clearing and checks are allowed", c)
		}
	}
}

func TestPromptWithNudge_NudgeCap_ReturnsStartTimeout(t *testing.T) {
	f := &fakePrompter{screens: []string{"1", "2", "3", "4", "5"}}
	err := PromptWithNudge(f, "hi", DefaultNudgeCaps, time.Time{}, f.now, f.report)
	if !errors.Is(err, ErrStartTimeout) || errors.Is(err, ErrStuckSubmission) {
		t.Fatalf("PromptWithNudge() error = %v, want ErrStartTimeout only", err)
	}
	assertCalls(t, f.calls, "submit:hi", "wait", "enter", "wait", "enter", "wait", "enter", "wait")
	assertDecisions(t, f.decisions, NudgeEnter, NudgeEnter, NudgeEnter, NudgeTimeout)
}

func TestPromptWithNudge_NilReport(t *testing.T) {
	f := &fakePrompter{screens: []string{"empty"}}
	if err := PromptWithNudge(f, "hi", DefaultNudgeCaps, time.Time{}, f.now, nil); !errors.Is(err, ErrStuckSubmission) {
		t.Fatalf("PromptWithNudge() error = %v, want ErrStuckSubmission", err)
	}
}

func assertDecisions(t *testing.T, got []NudgeDecision, want ...NudgeDecision) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Fatalf("decisions = %v, want %v", got, want)
	}
}

func TestPromptWithNudge_CallerCaps(t *testing.T) {
	caps := NudgeCaps{Grace: time.Second, Nudges: 2, Retypes: 1}
	f := &fakePrompter{screens: []string{"empty", "typed", "typed"}}
	err := PromptWithNudge(f, "hi", caps, time.Time{}, f.now, f.report)
	if !errors.Is(err, ErrStartTimeout) {
		t.Fatalf("PromptWithNudge() error = %v, want ErrStartTimeout", err)
	}
	assertCalls(t, f.calls, "submit:hi", "wait", "enter", "wait", "clear", "clear", "clear", "submit:hi", "wait")
	for _, got := range f.timeouts {
		if got != caps.Grace {
			t.Fatalf("WaitStart timeouts = %v, want %v each", f.timeouts, caps.Grace)
		}
	}

	f = &fakePrompter{screens: []string{"empty"}}
	err = PromptWithNudge(f, "hi", caps, time.Time{}, f.now, f.report)
	if !errors.Is(err, ErrStuckSubmission) {
		t.Fatalf("PromptWithNudge() error = %v, want ErrStuckSubmission", err)
	}
	assertCalls(t, f.calls, "submit:hi", "wait", "clear", "clear", "clear", "submit:hi", "wait")
}

func TestPromptWithNudge_HonoursDeadline(t *testing.T) {
	f := &fakePrompter{screens: []string{"1", "2", "3", "4"}}
	deadline := f.clock.Add(DefaultNudgeCaps.Grace + 5*time.Second)
	err := PromptWithNudge(f, "hi", DefaultNudgeCaps, deadline, f.now, f.report)
	if !errors.Is(err, ErrStartTimeout) {
		t.Fatalf("PromptWithNudge() error = %v, want ErrStartTimeout", err)
	}
	want := []time.Duration{DefaultNudgeCaps.Grace, 5 * time.Second}
	if len(f.timeouts) != len(want) || f.timeouts[0] != want[0] || f.timeouts[1] != want[1] {
		t.Fatalf("WaitStart timeouts = %v, want %v", f.timeouts, want)
	}
}

func TestPromptWithNudge_ExpiredDeadline_NeverSubmits(t *testing.T) {
	f := &fakePrompter{screens: []string{"a"}}
	err := PromptWithNudge(f, "hi", DefaultNudgeCaps, f.clock.Add(-time.Second), f.now, f.report)
	if !errors.Is(err, ErrStartTimeout) {
		t.Fatalf("PromptWithNudge() error = %v, want ErrStartTimeout", err)
	}
	assertCalls(t, f.calls)
	assertDecisions(t, f.decisions, NudgeTimeout)
}
