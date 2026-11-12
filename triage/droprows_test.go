package triage

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/elentok/gx/tickets/ticketspec"
)

const chainManifest = `- slug: a
  type: implement
  body: a.md
  from: proj:follow-ups/01
- slug: b
  type: implement
  body: b.md
  blocked_by: [a]
  from: [proj:follow-ups/02, proj:follow-ups/03]
- slug: c
  type: implement
  body: c.md
  blocked_by: [b]
  from: proj:follow-ups/04
`

func dropFixture(t *testing.T, manifest string) string {
	t.Helper()
	dir := t.TempDir()
	for _, f := range []string{"a.md", "b.md", "c.md"} {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("body "+f), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	p := filepath.Join(dir, "manifest.yaml")
	if err := os.WriteFile(p, []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func entriesBySlug(t *testing.T, p string) map[string]ticketspec.ManifestEntry {
	t.Helper()
	es, err := ticketspec.LoadEntries(p)
	if err != nil {
		t.Fatal(err)
	}
	m := map[string]ticketspec.ManifestEntry{}
	for _, e := range es {
		m[e.Slug] = e
	}
	return m
}

func TestDropRows_MiddleOfChainClosesTheGap(t *testing.T) {
	p := dropFixture(t, chainManifest)
	// The group entry b is dropped whole by naming only one of its rows.
	dropped, err := DropRows(p, []string{"proj:follow-ups/03"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(dropped, []string{"b"}) {
		t.Fatalf("dropped = %v", dropped)
	}
	m := entriesBySlug(t, p)
	if _, ok := m["b"]; ok || len(m) != 2 {
		t.Fatalf("entries = %v", m)
	}
	if got := m["c"].BlockedBy; !slices.Equal(got, []string{"a"}) {
		t.Errorf("c blocked_by = %v, want [a]", got)
	}
	if got := m["c"].From; !slices.Equal(got, []string{"proj:follow-ups/04"}) {
		t.Errorf("c from = %v", got)
	}
	dir := filepath.Dir(p)
	if _, err := os.Stat(filepath.Join(dir, "b.md")); !os.IsNotExist(err) {
		t.Errorf("b.md still exists: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "a.md")); err != nil {
		t.Errorf("a.md removed: %v", err)
	}
}

func TestDropRows_TwoRowsAtOnceLeavesNoDanglingSlug(t *testing.T) {
	p := dropFixture(t, chainManifest)
	dropped, err := DropRows(p, []string{"proj:follow-ups/01", "proj:follow-ups/02"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(dropped, []string{"a", "b"}) {
		t.Fatalf("dropped = %v", dropped)
	}
	m := entriesBySlug(t, p)
	if len(m) != 1 || len(m["c"].BlockedBy) != 0 {
		t.Fatalf("entries = %+v", m)
	}
}

func TestDropRows_UnknownRowIsANoOp(t *testing.T) {
	p := dropFixture(t, chainManifest)
	before, _ := os.ReadFile(p)
	dropped, err := DropRows(p, []string{"proj:follow-ups/99"}, nil)
	if err != nil || len(dropped) != 0 {
		t.Fatalf("dropped %v, err %v", dropped, err)
	}
	after, _ := os.ReadFile(p)
	if string(before) != string(after) {
		t.Error("manifest changed")
	}
}

func TestDropRows_ShortFromAndComments(t *testing.T) {
	short := func(a string) string { return strings.TrimPrefix(a, "proj:") }
	tests := []struct {
		name, manifest, row string
		wantDropped         []string
	}{
		{"short from, full row", "# keep me\n- slug: a\n  type: implement\n  body: a.md\n  from: follow-ups/01\n# tail\n- slug: c\n  type: implement\n  body: c.md\n  blocked_by: [a]\n", "proj:follow-ups/01", []string{"a"}},
		{"full from, short row", "# keep me\n- slug: a\n  type: implement\n  body: a.md\n  from: proj:follow-ups/01\n# tail\n- slug: c\n  type: implement\n  body: c.md\n  blocked_by: [a]\n", "follow-ups/01", []string{"a"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := dropFixture(t, tt.manifest)
			dropped, err := DropRows(p, []string{tt.row}, short)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(dropped, tt.wantDropped) {
				t.Fatalf("dropped = %v", dropped)
			}
			got, _ := os.ReadFile(p)
			if !strings.Contains(string(got), "# tail") {
				t.Errorf("comment lost:\n%s", got)
			}
			if m := entriesBySlug(t, p); len(m["c"].BlockedBy) != 0 {
				t.Errorf("c blocked_by = %v", m["c"].BlockedBy)
			}
		})
	}
}
