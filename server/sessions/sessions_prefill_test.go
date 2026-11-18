package sessions

import (
	"strings"
	"testing"

	"github.com/elentok/gx/harness"
)

func TestCanPrefill(t *testing.T) {
	old := claudeSupportsPrefill
	t.Cleanup(func() { claudeSupportsPrefill = old })
	supported := true
	claudeSupportsPrefill = func([]string) bool { return supported }
	mg := &Manager{}
	claude := string(harness.Claude)
	tests := []struct {
		name      string
		meta      Meta
		supported bool
		want      bool
	}{
		{"plain text on a claude that knows the flag", Meta{Harness: claude, FirstPrompt: "fix the bug"}, true, true},
		{"claude that does not know the flag", Meta{Harness: claude, FirstPrompt: "fix the bug"}, false, false},
		{"no prompt", Meta{Harness: claude}, true, false},
		{"slash command", Meta{Harness: claude, FirstPrompt: "/gx-design x"}, true, false},
		{"images go in as pastes", Meta{Harness: claude, FirstPrompt: "look", FirstImages: []string{"a"}}, true, false},
		{"too long for argv", Meta{Harness: claude, FirstPrompt: strings.Repeat("x", harness.PrefillMaxBytes+1)}, true, false},
		{"another harness", Meta{Harness: string(harness.Codex), FirstPrompt: "fix the bug"}, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			supported = tt.supported
			if got := mg.canPrefill(&tt.meta); got != tt.want {
				t.Errorf("canPrefill = %v, want %v", got, tt.want)
			}
		})
	}
}
