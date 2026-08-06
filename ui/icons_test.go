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
		{"TicketNeedsInfo", icons.TicketNeedsInfo, '\uf059'},
		{"TicketNeedsAttention", icons.TicketNeedsAttention, '\uf071'},
		{"TicketBlocked", icons.TicketBlocked, '\uf28e'},
		{"TicketDone", icons.TicketDone, '\uf00c'},
		{"TicketError", icons.TicketError, '\uf057'},
		{"TicketPaused", icons.TicketPaused, '\uf28c'},
	}
	for _, c := range cases {
		if got := []rune(c.got); len(got) != 1 || got[0] != c.want {
			t.Errorf("%s = %q, want %q", c.name, c.got, string(c.want))
		}
	}
}
