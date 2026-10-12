package nativerunner_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/nativerunner"
)

// prepareForeignAgent writes an old agent directory whose runner has no
// layout, with files that would be pruned or cleaned up if it were headless.
func prepareForeignAgent(t *testing.T, root, label string) string {
	t.Helper()
	dir := filepath.Join(root, label)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(nativerunner.Meta{Runner: "martian", PID: agentPID + 1, SessionID: agentSID})
	files := map[string][]byte{nativerunner.MetaFile: data, nativerunner.OutFile: nil, nativerunner.ErrFile: nil, nativerunner.StdinFile: nil}
	old := time.Now().Add(-2 * nativerunner.DefaultLogRetention)
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, content, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, old, old); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func assertUntouched(t *testing.T, dir string) {
	t.Helper()
	for _, name := range []string{nativerunner.MetaFile, nativerunner.OutFile, nativerunner.ErrFile, nativerunner.StdinFile} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("%s was touched: %v", name, err)
		}
	}
}

func TestAgentDir_UnknownRunnerIsNeverTouched(t *testing.T) {
	root := t.TempDir()
	dir := prepareForeignAgent(t, root, "foreign")
	r := &nativerunner.Headless{Root: root, Procs: agentProcs()}

	pruned, err := r.Prune(nativerunner.DefaultLogRetention, time.Now(), func(string) bool { return false })
	if err != nil || len(pruned) != 0 {
		t.Errorf("Prune = %q, %v; want nothing pruned", pruned, err)
	}
	if err := r.Cleanup("foreign"); err != nil {
		t.Errorf("Cleanup = %v, want nil", err)
	}
	assertUntouched(t, dir)
}

func TestOpenDir_ReadsThroughTheNamedRunner(t *testing.T) {
	root := t.TempDir()
	dir := prepareAgent(t, root, "done", "", lines(evAssistant, evResult))
	d, err := nativerunner.OpenDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if d.Log() != filepath.Join(dir, nativerunner.OutFile) || d.Pruned() {
		t.Errorf("Log = %s, Pruned = %v; want live out.jsonl", d.Log(), d.Pruned())
	}
	if live, err := d.Alive(agentProcs()); err != nil || !live {
		t.Errorf("Alive = %v, %v; want live", live, err)
	}
	if err := os.Remove(d.Log()); err != nil {
		t.Fatal(err)
	}
	if !d.Pruned() {
		t.Error("Pruned = false after the log was removed")
	}
}

func TestOpenDir_UnknownRunner(t *testing.T) {
	dir := prepareForeignAgent(t, t.TempDir(), "foreign")
	if _, err := nativerunner.OpenDir(dir); !errors.Is(err, nativerunner.ErrUnknownRunner) {
		t.Errorf("OpenDir(foreign) err = %v, want ErrUnknownRunner", err)
	}
}

func TestAgentDir_InspectReportsUnknownRunner(t *testing.T) {
	root := t.TempDir()
	prepareForeignAgent(t, root, "foreign")
	r := &nativerunner.Headless{Root: root, Procs: agentProcs()}
	_, _, err := r.Inspect("foreign")
	if !errors.Is(err, nativerunner.ErrUnknownRunner) {
		t.Fatalf("Inspect(foreign) err = %v, want ErrUnknownRunner", err)
	}
	if want := `"martian"`; !strings.Contains(err.Error(), want) {
		t.Errorf("Inspect(foreign) err = %q, want it to name runner %s", err, want)
	}
}

func TestAgentDir_HeadlessCleanupRemovesOnlyTheFIFO(t *testing.T) {
	root := t.TempDir()
	dir := prepareAgent(t, root, "done", "", lines(evAssistant, evResult))
	if err := os.WriteFile(filepath.Join(dir, nativerunner.StdinFile), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	r := &nativerunner.Headless{Root: root, Procs: agentProcs()}
	if err := r.Cleanup("done"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, nativerunner.StdinFile)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("stdin survived cleanup: %v", err)
	}
	for _, kept := range []string{nativerunner.MetaFile, nativerunner.OutFile} {
		if _, err := os.Stat(filepath.Join(dir, kept)); err != nil {
			t.Errorf("cleanup removed %s: %v", kept, err)
		}
	}
}
