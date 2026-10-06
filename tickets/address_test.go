package tickets

import (
	"errors"
	"testing"
)

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
		{in: "06", ctx: AddressContext{Project: "gx"}, code: CodeEpicRequired},
		{in: "", ctx: ctx, code: CodeMalformedAddress},
		{in: "p:06", ctx: ctx, code: CodeMalformedAddress},
		{in: "epic/", ctx: ctx, code: CodeMalformedAddress},
		{in: "a:b:c/06", ctx: ctx, code: CodeMalformedAddress},
		{in: "epic/x6", ctx: ctx, code: CodeMalformedAddress},
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
