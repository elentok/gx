package nativerunner_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/agentrunner/claudeui"
	"github.com/elentok/gx/nativerunner/holder"
	"github.com/elentok/gx/testutil/agentfake"
)

// TestPty_Deliver runs a PTY prompt through deliver.Deliver over the real holder
// and the Claude TUI fake. Each row counts the Enters the fake received, so a
// stacked Enter or a retype shows up as a count.
func TestPty_Deliver(t *testing.T) {
	t.Parallel()
	rows := []struct {
		name    string
		text    string
		swallow bool
		// resized makes every screen read untrusted, as after a PTY resize.
		resized    bool
		wantErr    error
		wantEnters int
	}{
		{name: "plain prompt lands with one Enter", text: "do it", wantEnters: 1},
		{name: "plain prompt with a swallowed Enter gets one bare-Enter nudge", text: "do it", swallow: true, wantEnters: 2},
		{name: "slash command lands with one Enter", text: "/model", wantEnters: 1},
		{name: "slash command with a swallowed Enter gets one settle Enter", text: "/model", swallow: true, wantEnters: 2},
		{name: "undecided slash read is not delivered and presses no more Enters", text: "/model", swallow: true, resized: true, wantErr: agentrunner.ErrNotDelivered, wantEnters: 1},
		{name: "compact takes the nudge path", text: "/compact", wantEnters: 1},
		{name: "compact with arguments takes the nudge path", text: "/compact focus", wantEnters: 1},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			p, s := launchPty(t, claudeui.LaunchProofCaps{})
			p.PromptGrace = 500 * time.Millisecond
			if row.swallow {
				ptyControl(t, p, s, agentfake.Control{Action: agentfake.ActionSwallowEnter})
			}
			if row.resized {
				if err := os.WriteFile(filepath.Join(p.Root, s.Label, holder.ResizeBeforeReadFile), nil, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			err := p.Prompt(s, row.text)
			if !errors.Is(err, row.wantErr) {
				t.Fatalf("Prompt = %v; want %v", err, row.wantErr)
			}
			keys := fakeKeysTyped(t, p, s)
			if got := strings.Count(keys, row.text); got != 1 {
				t.Errorf("text typed %d times in %q; want once", got, keys)
			}
			if got := strings.Count(keys, "\r"); got != row.wantEnters {
				t.Errorf("Enters = %d in %q; want %d", got, keys, row.wantEnters)
			}
		})
	}
}
