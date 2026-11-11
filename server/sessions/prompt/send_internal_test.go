package prompt

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/elentok/gx/agentrunner"
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
		{"launch prompt not delivered", fmt.Errorf("%w: %q: Unknown command:", agentrunner.ErrNotDelivered, "/x"), `prompt not delivered: "/x": Unknown command:`, true},
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

type failingPaster struct {
	failAt int
	pasted []string
}

func (p *failingPaster) Paste(s string) error {
	if len(p.pasted) == p.failAt {
		return errors.New("paste failed")
	}
	p.pasted = append(p.pasted, s)
	return nil
}

func TestImageSetPaste_MarksAnImageOnlyAfterItsPaste(t *testing.T) {
	t.Parallel()
	var marked []string
	imgs := imageSet{
		ids:   []string{"a", "b", "c"},
		paths: []string{"/a", "/b", "/c"},
		sent:  func(id string) { marked = append(marked, id) },
	}
	c := &failingPaster{failAt: 1}
	if err := imgs.paste(c); err == nil {
		t.Fatal("paste = nil, want the second paste's error")
	}
	if want := []string{"a"}; !slices.Equal(marked, want) {
		t.Errorf("marked = %v, want %v", marked, want)
	}
}

func TestRenameCommandArg(t *testing.T) {
	cases := []struct {
		in   string
		name string
		ok   bool
	}{
		{"/rename blabla", "blabla", true},
		{"  /name  my agent ", "my agent", true},
		{"/rename", "", false},
		{"/rename   ", "", false},
		{"/renamed x", "", false},
		{"hello /rename x", "", false},
		{"/rename a\nb", "", false},
	}
	for _, c := range cases {
		name, ok := renameCommandArg(c.in)
		if name != c.name && ok || ok != c.ok {
			t.Errorf("renameCommandArg(%q) = %q, %v; want %q, %v", c.in, name, ok, c.name, c.ok)
		}
	}
}
