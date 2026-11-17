package brief

import (
	"os"
	"path/filepath"
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

func TestBiomeParse(t *testing.T) {
	t.Parallel()
	read := func(name string) []byte {
		b, err := os.ReadFile(filepath.Join("testdata", name))
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	cases := []struct {
		sample  string
		out     string // overrides sample
		units   int    // failure units, 0 means pass
		text    string
		wantRaw bool
	}{
		{sample: "biome-clean.txt"},
		{sample: "biome-info.txt", units: 1, text: "3:14 lint/style/useTemplate"},
		{sample: "biome-warning.txt", units: 1, text: "1:8 lint/suspicious/noExplicitAny"},
		{sample: "biome-error.txt", units: 1, text: "2:7 lint/correctness/noUnusedVariables"},
		{sample: "biome-hidden.txt", units: 1, text: "4 diagnostics were not printed"},
		{out: "The `json` reporter is experimental.\nsomething else\n", units: 1, text: "something else"},
	}
	for _, c := range cases {
		out := []byte(c.out)
		if c.sample != "" {
			out = read(c.sample)
		}
		got := Biome{}.Parse(out, 0)
		if len(got.Failures) != c.units {
			t.Errorf("%s%q: failures = %+v, want %d", c.sample, c.out, got.Failures, c.units)
			continue
		}
		if c.units == 0 && got.Units == 0 {
			t.Errorf("%s: a pass must count a unit", c.sample)
		}
		if c.units > 0 && !strings.Contains(got.Failures[0].Text, c.text) {
			t.Errorf("%s%q: text = %q, want %q", c.sample, c.out, got.Failures[0].Text, c.text)
		}
	}
	two := Biome{}.Parse(read("biome-info.txt"), 0)
	if two.Failures[0].Location != "src/a.ts" || two.Failures[0].Name != "2 findings" {
		t.Errorf("unit = %+v, want src/a.ts with 2 findings", two.Failures[0])
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
