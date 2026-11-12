package tickets_test

import (
	"strings"
	"testing"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

func parseWithOriginal(t *testing.T, field string) schema.Ticket {
	t.Helper()
	raw := "---\nid: \"01\"\nstatus: open\ntype: implement\n" + field + "---\n\n# 01\n"
	tk, err := schema.ParseTicketFromRaw(raw, "01-x.md")
	if err != nil {
		t.Fatalf("parse %q: %v", field, err)
	}
	return tk
}

func TestOriginalAddress_StringAndListBothLoad(t *testing.T) {
	a := tickets.Address{Project: "p", Epic: "follow-ups", ID: "01"}
	b := tickets.Address{Project: "p", Epic: "follow-ups", ID: "02"}
	other := tickets.Address{Project: "p", Epic: "follow-ups", ID: "03"}

	single := tickets.Ticket{OriginalAddress: parseWithOriginal(t, "original_address: p:follow-ups/01\n").OriginalAddress}
	list := tickets.Ticket{OriginalAddress: parseWithOriginal(t, "original_address: [p:follow-ups/01, p:follow-ups/02]\n").OriginalAddress}

	if !tickets.IsOriginal(single, a) || tickets.IsOriginal(single, b) {
		t.Errorf("string form: IsOriginal(a, b) = %v, %v; want true, false", tickets.IsOriginal(single, a), tickets.IsOriginal(single, b))
	}
	if !tickets.IsOriginal(list, a) || !tickets.IsOriginal(list, b) || tickets.IsOriginal(list, other) {
		t.Errorf("list form must match every entry and nothing else")
	}
}

func TestOriginalAddress_WriteBackKeepsShape(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"original_address: p:follow-ups/01\n", "original_address: p:follow-ups/01\n"},
		{"original_address: [p:follow-ups/01, p:follow-ups/02]\n", "- p:follow-ups/02"},
	} {
		out, err := schema.MarshalTicket(parseWithOriginal(t, c.in), "# 01\n")
		if err != nil || !strings.Contains(string(out), c.want) {
			t.Errorf("write-back of %q = %q, %v; want it to contain %q", c.in, out, err, c.want)
		}
	}
}
