package brief

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestGoBuildPlan_PassesArgsThrough(t *testing.T) {
	if _, err := (GoBuild{}).Plan([]string{"PATH=" + t.TempDir()}, "", nil); err == nil {
		t.Fatal("Plan with no go on PATH = nil error, want one")
	}
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "go"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	env := []string{"PATH=" + bin}
	inv, err := GoBuild{}.Plan(env, "", []string{"./cmd"})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"build", "./cmd"}; !slices.Equal(inv.Argv, want) {
		t.Errorf("build argv = %v, want %v", inv.Argv, want)
	}
	inv, _ = GoVet{}.Plan(env, "", []string{"./cmd"})
	if want := []string{"vet", "./cmd"}; !slices.Equal(inv.Argv, want) {
		t.Errorf("vet argv = %v, want %v", inv.Argv, want)
	}
}

func TestGoBuildParse_CleanExitIsAPass(t *testing.T) {
	got := GoBuild{}.Parse(nil, 0)
	if got.Units != 1 || got.Summary != "no build errors" {
		t.Errorf("outcome = %+v, want 1 unit, summary %q", got, "no build errors")
	}
	got = GoVet{}.Parse(nil, 0)
	if got.Units != 1 || got.Summary != "no vet findings" {
		t.Errorf("outcome = %+v, want 1 unit, summary %q", got, "no vet findings")
	}
}

func TestGoBuildParse_GroupsDiagnosticsByFile(t *testing.T) {
	out := "# github.com/elentok/gx/cmd\n" +
		"cmd/a.go:3:5: undefined: x\n" +
		"cmd/a.go:9:1: undefined: y\n" +
		"cmd/b.go:2:2: missing return\n" +
		"\tmore detail\n" +
		"vet: some package-level failure\n"
	got := GoBuild{}.Parse([]byte(out), 1)
	if len(got.Failures) != 2 {
		t.Fatalf("failures = %+v, want 2 file units", got.Failures)
	}
	a := got.Failures[0]
	if a.Location != "cmd/a.go" || a.Name != "2 errors" || a.Text != "3:5: undefined: x\n9:1: undefined: y" {
		t.Errorf("first unit = %+v", a)
	}
	b := got.Failures[1]
	if b.Location != "cmd/b.go" || b.Name != "1 error" || b.Text != "2:2: missing return\n\tmore detail" {
		t.Errorf("second unit = %+v", b)
	}
}

func TestGoBuildParse_UnparsedFailureYieldsNoUnits(t *testing.T) {
	got := GoBuild{}.Parse([]byte("go: cannot find main module\n"), 1)
	if len(got.Failures) != 0 {
		t.Errorf("failures = %+v, want none so the raw head is shown", got.Failures)
	}
}
