package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeRecord(t *testing.T, base, dir, body string) {
	t.Helper()
	d := filepath.Join(base, dir)
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(d, pidFileName), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestStopFixture(t *testing.T) {
	base := t.TempDir()
	writeRecord(t, base, "gxf1", `{"pid":11,"scenario":"ready","worktree":"/w/mine"}`)
	writeRecord(t, base, "gxf2", `{"pid":22,"scenario":"mixed","worktree":"/w/other"}`)

	var signalled []int
	sig := func(pid int) error { signalled = append(signalled, pid); return nil }

	if err := stopFixture("ready", "/w/mine", []string{base}, sig); err != nil || len(signalled) != 1 || signalled[0] != 11 {
		t.Fatalf("stop by name: err=%v signalled=%v", err, signalled)
	}
	signalled = nil
	if err := stopFixture("11", "/w/mine", []string{base}, sig); err != nil || len(signalled) != 1 {
		t.Fatalf("stop by pid: err=%v signalled=%v", err, signalled)
	}
}

func TestStopFixture_RefusesAnotherWorktree(t *testing.T) {
	base := t.TempDir()
	writeRecord(t, base, "gxf2", `{"pid":22,"scenario":"mixed","worktree":"/w/other"}`)
	err := stopFixture("mixed", "/w/mine", []string{base}, func(int) error {
		t.Error("signalled a fixture of another worktree")
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "refusing to stop") || !strings.Contains(err.Error(), "/w/other") {
		t.Fatalf("err = %v, want a refusal naming the other worktree", err)
	}
}

func TestStopFixture_NoMatch(t *testing.T) {
	err := stopFixture("ready", "/w/mine", []string{t.TempDir()}, func(int) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "no running fixture") {
		t.Fatalf("err = %v", err)
	}
}
