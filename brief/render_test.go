package brief

import (
	"bytes"
	"fmt"
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
		units = append(units, Unit{Location: "pkg", Name: "Test" + string(rune('A'+i)), Text: fmt.Sprintf("    z_test.go:%d: boom", 42+i)})
	}
	var w bytes.Buffer
	renderUnits(&w, units)
	if want := "  TestF z_test.go:47\n1 more failures"; !strings.Contains(w.String(), want) {
		t.Errorf("names-only unit lacks file:line, want %q in:\n%s", want, w.String())
	}
}

func TestRenderUnits_ManyBuildFailuresEachShowFileLine(t *testing.T) {
	var units []Unit
	for i := range maxFullUnits + 3 {
		pkg := "example.com/p" + string(rune('a'+i))
		units = append(units, Unit{Location: pkg, Name: pkg, Kind: KindBuildFailed, Text: "./x.go:" + string(rune('1'+i)) + ":5: undefined: foo"})
	}
	var w bytes.Buffer
	renderUnits(&w, units)
	for i := range units {
		if want := "x.go:" + string(rune('1'+i)); !strings.Contains(w.String(), want) {
			t.Errorf("no %q for unit %d in:\n%s", want, i, w.String())
		}
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

func TestRenderUnits_GoFailureDropsJSONLogLinesAndShowsAssertionFirst(t *testing.T) {
	var text strings.Builder
	for i := range 10 {
		fmt.Fprintf(&text, "{\"level\":\"info\",\"msg\":\"request %d\"}\n", i)
	}
	text.WriteString("a_test.go:42: got 1, want 2\nctx 1\nctx 2\nctx 3\nctx 4")
	var w bytes.Buffer
	renderUnits(&w, []Unit{{Location: "pkg", Name: "TestA", Text: text.String()}})
	want := "pkg\n  TestA\n    a_test.go:42: got 1, want 2\n    ctx 1\n    ctx 2\n    ctx 3\n    [1 more lines cut, search the log]\n"
	if w.String() != want {
		t.Errorf("renderUnits = %q, want %q", w.String(), want)
	}
}

func TestRenderUnits_VitestFailureDropsActWarnings(t *testing.T) {
	text := "Warning: An update to Foo inside a test was not wrapped in act(...).\n\n" +
		"When testing, code that causes React state updates should be wrapped into act(...):\n\n" +
		"act(() => {\n  /* fire events that update state */\n});\n\n" +
		"This ensures that you're testing the behavior. Learn more at https://reactjs.org/link/wrap-tests-with-act\n" +
		"AssertionError: expected 1 to be 2\nExpected: 2\nReceived: 1"
	var w bytes.Buffer
	renderUnits(&w, []Unit{{Location: "a.test.ts", Name: "t", Text: text}})
	want := "a.test.ts\n  t\n    AssertionError: expected 1 to be 2\n    Expected: 2\n    Received: 1\n"
	if w.String() != want {
		t.Errorf("renderUnits = %q, want %q", w.String(), want)
	}
}

func TestRenderUnits_ManyFailuresWithOneCauseShowTheCauseOnce(t *testing.T) {
	var units []Unit
	for i := range 100 {
		units = append(units, Unit{Location: "a.test.ts", Name: fmt.Sprintf("test %d", i), Text: "ReferenceError: foo is not defined"})
	}
	var w bytes.Buffer
	renderUnits(&w, units)
	want := "100 tests failed with the same cause:\n  ReferenceError: foo is not defined\n"
	if w.String() != want {
		t.Errorf("renderUnits = %q, want %q", w.String(), want)
	}
}

func TestRerunLine_IsCappedInLength(t *testing.T) {
	var units []Unit
	for i := range 100 {
		units = append(units, Unit{Location: "a.test.ts", Name: fmt.Sprintf("test number %d", i), Text: "x"})
	}
	got := rerunLine("vitest", "", Vitest{}.RerunArgs(nil, units))
	if len(got) > maxRerunChars+200 || strings.Contains(got, "test number 50") {
		t.Errorf("rerun hint not capped (%d chars): %.200s", len(got), got)
	}
}

func TestRerunLine_NamesTheWebuiDir(t *testing.T) {
	got := rerunLine("playwright", "webui", []string{"epics.e2e.ts"})
	if want := "re-run only the failing tests: gx brief -C webui playwright epics.e2e.ts"; got != want {
		t.Errorf("rerunLine = %q, want %q", got, want)
	}
}
