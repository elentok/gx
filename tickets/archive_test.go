package tickets_test

import (
	"testing"

	"github.com/elentok/gx/tickets"
)

func TestArchiveKey_RoundTrip(t *testing.T) {
	cases := []struct {
		addr, name, key string
	}{
		{"proj:foo", "foo", "proj:.archive/foo"},
		{"proj:foo/01", "foo", "proj:.archive/foo/01"},
		{"proj:foo", "foo-2", "proj:.archive/foo-2"},
		{"proj:foo/01", "foo-2", "proj:.archive/foo-2/01"},
	}
	for _, c := range cases {
		key := tickets.ArchiveKey(c.addr, c.name)
		if key != c.key {
			t.Errorf("ArchiveKey(%q, %q) = %q, want %q", c.addr, c.name, key, c.key)
		}
		if !tickets.IsArchiveKey(key) {
			t.Errorf("IsArchiveKey(%q) = false", key)
		}
		back, ok := tickets.ArchiveKeyAddress(key)
		want := c.addr
		if c.name == "foo-2" {
			want = map[string]string{"proj:foo": "proj:foo-2", "proj:foo/01": "proj:foo-2/01"}[c.addr]
		}
		if !ok || back != want {
			t.Errorf("ArchiveKeyAddress(%q) = %q, %v, want %q", key, back, ok, want)
		}
	}
}

func TestArchiveEpicKey_DropsTicketSuffix(t *testing.T) {
	for in, want := range map[string]string{
		"proj:.archive/foo-2/01": "proj:.archive/foo-2",
		"proj:.archive/foo-2":    "proj:.archive/foo-2",
	} {
		if got := tickets.ArchiveEpicKey(in); got != want {
			t.Errorf("ArchiveEpicKey(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestArchiveKeyAddress_NotAnArchiveKey(t *testing.T) {
	for _, s := range []string{"proj:foo", "proj:foo/01", "proj", "", "proj:.archive", "proj:.archive/"} {
		if tickets.IsArchiveKey(s) {
			t.Errorf("IsArchiveKey(%q) = true", s)
		}
		if got, ok := tickets.ArchiveKeyAddress(s); ok {
			t.Errorf("ArchiveKeyAddress(%q) = %q, true", s, got)
		}
	}
}
