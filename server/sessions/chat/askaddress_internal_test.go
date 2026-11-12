package chat

import "testing"

func TestAskAddress(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		m    Meta
		want string
	}{
		"run ticket":        {Meta{ID: "queue-x", RunTicket: "gx:e/01"}, "gx:e/01"},
		"fix of":            {Meta{ID: "s1", FixOf: "gx:e/02"}, "gx:e/02"},
		"plain with ticket": {Meta{ID: "s2", Ticket: "gx:e/03"}, "gx:e/03"},
		"plain without":     {Meta{ID: "s3"}, ""},
		"flow keeps no ask": {Meta{ID: "s4", Ticket: "gx:e/04", Flow: FlowRef{Flow: "design"}}, ""},
		"fix beats ticket":  {Meta{ID: "s5", FixOf: "gx:e/05", Ticket: "gx:e/06"}, "gx:e/05"},
	}
	for name, c := range cases {
		if got := askAddress(c.m); got != c.want {
			t.Errorf("%s: askAddress = %q, want %q", name, got, c.want)
		}
	}
}
