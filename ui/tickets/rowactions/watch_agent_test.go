package rowactions

import (
	"fmt"
	"testing"
)

func TestWatchModal_KeepsNewestLines(t *testing.T) {
	texts := func(prefix string, n int) []string {
		out := make([]string, n)
		for i := range out {
			out[i] = fmt.Sprintf("%s %d", prefix, i)
		}
		return out
	}
	w := WatchModal{}.Open(WatchLoadedMsg{lines: texts("old", maxWatchLines)}, 120, 40)
	w = w.appendLines(texts("new", 500))
	if len(w.lines) != maxWatchLines || w.lines[len(w.lines)-1] != "new 499" || w.lines[0] != "old 500" {
		t.Errorf("kept %d lines, first %q, last %q", len(w.lines), w.lines[0], w.lines[len(w.lines)-1])
	}
}
