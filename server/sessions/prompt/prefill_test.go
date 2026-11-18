package prompt

import (
	"strings"
	"testing"
)

func TestScreenShowsPrompt(t *testing.T) {
	prompt := "when a question is asked by a queue agent it takes\n\nthe whole screen, so make it scroll"
	tests := []struct {
		name   string
		screen string
		want   bool
	}{
		{"wrapped in the input box", "──────\n❯ when a question is asked by a queue\n  agent it takes the whole screen\n──────", true},
		{"empty input box", "──────\n❯ \n──────", false},
		{"someone else's text", "──────\n❯ fix the bug\n──────", false},
		{"nothing drawn", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := screenShowsPrompt(tt.screen, prompt); got != tt.want {
				t.Errorf("screenShowsPrompt = %v, want %v", got, tt.want)
			}
		})
	}
	if screenShowsPrompt(strings.Repeat("x", 100), "  \n ") {
		t.Error("a blank prompt must never read as shown")
	}
}
