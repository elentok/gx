package brief

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeIndex(t *testing.T, entries ...indexEntry) string {
	t.Helper()
	var b strings.Builder
	for _, e := range entries {
		line, _ := json.Marshal(e)
		b.Write(line)
		b.WriteByte('\n')
	}
	path := filepath.Join(t.TempDir(), "index.jsonl")
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestIndexAttempt_Lookup(t *testing.T) {
	t.Parallel()
	args := []string{"./..."}
	match := indexEntry{Tool: "go-test", TreeID: "t1", Args: args}
	for _, tc := range []struct {
		name    string
		entries []indexEntry
		tree    string
		want    int
	}{
		{"no lines", nil, "t1", 1},
		{"other tool", []indexEntry{{Tool: "vitest", TreeID: "t1", Args: args, Attempt: 4}}, "t1", 1},
		{"other args", []indexEntry{{Tool: "go-test", TreeID: "t1", Args: []string{"./x"}, Attempt: 4}}, "t1", 1},
		{"other tree", []indexEntry{{Tool: "go-test", TreeID: "t2", Args: args, Attempt: 4}}, "t1", 1},
		{"old line without attempt reads as 1", []indexEntry{match}, "t1", 2},
		{"newest match wins", []indexEntry{{Tool: "go-test", TreeID: "t1", Args: args, Attempt: 1}, {Tool: "go-test", TreeID: "t1", Args: args, Attempt: 2}, {Tool: "vitest", TreeID: "t1", Args: args, Attempt: 9}}, "t1", 3},
		{"empty tree id", []indexEntry{{Tool: "go-test", Args: args, Attempt: 2}}, "", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := nextAttempt(writeIndex(t, tc.entries...), "go-test", tc.tree, args); got != tc.want {
				t.Errorf("attempt = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestIndexAttempt_MatchBeyondBudgetGivesOne(t *testing.T) {
	t.Parallel()
	args := []string{"./..."}
	filler := indexEntry{Tool: "go-test", TreeID: "other", Args: args, Log: strings.Repeat("f", 500)}
	path := writeIndex(t, indexEntry{Tool: "go-test", TreeID: "t1", Args: args, Attempt: 3}, filler, filler, filler, filler)
	if got := nextAttemptWithin(path, "go-test", "t1", args, 1000); got != 1 {
		t.Errorf("attempt = %d, want 1 (match is outside the budget)", got)
	}
	if got := nextAttemptWithin(path, "go-test", "t1", args, 1<<20); got != 4 {
		t.Errorf("attempt = %d, want 4 with a wide budget", got)
	}
}

func TestSignature_Cases(t *testing.T) {
	t.Parallel()
	fail := func(text string) Outcome { return Outcome{Failures: []Unit{{Text: text}}} }
	if got := signatureOf(0, fail("boom"), nil); got != "" {
		t.Errorf("pass signature = %q, want empty", got)
	}
	if got := signatureOf(1, fail("\n\n  first line\nsecond"), nil); got != "first line" {
		t.Errorf("signature = %q, want the first non-empty line", got)
	}
	long := strings.Repeat("é", 300)
	got := signatureOf(1, fail(long), nil)
	if len([]rune(got)) != 200 || !strings.HasPrefix(long, got) {
		t.Errorf("signature has %d runes, want 200 cut on a rune boundary", len([]rune(got)))
	}
	if got := signatureOf(1, Outcome{Unhandled: "1 unhandled error\nTypeError: x is undefined\n  at f"}, nil); got != "TypeError: x is undefined" {
		t.Errorf("unhandled signature = %q", got)
	}
	if got := signatureOf(2, Outcome{}, []byte("something broke\nmore\n")); !strings.Contains(got, "something broke") {
		t.Errorf("raw-head signature = %q", got)
	}
}
