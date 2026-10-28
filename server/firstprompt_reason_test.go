package server

import (
	"fmt"
	"strings"
	"testing"

	"github.com/elentok/gx/agentrunner/claudeui"
)

func TestFailureReason(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		err         error
		wantReason  string
		clearNeeded bool
	}{
		{"slash step gave up in vim NORMAL", fmt.Errorf("step 0: %w", claudeui.ErrNotReadyToType), "input is in vim NORMAL mode", false},
		{"nudge gave up in vim NORMAL", fmt.Errorf("%w: %w", claudeui.ErrStuckSubmission, claudeui.ErrNotReadyToType), "input is in vim NORMAL mode", false},
		{"blocked before typing", errSessionBlocked, "session is blocked", false},
		{"input not empty", fmt.Errorf("%w: %w", claudeui.ErrStuckSubmission, claudeui.ErrInputNotEmpty), "input not empty." + clearInputHint, false},
		{"start timeout", claudeui.ErrStartTimeout, "agent never started", true},
		{"stuck", claudeui.ErrStuckSubmission, "prompt never reached the input", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reason, clearNeeded := failureReason(tt.err)
			if reason != tt.wantReason || clearNeeded != tt.clearNeeded {
				t.Errorf("failureReason() = %q, %v, want %q, %v", reason, clearNeeded, tt.wantReason, tt.clearNeeded)
			}
		})
	}
}

func TestWithClearHint(t *testing.T) {
	t.Parallel()
	if got := withClearHint("x"); !strings.HasPrefix(got, "x.") || !strings.HasSuffix(got, clearInputHint) {
		t.Errorf("withClearHint() = %q", got)
	}
}
