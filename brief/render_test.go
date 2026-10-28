package brief

import (
	"bytes"
	"strings"
	"testing"
)

func TestRawHead_StartsAtTheErrorAndDropsBuildNoise(t *testing.T) {
	out := "dist/assets/a.woff2  12 kB\n(!) Some chunks are larger than 500 kB\n- Use dynamic import()\n\n" +
		"\x1b[31mError: expect(locator).toBeVisible() failed\x1b[0m\nline 2\n"
	var w bytes.Buffer
	rawHead(&w, []byte(out))
	want := "Error: expect(locator).toBeVisible() failed\nline 2\n"
	if w.String() != want {
		t.Errorf("rawHead = %q, want %q", w.String(), want)
	}
}

func TestRawHead_FallsBackToTheTail(t *testing.T) {
	var lines []string
	for i := range 30 {
		lines = append(lines, "line "+string(rune('a'+i%26)))
	}
	lines[29] = "the last line"
	var w bytes.Buffer
	rawHead(&w, []byte(strings.Join(lines, "\n")+"\n"))
	if !strings.Contains(w.String(), "the last line") || strings.Contains(w.String(), "cut") {
		t.Errorf("tail fallback lost the last line or cut it:\n%s", w.String())
	}
}
