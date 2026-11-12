package brief

import (
	"strings"
	"testing"
)

func TestTscParse_KeepsFilelessDiagnosticsAsGlobalUnit(t *testing.T) {
	out := "src/a.ts(1,14): error TS2322: Type 'string' is not assignable to type 'number'.\n" +
		"error TS5101: Option 'baseUrl' is deprecated.\n" +
		"  Specify compilerOption 'ignoreDeprecations'.\n"
	got := Tsc{}.Parse([]byte(out), 2)
	if len(got.Failures) != 2 {
		t.Fatalf("failures = %+v, want a file unit and a global unit", got.Failures)
	}
	g := got.Failures[1]
	if g.Location != "(global)" || g.Name != "1 error" {
		t.Errorf("global unit = %+v, want location (global), name 1 error", g)
	}
	want := "error TS5101: Option 'baseUrl' is deprecated.\n  Specify compilerOption 'ignoreDeprecations'."
	if g.Text != want {
		t.Errorf("global text = %q, want %q", g.Text, want)
	}
}

func TestVitestParse_ReportsUnhandledErrorsWhenTestsPass(t *testing.T) {
	out := `{"numTotalTestSuites":1,"numTotalTests":1,"testResults":[{"name":"a.test.ts","status":"passed","assertionResults":[{"fullName":"a","status":"passed"}]}]}
 ⎯⎯⎯⎯⎯⎯ Unhandled Errors ⎯⎯⎯⎯⎯⎯

Vitest caught 7 unhandled errors during the test run.
This might cause false positive tests.

 ⎯⎯⎯⎯⎯⎯ Unhandled Error ⎯⎯⎯⎯⎯⎯⎯
TypeError: shell?.closeSliding is not a function
 ❯ src/palette.ts:12:3
 ❯ src/other.ts:1:1
`
	got := Vitest{}.Parse([]byte(out), 1)
	want := "7 unhandled errors\nTypeError: shell?.closeSliding is not a function\n ❯ src/palette.ts:12:3"
	if got.Unhandled != want {
		t.Errorf("Unhandled = %q, want %q", got.Unhandled, want)
	}
	var b strings.Builder
	report(Vitest{}, Options{Out: &b}, got, []byte(out), indexEntry{Exit: 1, Log: "/log"})
	if s := b.String(); !strings.HasPrefix(s, "FAIL vitest: tests passed, 7 unhandled errors") || strings.Contains(s, "numTotalTests") {
		t.Errorf("report = %q", s)
	}
}
