package ui

import "testing"

func TestIcons_TicketNerdFontCodepoints(t *testing.T) {
	icons := Icons(true)
	cases := []struct {
		name string
		got  string
		want rune
	}{
		{"TicketOpen", icons.TicketOpen, '\uf10c'},
		{"TicketClaimed", icons.TicketClaimed, '\uf042'},
		{"TicketNeedsAnswer", icons.TicketNeedsAnswer, '\uf059'},
		{"TicketNeedsRepair", icons.TicketNeedsRepair, '\uf071'},
		{"TicketBlocked", icons.TicketBlocked, '\uf28e'},
		{"TicketDone", icons.TicketDone, '\uf00c'},
		{"TicketError", icons.TicketError, '\uf057'},
		{"TicketPaused", icons.TicketPaused, '\uf28c'},
		{"SuggestedAction", icons.SuggestedAction, '\uf0eb'},
	}
	for _, c := range cases {
		if got := []rune(c.got); len(got) != 1 || got[0] != c.want {
			t.Errorf("%s = %q, want %q", c.name, c.got, string(c.want))
		}
	}
}

func TestIcons_ASCIITriangleDirection(t *testing.T) {
	icons := Icons(false)
	if icons.TriangleCollapsed != "▶" {
		t.Errorf("TriangleCollapsed = %q, want %q", icons.TriangleCollapsed, "▶")
	}
	if icons.TriangleExpanded != "▼" {
		t.Errorf("TriangleExpanded = %q, want %q", icons.TriangleExpanded, "▼")
	}
}
