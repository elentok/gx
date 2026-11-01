package claudeui

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Vim-mode screens for the agent-lifecycle probe. They are hand-made
// ("synthetic-" prefix and "# synthetic" header), not captures: a real claude
// could not be run where they were written.

var vimStates = []struct {
	name       string
	wantInsert bool
}{
	{"normal", false},
	{"insert-midturn", true},
	{"post-interrupt", true},
}

var sgr = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// vimFixtureRows loads a vim fixture as screen rows. The "# synthetic" header
// is dropped. Styled files write ESC as "␛" (it can't be typed into the
// fixture); it is turned back into the real byte here.
func vimFixtureRows(t *testing.T, state string, styled bool) []string {
	t.Helper()
	kind := "screen"
	if styled {
		kind = "styled"
	}
	b, err := os.ReadFile(filepath.Join("testdata", "synthetic-vim-"+state+"."+kind+".txt"))
	if err != nil {
		t.Fatal(err)
	}
	text := strings.ReplaceAll(string(b), "␛", "\x1b")
	rows := strings.Split(text, "\n")
	if len(rows) == 0 || !strings.HasPrefix(rows[0], "# synthetic") {
		t.Fatalf("%s %s: missing %q header", state, kind, "# synthetic")
	}
	return rows[2:] // header and the blank line after it
}

func inputRow(rows []string) (string, bool) {
	for _, r := range rows {
		if strings.HasPrefix(r, "❯") && !strings.Contains(r, "summarize") {
			return r, true
		}
	}
	return "", false
}

func hasInsertMarker(rows []string) bool {
	for _, r := range rows {
		if strings.Contains(r, "-- INSERT --") {
			return true
		}
	}
	return false
}

func TestVimFixtures(t *testing.T) {
	for _, s := range vimStates {
		for _, styled := range []bool{false, true} {
			name := s.name + "/plain"
			if styled {
				name = s.name + "/styled"
			}
			t.Run(name, func(t *testing.T) {
				rows := vimFixtureRows(t, s.name, styled)
				plain := make([]string, len(rows))
				for i, r := range rows {
					plain[i] = sgr.ReplaceAllString(r, "")
				}
				if styled && !strings.Contains(strings.Join(rows, "\n"), "\x1b[") {
					t.Error("styled fixture has no SGR sequences")
				}
				if _, ok := inputRow(plain); !ok {
					t.Error("no input row (❯) found")
				}
				if got := hasInsertMarker(plain); got != s.wantInsert {
					t.Errorf("-- INSERT -- row present = %v, want %v", got, s.wantInsert)
				}
			})
		}
	}
}
