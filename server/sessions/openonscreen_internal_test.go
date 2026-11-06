package sessions

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/wire"
)

func TestOpenOnScreen(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		web, tui bool
		afk      bool
		want     bool
	}{
		{"nothing open", false, false, false, false},
		{"nothing open, AFK", false, false, true, false},
		{"web open", true, false, false, true},
		{"web open, AFK", true, false, true, true},
		{"TUI open", false, true, false, true},
		{"TUI open, AFK", false, true, true, false},
		{"both open, AFK", true, true, true, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			if tc.afk {
				if err := os.WriteFile(filepath.Join(dir, "afk.json"), []byte(`{"on":true}`), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			afk, err := queue.OpenAFK(dir, nil)
			if err != nil {
				t.Fatal(err)
			}
			focus := NewFocusReports()
			for client, web := range map[string]bool{"tui-1": false, "web-1": true} {
				if (web && !tc.web) || (!web && !tc.tui) {
					continue
				}
				focus.Connect(client)
				focus.report(wire.FocusReport{Client: client, Focused: true, Session: "s1", Web: web})
			}
			mg := &Manager{focus: focus, afk: afk}
			if got := mg.OpenOnScreen("s1"); got != tc.want {
				t.Errorf("OpenOnScreen = %t, want %t", got, tc.want)
			}
		})
	}
}
