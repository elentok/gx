package ticketspec

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCaseClash(t *testing.T) {
	cases := []struct {
		name     string
		existing string
		missing  string
		want     string
	}{
		{"same stem, other extension", "Foo.tsx", "Foo.css", ""},
		{"same stem, test suffix", "Foo.tsx", "Foo.test.tsx", ""},
		{"same stem, different case and extension", "foo.tsx", "Foo.ts", ""},
		{"case-only difference", "Foo.tsx", "foo.tsx", "Foo.tsx"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, tc.existing), nil, 0o644); err != nil {
				t.Fatal(err)
			}
			if got := caseClash(filepath.Join(dir, tc.missing)); got != tc.want {
				t.Fatalf("caseClash(%q) = %q, want %q", tc.missing, got, tc.want)
			}
		})
	}
}
