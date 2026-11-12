package tickets

import (
	"errors"
	"strings"
	"testing"

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
	a := Address{Project: "p", Epic: "follow-ups", ID: "01"}
	b := Address{Project: "p", Epic: "follow-ups", ID: "02"}
	other := Address{Project: "p", Epic: "follow-ups", ID: "03"}

	single := Ticket{OriginalAddress: parseWithOriginal(t, "original_address: p:follow-ups/01\n").OriginalAddress}
	list := Ticket{OriginalAddress: parseWithOriginal(t, "original_address: [p:follow-ups/01, p:follow-ups/02]\n").OriginalAddress}

	if !IsOriginal(single, a) || IsOriginal(single, b) {
		t.Errorf("string form: IsOriginal(a, b) = %v, %v; want true, false", IsOriginal(single, a), IsOriginal(single, b))
	}
	if !IsOriginal(list, a) || !IsOriginal(list, b) || IsOriginal(list, other) {
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

func TestParseAddress(t *testing.T) {
	t.Parallel()
	ctx := AddressContext{Project: "gx", Epic: "daemon"}
	tests := []struct {
		in   string
		ctx  AddressContext
		want string
		code string
	}{
		{in: "other:epic/06", ctx: ctx, want: "other:epic/06"},
		{in: "epic/06", ctx: ctx, want: "gx:epic/06"},
		{in: "06", ctx: ctx, want: "gx:daemon/06"},
		{in: "10b1", ctx: ctx, want: "gx:daemon/10b1"},
		{in: "6", ctx: ctx, want: "gx:daemon/06"},
		{in: "6a", ctx: ctx, want: "gx:daemon/06a"},
		{in: "epic/6a", ctx: ctx, want: "gx:epic/06a"},
		{in: "6a1", ctx: ctx, want: "gx:daemon/06a1"},
		{in: "06", ctx: AddressContext{Project: "gx"}, code: CodeEpicRequired},
		{in: "", ctx: ctx, code: CodeMalformedAddress},
		{in: "p:06", ctx: ctx, code: CodeMalformedAddress},
		{in: "epic/", ctx: ctx, code: CodeMalformedAddress},
		{in: "a:b:c/06", ctx: ctx, code: CodeMalformedAddress},
		{in: "epic/x6", ctx: ctx, code: CodeMalformedAddress},
		{in: "../06", ctx: ctx, code: CodeMalformedAddress},
		{in: "./06", ctx: ctx, code: CodeMalformedAddress},
		{in: "..:epic/06", ctx: ctx, code: CodeMalformedAddress},
		{in: "p:../06", ctx: ctx, code: CodeMalformedAddress},
		{in: "06", ctx: AddressContext{Project: "gx", Epic: ".."}, code: CodeMalformedAddress},
		{in: "p:a..b/06", ctx: ctx, want: "p:a..b/06"},
		{in: "p:.hidden/06", ctx: ctx, want: "p:.hidden/06"},
	}
	for _, tt := range tests {
		got, err := ParseAddress(tt.in, tt.ctx)
		if tt.code != "" {
			var ae *AddressError
			if !errors.As(err, &ae) || ae.Code != tt.code {
				t.Errorf("ParseAddress(%q) err = %v, want code %s", tt.in, err, tt.code)
			}
			continue
		}
		if err != nil || got.String() != tt.want {
			t.Errorf("ParseAddress(%q) = %v, %v; want %s", tt.in, got, err, tt.want)
		}
	}
}
