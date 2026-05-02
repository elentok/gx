package git

import "testing"

func TestParseDecorations_Empty(t *testing.T) {
	if got := parseDecorations(""); got != nil {
		t.Fatalf("expected nil decorations, got %+v", got)
	}
}

func TestParseDecorations_TagLocalRemoteHead(t *testing.T) {
	got := parseDecorations("HEAD -> main, origin/main, tag: v1.0.0, feature/x")
	if len(got) != 4 {
		t.Fatalf("expected 4 decorations, got %+v", got)
	}
	if got[0] != (RefDecoration{Name: "main", Kind: RefDecorationLocalBranch}) {
		t.Fatalf("unexpected first decoration: %+v", got[0])
	}
	if got[1] != (RefDecoration{Name: "origin/main", Kind: RefDecorationRemoteBranch}) {
		t.Fatalf("unexpected second decoration: %+v", got[1])
	}
	if got[2] != (RefDecoration{Name: "v1.0.0", Kind: RefDecorationTag}) {
		t.Fatalf("unexpected third decoration: %+v", got[2])
	}
	if got[3] != (RefDecoration{Name: "feature/x", Kind: RefDecorationLocalBranch}) {
		t.Fatalf("unexpected fourth decoration: %+v", got[3])
	}
}

func TestParseDecorations_IgnoresWhitespace(t *testing.T) {
	got := parseDecorations("  tag: v1.0.0,   origin/main  ")
	if len(got) != 2 {
		t.Fatalf("expected 2 decorations, got %+v", got)
	}
	if got[0].Name != "v1.0.0" || got[1].Name != "origin/main" {
		t.Fatalf("unexpected decorations: %+v", got)
	}
}

func TestInitials_Empty(t *testing.T) {
	if got := initials(""); got != "?" {
		t.Fatalf("initials(\"\") = %q, want ?", got)
	}
}

func TestInitials_SingleWord(t *testing.T) {
	if got := initials("alice"); got != "AL" {
		t.Fatalf("initials(single) = %q, want AL", got)
	}
}

func TestInitials_SingleRune(t *testing.T) {
	if got := initials("q"); got != "Q" {
		t.Fatalf("initials(single rune) = %q, want Q", got)
	}
}

func TestInitials_TwoWords(t *testing.T) {
	if got := initials("Alice Baker"); got != "AB" {
		t.Fatalf("initials(two words) = %q, want AB", got)
	}
}

func TestInitials_MultiWordUsesFirstAndLast(t *testing.T) {
	if got := initials("Alice Beth Carter"); got != "AC" {
		t.Fatalf("initials(multiword) = %q, want AC", got)
	}
}
