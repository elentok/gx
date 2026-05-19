package log

import (
	"testing"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/git"
)

func TestCompileHideRefs(t *testing.T) {
	refs := compileHideRefs([]string{"^origin/", "[invalid"})
	// "[invalid" is invalid regex, silently skipped
	if len(refs) != 1 {
		t.Fatalf("expected 1 compiled ref, got %d", len(refs))
	}
	if !refs[0].MatchString("origin/main") {
		t.Error("pattern should match 'origin/main'")
	}
}

func TestCompileRefRules(t *testing.T) {
	rules := []config.ImportantRefRule{
		{Patterns: []string{"^main$"}, Color: "yellow"},
		{Patterns: []string{"[invalid"}, Color: "red"},     // invalid pattern → rule skipped
		{Patterns: []string{"^v\\d"}, Color: "invalidhex"}, // invalid color → rule skipped
	}
	compiled := compileRefRules(rules)
	if len(compiled) != 1 {
		t.Fatalf("expected 1 compiled rule, got %d", len(compiled))
	}
	c, ok := matchRefRule("main", compiled)
	if !ok || c == nil {
		t.Error("expected match for 'main'")
	}
	_, ok = matchRefRule("feature/x", compiled)
	if ok {
		t.Error("expected no match for 'feature/x'")
	}
}

func TestSortDecorations(t *testing.T) {
	rules := compileRefRules([]config.ImportantRefRule{
		{Patterns: []string{"^main$"}, Color: "yellow"},
	})

	decs := []git.RefDecoration{
		{Name: "feature/x"},
		{Name: "main"},
		{Name: "other"},
	}

	sorted := sortDecorations(decs, rules)
	if sorted[0].Name != "main" {
		t.Errorf("expected 'main' first, got %q", sorted[0].Name)
	}

	// no rules — original order preserved
	orig := sortDecorations(decs, nil)
	if orig[0].Name != "feature/x" {
		t.Errorf("expected original order preserved, got %q", orig[0].Name)
	}
}
