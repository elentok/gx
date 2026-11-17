package agentrunner_test

import (
	"testing"

	"github.com/elentok/gx/agentrunner"
)

func TestNormalise(t *testing.T) {
	tests := []struct {
		in, want   string
		deprecated bool
	}{
		{"herdr", "auto", true},
		{"auto", "auto", false},
		{"pty", "pty", false},
		{"headless", "headless", false},
		{"", "", false},
	}
	for _, tt := range tests {
		got, dep := agentrunner.Normalise(tt.in)
		if got != tt.want || dep != tt.deprecated {
			t.Errorf("Normalise(%q) = %q, %v; want %q, %v", tt.in, got, dep, tt.want, tt.deprecated)
		}
	}
	if err := agentrunner.ValidateSetting("herdr"); err != nil {
		t.Errorf("ValidateSetting(herdr) = %v", err)
	}
}
