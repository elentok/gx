package migratefail_test

import (
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"slices"
	"sync"
	"testing"

	"github.com/elentok/gx/migratefail"
)

func TestRecord_SurvivesAndReadsSorted(t *testing.T) {
	dir := t.TempDir()
	if err := migratefail.Record(dir, "b", "boom b"); err != nil {
		t.Fatal(err)
	}
	if err := migratefail.Record(dir, "a", "boom a"); err != nil {
		t.Fatal(err)
	}
	if err := migratefail.Record(dir, "b", "newer b"); err != nil {
		t.Fatal(err)
	}
	got, err := migratefail.Read(dir, []string{"a", "b"})
	if err != nil {
		t.Fatal(err)
	}
	want := []migratefail.Failure{{Project: "a", Error: "boom a"}, {Project: "b", Error: "newer b"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestClear_RemovesOneProject(t *testing.T) {
	dir := t.TempDir()
	_ = migratefail.Record(dir, "a", "x")
	_ = migratefail.Record(dir, "b", "y")
	if err := migratefail.Clear(dir, "a"); err != nil {
		t.Fatal(err)
	}
	got, _ := migratefail.Read(dir, []string{"a", "b"})
	if len(got) != 1 || got[0].Project != "b" {
		t.Fatalf("got %v", got)
	}
}

func TestClear_NoEntryWritesNothing(t *testing.T) {
	dir := t.TempDir()
	if err := migratefail.Clear(dir, "a"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(migratefail.Path(dir)); !os.IsNotExist(err) {
		t.Fatalf("file created: %v", err)
	}
}

const helperEnv = "MIGRATEFAIL_HELPER_MODE"

// TestHelperProcess is the child side of TestRecordAndClear_TwoProcesses; it
// does nothing in a normal run.
func TestHelperProcess(t *testing.T) {
	mode := os.Getenv(helperEnv)
	if mode == "" {
		t.Skip("child-process helper")
	}
	dir := os.Getenv("MIGRATEFAIL_HELPER_DIR")
	for i := range helperRounds {
		var err error
		if mode == "record" {
			err = migratefail.Record(dir, fmt.Sprintf("a%d", i), "x")
		} else {
			err = migratefail.Clear(dir, fmt.Sprintf("b%d", i))
		}
		if err != nil {
			t.Fatal(err)
		}
	}
}

const helperRounds = 60

func TestRecordAndClear_TwoProcesses(t *testing.T) {
	dir := t.TempDir()
	var configured, want []string
	for i := range helperRounds {
		if err := migratefail.Record(dir, fmt.Sprintf("b%d", i), "y"); err != nil {
			t.Fatal(err)
		}
		configured = append(configured, fmt.Sprintf("a%d", i), fmt.Sprintf("b%d", i))
		want = append(want, fmt.Sprintf("a%d", i))
	}
	var wg sync.WaitGroup
	outs := make([][]byte, 2)
	errs := make([]error, 2)
	for i, mode := range []string{"record", "clear"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cmd := exec.Command(os.Args[0], "-test.run=^TestHelperProcess$")
			cmd.Env = append(os.Environ(), helperEnv+"="+mode, "MIGRATEFAIL_HELPER_DIR="+dir)
			outs[i], errs[i] = cmd.CombinedOutput()
		}()
	}
	wg.Wait()
	for i := range errs {
		if errs[i] != nil {
			t.Fatalf("child %d: %v\n%s", i, errs[i], outs[i])
		}
	}
	got, err := migratefail.Read(dir, configured)
	if err != nil {
		t.Fatal(err)
	}
	var gotNames []string
	for _, f := range got {
		gotNames = append(gotNames, f.Project)
	}
	slices.Sort(want)
	if !reflect.DeepEqual(gotNames, want) {
		t.Fatalf("lost update: got %d entries %v, want %d", len(gotNames), gotNames, len(want))
	}
}

func TestRead_DropsUnconfiguredProjects(t *testing.T) {
	dir := t.TempDir()
	_ = migratefail.Record(dir, "gone", "x")
	_ = migratefail.Record(dir, "kept", "y")
	got, err := migratefail.Read(dir, []string{"kept"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Project != "kept" {
		t.Fatalf("got %v", got)
	}
}
