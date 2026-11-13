package oneoff

import (
	"testing"

	"github.com/elentok/gx/tickets"
)

func TestIsOneOffEpic_ByNameAndLegacyFile(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		path string
		want bool
	}{
		{"/store/proj/one-offs", true},
		{"/store/proj/one-offs-archived", true},
		{"/store/proj/ordinary", false},
	} {
		if got := IsOneOffEpic(tickets.Epic{Path: tc.path}); got != tc.want {
			t.Errorf("IsOneOffEpic(%s) = %v, want %v", tc.path, got, tc.want)
		}
	}
	legacy := tickets.Epic{Path: "/store/proj/old", Tickets: []tickets.Ticket{{Path: "/store/proj/old/issues/01-one-off.md"}}}
	if !IsOneOffEpic(legacy) {
		t.Error("old single-ticket one-off epic is not detected")
	}
}

func TestEpicAddress(t *testing.T) {
	t.Parallel()
	if got := EpicAddress("p:map/01"); got != "p:map" {
		t.Errorf("EpicAddress = %q, want p:map", got)
	}
}
