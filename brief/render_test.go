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

func TestRenderUnits_CollapsesIdenticalUnitsWithACount(t *testing.T) {
	u := Unit{Location: "pkg", Name: "TestA", Text: "a_test.go:7: boom"}
	var w bytes.Buffer
	renderUnits(&w, []Unit{u, u, u})
	want := "pkg\n  TestA (x3)\n    a_test.go:7: boom\n"
	if w.String() != want {
		t.Errorf("renderUnits = %q, want %q", w.String(), want)
	}
}

func TestRenderUnits_NamesOnlyUnitsGetFileLine(t *testing.T) {
	var units []Unit
	for i := range maxFullUnits + 1 {
		units = append(units, Unit{Location: "pkg", Name: "Test" + string(rune('A'+i)), Text: "    z_test.go:42: boom"})
	}
	var w bytes.Buffer
	renderUnits(&w, units)
	if want := "  TestF z_test.go:42\n1 more failures"; !strings.Contains(w.String(), want) {
		t.Errorf("names-only unit lacks file:line, want %q in:\n%s", want, w.String())
	}
}

func TestGoTestParse_CountRunsKeepOneRunsText(t *testing.T) {
	var out strings.Builder
	for range 2 {
		out.WriteString(`{"Action":"run","Package":"p","Test":"TestA"}` + "\n")
		out.WriteString(`{"Action":"output","Package":"p","Test":"TestA","Output":"a_test.go:1: boom\n"}` + "\n")
		out.WriteString(`{"Action":"fail","Package":"p","Test":"TestA"}` + "\n")
	}
	got := GoTest{}.Parse([]byte(out.String()), 1)
	var w bytes.Buffer
	renderUnits(&w, got.Failures)
	if want := "p\n  TestA (x2)\n    a_test.go:1: boom\n"; w.String() != want {
		t.Errorf("renderUnits = %q, want %q", w.String(), want)
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
