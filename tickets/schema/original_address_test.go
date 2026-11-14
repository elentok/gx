package schema

import "testing"

func TestOriginalAddressesFirst(t *testing.T) {
	tests := []struct {
		name string
		in   OriginalAddresses
		want string
	}{
		{"empty", nil, ""},
		{"one", OriginalAddresses{"gx:e/01"}, "gx:e/01"},
		{"several", OriginalAddresses{"gx:e/01", "gx:e/02"}, "gx:e/01"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.in.First(); got != tc.want {
				t.Fatalf("First() = %q, want %q", got, tc.want)
			}
		})
	}
}
