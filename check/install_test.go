package check

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/testutil"
)

func testInstallRunner() *runner {
	return &runner{installs: map[string]*installEntry{}}
}

// The install in dir a waits for dir b's install to finish, so it only passes
// when installs in different dirs run at the same time.
func TestEnsureInstalled_DifferentDirsInstallInParallel(t *testing.T) {
	root := t.TempDir()
	a, b := filepath.Join(root, "a"), filepath.Join(root, "b")
	for _, d := range []string{a, b} {
		if err := os.Mkdir(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	r := testInstallRunner()

	waitForB := installNeed{dir: a, cmd: `touch started; for i in $(seq 100); do [ -f ../b/done ] && exit 0; sleep 0.05; done; exit 1`}
	touchDone := installNeed{dir: b, cmd: `touch done`}

	var wg sync.WaitGroup
	var errA error
	wg.Go(func() { _, errA = r.ensureInstalled(waitForB) })
	testutil.Poll(5*time.Second, 10*time.Millisecond, func() bool {
		_, err := os.Stat(filepath.Join(a, "started"))
		return err == nil
	})
	_, errB := r.ensureInstalled(touchDone)
	wg.Wait()
	if errA != nil || errB != nil {
		t.Fatalf("install a: %v, install b: %v", errA, errB)
	}
}

func TestEnsureInstalled_SameDirInstallsOnce(t *testing.T) {
	dir := t.TempDir()
	r := testInstallRunner()
	n := installNeed{dir: dir, cmd: `echo ran >> count`}

	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() { _, _ = r.ensureInstalled(n) })
	}
	wg.Wait()
	got, err := os.ReadFile(filepath.Join(dir, "count"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "ran\n" {
		t.Errorf("install ran %q, want once", got)
	}
}
