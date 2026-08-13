package agentfake

import (
	"strings"
	"testing"
	"time"
)

func TestParseArgs_DefaultsToIdle(t *testing.T) {
	opts, err := ParseArgs(nil)
	if err != nil {
		t.Fatalf("ParseArgs: %v", err)
	}
	if opts.Mode != ModeIdle {
		t.Fatalf("Mode = %q, want %q", opts.Mode, ModeIdle)
	}
}

func TestParseArgs_RejectsUnknownMode(t *testing.T) {
	if _, err := ParseArgs([]string{"--mode=bogus"}); err == nil {
		t.Fatal("expected error for unknown mode")
	}
}

func TestRun_Idle_EmitsIdleTitleOnceAndIgnoresInput(t *testing.T) {
	var out strings.Builder
	err := Run(Options{Mode: ModeIdle}, strings.NewReader("hello\n"), &out)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if out.String() != idleTitle {
		t.Fatalf("output = %q, want %q", out.String(), idleTitle)
	}
}

func TestRun_SlowWorking_WorksThenGoesIdle(t *testing.T) {
	var out strings.Builder
	start := time.Now()
	err := Run(Options{Mode: ModeSlowWorking, Duration: 10 * time.Millisecond}, strings.NewReader(""), &out)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if elapsed < 10*time.Millisecond {
		t.Fatalf("elapsed = %s, want >= 10ms", elapsed)
	}
	want := workingTitle + idleTitle
	if out.String() != want {
		t.Fatalf("output = %q, want %q", out.String(), want)
	}
}

func TestRun_Compact_GoesWorkingOnEachPromptThenIdleAgain(t *testing.T) {
	var out strings.Builder
	err := Run(Options{Mode: ModeCompact, Duration: 5 * time.Millisecond}, strings.NewReader("/compact\nanother prompt\n"), &out)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	want := idleTitle + workingTitle + idleTitle + workingTitle + idleTitle
	if out.String() != want {
		t.Fatalf("output = %q, want %q", out.String(), want)
	}
}

func TestRun_Stall_NeverEmitsWorkingTitleAfterInput(t *testing.T) {
	var out strings.Builder
	err := Run(Options{Mode: ModeStall}, strings.NewReader("prompt\n"), &out)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if out.String() != idleTitle {
		t.Fatalf("output = %q, want %q (no working title)", out.String(), idleTitle)
	}
}
