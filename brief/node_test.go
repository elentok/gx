package brief

import "testing"

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
