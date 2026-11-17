package herdr

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRun_Success_ReturnsOutput(t *testing.T) {
	cli, argv := stubHerdr(t, map[string]reply{"workspace list": {out: "ok"}})
	out, err := cli.run("workspace", "list")
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if string(out) != "ok" {
		t.Fatalf("run() = %q, want %q", out, "ok")
	}
	if got := calls(t, argv); len(got) != 1 || got[0] != "workspace list" {
		t.Fatalf("argv = %v, want [workspace list]", got)
	}
}

func TestRun_Failure_WrapsCommandAndOutput(t *testing.T) {
	cli, _ := stubHerdr(t, map[string]reply{"workspace list": {out: "boom", exit: 1}})
	_, err := cli.run("workspace", "list")
	if err == nil {
		t.Fatal("run() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "$ herdr workspace list") {
		t.Errorf("error missing command line: %v", err)
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Errorf("error missing output: %v", err)
	}
	if !strings.Contains(err.Error(), "exit status 1") {
		t.Errorf("error missing underlying error: %v", err)
	}
}

func TestRunJSON_ParsesResultField(t *testing.T) {
	cli, _ := stubHerdr(t, map[string]reply{"x y": {out: `{"id":"cli:x","result":{"count":3}}`}})
	var result struct {
		Count int `json:"count"`
	}
	if err := cli.runJSON([]string{"x", "y"}, &result); err != nil {
		t.Fatalf("runJSON() error = %v", err)
	}
	if result.Count != 3 {
		t.Fatalf("result.Count = %d, want 3", result.Count)
	}
}

func TestRunJSON_PropagatesCommandError(t *testing.T) {
	cli, _ := stubHerdr(t, map[string]reply{"x y": {out: `{"error":{"code":"nope"}}`, exit: 1}})
	var result struct{}
	if err := cli.runJSON([]string{"x", "y"}, &result); err == nil {
		t.Fatal("runJSON() error = nil, want error")
	}
}

func TestRunJSON_MalformedOutput_ReturnsError(t *testing.T) {
	cli, _ := stubHerdr(t, map[string]reply{"x y": {out: "not json"}})
	var result struct{}
	if err := cli.runJSON([]string{"x", "y"}, &result); err == nil {
		t.Fatal("runJSON() error = nil, want error")
	}
}

func TestBinary_FallsBackToHerdrBinPath(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "herdr")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", t.TempDir()) // no herdr here, as under launchd
	t.Setenv("HERDR_BIN_PATH", bin)

	if got := (CLI{}).binary(); got != bin {
		t.Errorf("binary() = %q, want %q", got, bin)
	}
}

func TestBinary_FallsBackToInstallDirs(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "herdr")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", t.TempDir())
	t.Setenv("HERDR_BIN_PATH", "")
	old := fallbackDirs
	fallbackDirs = []string{dir}
	t.Cleanup(func() { fallbackDirs = old })

	if got := (CLI{}).binary(); got != bin {
		t.Errorf("binary() = %q, want %q", got, bin)
	}
}
