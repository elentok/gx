package stage

import (
	"strings"
	"testing"
)

func TestParseUnifiedDiff_TracksHunksAndChangedLines(t *testing.T) {
	raw := strings.Join([]string{
		"diff --git a/a.txt b/a.txt",
		"index 1111111..2222222 100644",
		"--- a/a.txt",
		"+++ b/a.txt",
		"@@ -1,3 +1,3 @@",
		" one",
		"-two",
		"+two changed",
		" three",
	}, "\n") + "\n"

	p := parseUnifiedDiff(raw)
	if len(p.Hunks) != 1 {
		t.Fatalf("hunks = %d, want 1", len(p.Hunks))
	}
	if len(p.Changed) != 2 {
		t.Fatalf("changed = %d, want 2", len(p.Changed))
	}
	if p.Changed[0].Prefix != '-' || p.Changed[1].Prefix != '+' {
		t.Fatalf("unexpected changed prefixes: %#v", p.Changed)
	}
}

func TestBuildSingleLinePatch(t *testing.T) {
	raw := strings.Join([]string{
		"diff --git a/a.txt b/a.txt",
		"index 1111111..2222222 100644",
		"--- a/a.txt",
		"+++ b/a.txt",
		"@@ -1,2 +1,2 @@",
		"-old",
		"+new",
	}, "\n") + "\n"
	p := parseUnifiedDiff(raw)
	patch, err := buildSingleLinePatch(p, 1)
	if err != nil {
		t.Fatalf("buildSingleLinePatch: %v", err)
	}
	if !strings.Contains(patch, "@@ -") {
		t.Fatalf("unexpected hunk header in patch:\n%s", patch)
	}
	if !strings.Contains(patch, "+new") {
		t.Fatalf("patch missing selected line:\n%s", patch)
	}
}
