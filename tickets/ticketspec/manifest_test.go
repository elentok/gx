package ticketspec

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeManifest writes manifest.yaml plus the body files and returns its path.
func writeManifest(t *testing.T, yamlText string, bodies map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, b := range bodies {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(b), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	p := filepath.Join(dir, "manifest.yaml")
	if err := os.WriteFile(p, []byte(yamlText), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func entry(slug, blockedBy string) string {
	s := "- slug: " + slug + "\n  type: implement\n  body: " + slug + ".md\n"
	if blockedBy != "" {
		s += "  blocked_by: " + blockedBy + "\n"
	}
	return s
}

func bodiesFor(slugs ...string) map[string]string {
	m := map[string]string{}
	for _, s := range slugs {
		m[s+".md"] = "body of " + s
	}
	return m
}

func TestParseManifest_Refusals(t *testing.T) {
	tests := []struct {
		name   string
		yaml   string
		bodies map[string]string
		want   string
	}{
		{"duplicate slug", entry("a", "") + entry("a", ""), bodiesFor("a"), `duplicate slug "a"`},
		{"commit slug", entry("commit", ""), bodiesFor("commit"), `"commit" is reserved`},
		{"digits-only slug", entry("07", ""), bodiesFor("07"), `digits only`},
		{"unknown field", entry("a", "") + "  parent: 01\n", bodiesFor("a"), "field parent not found"},
		{"missing body file", entry("a", ""), nil, `body file "a.md"`},
		{"unknown placeholder", entry("a", ""), map[string]string{"a.md": "see {{zzz}}"}, "unknown slug {{zzz}}"},
		{"unknown blocker ID", entry("a", "['09']"), bodiesFor("a"), `blocked_by ID "09"`},
		{"blocker with slash", entry("a", "[other/06]"), bodiesFor("a"), "names another epic"},
		{"unknown blocker slug", entry("a", "[nope]"), bodiesFor("a"), `"nope" is not a slug`},
		{"unknown type", "- slug: a\n  type: bogus\n  body: a.md\n", bodiesFor("a"), `unknown type "bogus"`},
		{"small window", entry("a", "") + "  expected_context_window: 500\n", bodiesFor("a"), "expected_context_window 500"},
		{"cycle", entry("a", "[b]") + entry("b", "[c]") + entry("c", "[a]"), bodiesFor("a", "b", "c"), "a -> b -> c -> a"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m, err := ParseManifest(writeManifest(t, tc.yaml, tc.bodies), []string{"01", "02"})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want containing %q", err, tc.want)
			}
			if m != nil {
				t.Fatalf("manifest must be nil on refusal")
			}
		})
	}
}

func TestParseManifest_ReportsEveryProblem(t *testing.T) {
	_, err := ParseManifest(writeManifest(t, entry("a", "")+entry("a", "")+entry("commit", ""), bodiesFor("a", "commit")), nil)
	if err == nil || !strings.Contains(err.Error(), "duplicate") || !strings.Contains(err.Error(), "reserved") {
		t.Fatalf("err = %v, want both problems", err)
	}
}

func slugsOf(m *Manifest) string {
	var s []string
	for _, e := range m.Entries {
		s = append(s, e.Slug)
	}
	return strings.Join(s, ",")
}

func TestParseManifest_Order(t *testing.T) {
	tests := []struct{ name, yaml, want string }{
		{"manifest order kept", entry("a", "") + entry("b", "") + entry("c", ""), "a,b,c"},
		{"blocker moves up", entry("a", "[b]") + entry("b", "") + entry("c", ""), "b,a,c"},
		{"transitive blockers move up", entry("a", "[b]") + entry("b", "[c]") + entry("c", ""), "c,b,a"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m, err := ParseManifest(writeManifest(t, tc.yaml, bodiesFor("a", "b", "c")), nil)
			if err != nil {
				t.Fatal(err)
			}
			if got := slugsOf(m); got != tc.want {
				t.Fatalf("order = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestParseManifest_BareNumberIsExistingID(t *testing.T) {
	m, err := ParseManifest(writeManifest(t, entry("a", "['07']"), bodiesFor("a")), []string{"07"})
	if err != nil {
		t.Fatal(err)
	}
	if got := m.Entries[0].BlockedBy; len(got) != 1 || got[0] != "07" {
		t.Fatalf("blocked_by = %v", got)
	}
	// An unquoted YAML number reads the same way.
	if _, err := ParseManifest(writeManifest(t, entry("a", "[07]"), bodiesFor("a")), []string{"07"}); err != nil {
		t.Fatalf("unquoted 07: %v", err)
	}
}

func TestRenderManifestBody(t *testing.T) {
	ids := map[string]string{"b": "05"}
	tests := []struct{ in, want string }{
		{"see {{b}}", "see 05"},
		{`quote \{{b}}`, "quote {{b}}"},
		{`verified at \{{commit}}`, "verified at {{commit}}"},
		{"verified at {{commit}}", "verified at {{commit}}"},
		{"tmpl {{.Name}}", "tmpl {{.Name}}"},
	}
	for _, tc := range tests {
		if got := RenderManifestBody(tc.in, ids); got != tc.want {
			t.Errorf("render(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestParseManifest_CommitPlaceholderIsNotUnknown(t *testing.T) {
	_, err := ParseManifest(writeManifest(t, entry("a", ""), map[string]string{"a.md": "verified at {{commit}} and \\{{zzz}}"}), nil)
	if err != nil {
		t.Fatal(err)
	}
}
