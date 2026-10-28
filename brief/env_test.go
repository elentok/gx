package brief

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGoTestPlan_FindsGoOnTheEnvPathNotTheProcessPath(t *testing.T) {
	bin := t.TempDir()
	want := filepath.Join(bin, "go")
	if err := os.WriteFile(want, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	inv, err := GoTest{}.Plan([]string{"PATH=" + bin}, t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if inv.Bin != want {
		t.Errorf("Bin = %q, want %q", inv.Bin, want)
	}

	if _, err := (GoTest{}).Plan([]string{"PATH=" + t.TempDir()}, t.TempDir(), nil); err == nil {
		t.Error("Plan found go although the env PATH has none")
	}
}

func TestGoTestProcs(t *testing.T) {
	tests := []struct {
		name  string
		cores int
		env   []string
		want  int
	}{
		{"half the cores, two fifths of that", 10, nil, 2},
		{"all cores on a small machine", 4, nil, 1},
		{"never below one", 1, nil, 1},
		{"big machine", 64, nil, 12},
		{"budget from GX_TEST_CPUS", 10, []string{"GX_TEST_CPUS=20"}, 8},
		{"bad GX_TEST_CPUS is ignored", 10, []string{"GX_TEST_CPUS=many"}, 2},
		{"zero GX_TEST_CPUS is ignored", 10, []string{"GX_TEST_CPUS=0"}, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := goTestProcs(tt.cores, tt.env); got != tt.want {
				t.Errorf("goTestProcs(%d, %v) = %d, want %d", tt.cores, tt.env, got, tt.want)
			}
		})
	}
}

func TestGoTestPlan_AddsAGomaxprocsDefault(t *testing.T) {
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "go"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	inv, err := GoTest{}.Plan([]string{"PATH=" + bin}, t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(inv.Env) != 1 || !strings.HasPrefix(inv.Env[0], "GOMAXPROCS=") {
		t.Errorf("Env = %v, want one GOMAXPROCS entry", inv.Env)
	}
}

func TestWithDefaults_CallerValueWins(t *testing.T) {
	got := withDefaults([]string{"PATH=/bin", "GOMAXPROCS=7"}, []string{"GOMAXPROCS=2", "OTHER=1"})
	want := []string{"PATH=/bin", "GOMAXPROCS=7", "OTHER=1"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("withDefaults = %v, want %v", got, want)
	}
}

func TestExecute_ChildGetsOnlyTheGivenEnv(t *testing.T) {
	t.Setenv("BRIEF_LEAK", "leaked")
	out, exit, err := execute("/usr/bin/env", nil, t.TempDir(), []string{"BRIEF_PROBE=yes"}, io.Discard)
	if err != nil || exit != 0 {
		t.Fatalf("exit = %d, err = %v", exit, err)
	}
	if got := strings.TrimSpace(string(out)); got != "BRIEF_PROBE=yes" {
		t.Errorf("child env = %q, want only BRIEF_PROBE=yes", got)
	}
}
