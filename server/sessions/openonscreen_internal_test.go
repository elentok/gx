package sessions

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/wire"
)

func TestWatchedSession(t *testing.T) {
	t.Parallel()
	watching := wire.FocusReport{Client: "web-1", Web: true, Focused: true, Session: "s1", Card: true}
	tests := []struct {
		name   string
		change func(*wire.FocusReport)
		closed bool
		want   string
	}{
		{name: "web page showing the card", want: "s1"},
		{name: "TUI report", change: func(r *wire.FocusReport) { r.Web = false }},
		{name: "unfocused", change: func(r *wire.FocusReport) { r.Focused = false }},
		{name: "no card", change: func(r *wire.FocusReport) { r.Card = false }},
		{name: "no session", change: func(r *wire.FocusReport) { r.Session = "" }},
		{name: "closed stream", closed: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := watching
			if tc.change != nil {
				tc.change(&r)
			}
			focus := NewFocusReports()
			focus.Connect(r.Client)
			focus.report(r)
			if tc.closed {
				focus.Disconnect(r.Client)
			}
			got, ok := focus.watchedSession(r.Client)
			if got != tc.want || ok != (tc.want != "") {
				t.Errorf("watchedSession = %q, %t; want %q", got, ok, tc.want)
			}
		})
	}
}

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
