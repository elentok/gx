package claudeui

import (
	"errors"
	"testing"
	"time"
)

// fakePrompter is an in-memory Prompter. starts scripts WaitStart's answers in
// order (false once exhausted); screens scripts Screen's answers in order (the
// last one repeats). Every WaitStart advances clock by the timeout it was given.
type fakePrompter struct {
	clock    time.Time
	starts   []bool
	screens  []string
	calls    []string
	timeouts []time.Duration
}

func (f *fakePrompter) Submit(text string) error {
	f.calls = append(f.calls, "submit:"+text)
	return nil
}

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
	if err := PromptWithNudge(f, "hi", DefaultNudgeCaps, time.Time{}, f.now); err != nil {
		t.Fatalf("PromptWithNudge() error = %v", err)
	}
	assertCalls(t, f.calls, "submit:hi", "wait")
}

func TestPromptWithNudge_ChangedScreen_NudgesWithEnter(t *testing.T) {
	f := &fakePrompter{starts: []bool{false, true}, screens: []string{"before", "typed"}}
	if err := PromptWithNudge(f, "hi", DefaultNudgeCaps, time.Time{}, f.now); err != nil {
		t.Fatalf("PromptWithNudge() error = %v", err)
	}
	assertCalls(t, f.calls, "submit:hi", "wait", "enter", "wait")
}

func TestPromptWithNudge_UnchangedScreen_Retypes(t *testing.T) {
	f := &fakePrompter{starts: []bool{false, true}, screens: []string{"empty"}}
	if err := PromptWithNudge(f, "hi", DefaultNudgeCaps, time.Time{}, f.now); err != nil {
		t.Fatalf("PromptWithNudge() error = %v", err)
	}
	assertCalls(t, f.calls, "submit:hi", "wait", "submit:hi", "wait")
}

func TestPromptWithNudge_RetypeCap_ReturnsStuckSubmission(t *testing.T) {
	f := &fakePrompter{screens: []string{"empty"}}
	err := PromptWithNudge(f, "hi", DefaultNudgeCaps, time.Time{}, f.now)
	if !errors.Is(err, ErrStuckSubmission) {
		t.Fatalf("PromptWithNudge() error = %v, want ErrStuckSubmission", err)
	}
	assertCalls(t, f.calls, "submit:hi", "wait", "submit:hi", "wait", "submit:hi", "wait")
}

func TestPromptWithNudge_NudgeCap_ReturnsStartTimeout(t *testing.T) {
	f := &fakePrompter{screens: []string{"1", "2", "3", "4", "5"}}
	err := PromptWithNudge(f, "hi", DefaultNudgeCaps, time.Time{}, f.now)
	if !errors.Is(err, ErrStartTimeout) || errors.Is(err, ErrStuckSubmission) {
		t.Fatalf("PromptWithNudge() error = %v, want ErrStartTimeout only", err)
	}
	assertCalls(t, f.calls, "submit:hi", "wait", "enter", "wait", "enter", "wait", "enter", "wait")
}

func TestPromptWithNudge_CallerCaps(t *testing.T) {
	caps := NudgeCaps{Grace: time.Second, Nudges: 2, Retypes: 1}
	f := &fakePrompter{screens: []string{"empty", "typed", "typed"}}
	err := PromptWithNudge(f, "hi", caps, time.Time{}, f.now)
	if !errors.Is(err, ErrStartTimeout) {
		t.Fatalf("PromptWithNudge() error = %v, want ErrStartTimeout", err)
	}
	assertCalls(t, f.calls, "submit:hi", "wait", "enter", "wait", "submit:hi", "wait")
	for _, got := range f.timeouts {
		if got != caps.Grace {
			t.Fatalf("WaitStart timeouts = %v, want %v each", f.timeouts, caps.Grace)
		}
	}

	f = &fakePrompter{screens: []string{"empty"}}
	err = PromptWithNudge(f, "hi", caps, time.Time{}, f.now)
	if !errors.Is(err, ErrStuckSubmission) {
		t.Fatalf("PromptWithNudge() error = %v, want ErrStuckSubmission", err)
	}
	assertCalls(t, f.calls, "submit:hi", "wait", "submit:hi", "wait")
}

func TestPromptWithNudge_HonoursDeadline(t *testing.T) {
	f := &fakePrompter{screens: []string{"1", "2", "3", "4"}}
	deadline := f.clock.Add(DefaultNudgeCaps.Grace + 5*time.Second)
	err := PromptWithNudge(f, "hi", DefaultNudgeCaps, deadline, f.now)
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
	err := PromptWithNudge(f, "hi", DefaultNudgeCaps, f.clock.Add(-time.Second), f.now)
	if !errors.Is(err, ErrStartTimeout) {
		t.Fatalf("PromptWithNudge() error = %v, want ErrStartTimeout", err)
	}
	assertCalls(t, f.calls)
}
