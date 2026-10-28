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
