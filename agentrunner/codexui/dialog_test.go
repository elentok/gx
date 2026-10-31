package codexui_test

import (
	"strings"
	"testing"

	"github.com/elentok/gx/agentrunner/codexui"
)

func TestParseDialog(t *testing.T) {
	t.Parallel()
	rows := func(s string) []string { return strings.Split(s, "\n") }
	for _, tc := range []struct {
		name       string
		rows       []string
		approvable bool
		body       string
	}{
		{"title and message", rows("› \n\nBlocked by hook\nrg foo\n\n"), true, "rg foo"},
		{"multi-line message", rows("Blocked by hook\n  git push\n  origin main\n\nnext"), true, "git push origin main"},
		{"title in another case", rows("blocked by hook\nrg foo"), true, "rg foo"},
		{"title with no message", rows("Blocked by hook\n\n"), false, ""},
		{"unknown screen", rows("› Try \"fix the failing test\"\n  gpt-6 · ~/repo"), false, ""},
		{"empty screen", nil, false, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			d := codexui.ParseDialog(tc.rows)
			if d.Approvable != tc.approvable || d.Body != tc.body {
				t.Fatalf("ParseDialog = %+v, want approvable %v body %q", d, tc.approvable, tc.body)
			}
			if d.Approvable != (d.ID != "") {
				t.Errorf("ID %q for approvable %v", d.ID, d.Approvable)
			}
		})
	}
}

func TestParseDialog_IDFollowsTheMessage(t *testing.T) {
	t.Parallel()
	a := codexui.ParseDialog([]string{"Blocked by hook", "rg foo"})
	b := codexui.ParseDialog([]string{"", "Blocked by hook", "rg foo", "", "tip"})
	c := codexui.ParseDialog([]string{"Blocked by hook", "rg bar"})
	if a.ID != b.ID || a.ID == c.ID {
		t.Errorf("ids %q %q %q: want the first two equal and the third different", a.ID, b.ID, c.ID)
	}
}
