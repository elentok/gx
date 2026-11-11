package archivekey_test

import (
	"testing"

	"github.com/elentok/gx/tickets/archivekey"
)

func TestNew_RoundTrip(t *testing.T) {
	cases := []struct {
		addr, name, key string
	}{
		{"proj:foo", "foo", "proj:.archive/foo"},
		{"proj:foo/01", "foo", "proj:.archive/foo/01"},
		{"proj:foo", "foo-2", "proj:.archive/foo-2"},
		{"proj:foo/01", "foo-2", "proj:.archive/foo-2/01"},
	}
	for _, c := range cases {
		key := archivekey.New(c.addr, c.name)
		if key != c.key {
			t.Errorf("New(%q, %q) = %q, want %q", c.addr, c.name, key, c.key)
		}
		if !archivekey.Is(key) {
			t.Errorf("Is(%q) = false", key)
		}
		back, ok := archivekey.Address(key)
		want := c.addr
		if c.name == "foo-2" {
			want = map[string]string{"proj:foo": "proj:foo-2", "proj:foo/01": "proj:foo-2/01"}[c.addr]
		}
		if !ok || back != want {
			t.Errorf("Address(%q) = %q, %v, want %q", key, back, ok, want)
		}
	}
}

func TestEpic_DropsTicketSuffix(t *testing.T) {
	for in, want := range map[string]string{
		"proj:.archive/foo-2/01": "proj:.archive/foo-2",
		"proj:.archive/foo-2":    "proj:.archive/foo-2",
	} {
		if got := archivekey.Epic(in); got != want {
			t.Errorf("Epic(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAddress_NotAnArchiveKey(t *testing.T) {
	for _, s := range []string{"proj:foo", "proj:foo/01", "proj", "", "proj:.archive", "proj:.archive/"} {
		if archivekey.Is(s) {
			t.Errorf("Is(%q) = true", s)
		}
		if got, ok := archivekey.Address(s); ok {
			t.Errorf("Address(%q) = %q, true", s, got)
		}
	}
}
