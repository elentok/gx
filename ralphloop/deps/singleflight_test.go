package deps

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// slowStub is an npm that logs "start" then "end" around a sleep, so overlapping
// installs show up as interleaved log lines.
func slowStub(t *testing.T, logPath string) string {
	t.Helper()
	binDir := t.TempDir()
	script := "#!/bin/sh\necho start >> " + logPath + "\nsleep 0.3\necho end >> " + logPath + "\n"
	if err := os.WriteFile(filepath.Join(binDir, "npm"), []byte(script), 0o755); err != nil {
		t.Fatalf("WriteFile stub: %v", err)
	}
	return binDir
}

func npmRepo(t *testing.T, packageJSON string) string {
	t.Helper()
	return installRepo(t, map[string]string{"package.json": packageJSON, "package-lock.json": "{}"}, nil)
}

func runConcurrently(t *testing.T, repos []string, binDir string) {
	t.Helper()
	var wg sync.WaitGroup
	for _, repo := range repos {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := installDependenciesWith(repo, binDir, ""); err != nil {
				t.Errorf("install in %s: %v", repo, err)
			}
		}()
	}
	wg.Wait()
}

func TestInstall_SameKey_RunsOneAfterTheOther(t *testing.T) {
	t.Parallel()
	logPath := filepath.Join(t.TempDir(), "log")
	binDir := slowStub(t, logPath)
	json := `{"name":"same-key-` + t.Name() + `"}`

	runConcurrently(t, []string{npmRepo(t, json), npmRepo(t, json)}, binDir)

	if got, want := strings.Join(readLog(t, logPath), ","), "start,end,start,end"; got != want {
		t.Errorf("log = %s, want %s", got, want)
	}
}

func TestInstall_DifferentKey_DoesNotWait(t *testing.T) {
	t.Parallel()
	logPath := filepath.Join(t.TempDir(), "log")
	binDir := slowStub(t, logPath)

	runConcurrently(t, []string{npmRepo(t, `{"name":"a-diff"}`), npmRepo(t, `{"name":"b-diff"}`)}, binDir)

	if got := strings.Join(readLog(t, logPath), ","); !strings.HasPrefix(got, "start,start") {
		t.Errorf("log = %s, want both installs to start before either ends", got)
	}
}

func TestInstall_CancelledWaiter_ReturnsWithoutInstalling(t *testing.T) {
	t.Parallel()
	logPath := filepath.Join(t.TempDir(), "log")
	binDir := slowStub(t, logPath)
	json := `{"name":"cancel-` + t.Name() + `"}`
	first, second := npmRepo(t, json), npmRepo(t, json)

	// Hold the key as a running install would.
	release, err := installFlights.acquire(context.Background(), installKey(context.Background(), first, first, "", binDir))
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	defer release()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err = installDependenciesWithin(ctx, second, binDir, "", time.Minute)
	if err == nil {
		t.Fatal("install error = nil, want the cancelled wait")
	}
	if got := readLog(t, logPath); got != nil {
		t.Errorf("stub invoked: %v", got)
	}
}

func TestFlights_FailedFirst_HandsOverToWaiter(t *testing.T) {
	t.Parallel()
	f := &flights{held: map[string]*flight{}}
	releaseFirst, err := f.acquire(context.Background(), "k")
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	got := make(chan error, 1)
	go func() {
		release, err := f.acquire(context.Background(), "k")
		if err == nil {
			release()
		}
		got <- err
	}()
	select {
	case <-got:
		t.Fatal("waiter acquired while the first still held the key")
	case <-time.After(50 * time.Millisecond):
	}
	releaseFirst() // the first install failed; release is the same either way
	select {
	case err := <-got:
		if err != nil {
			t.Errorf("waiter error = %v, want it to take over", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("waiter never took over")
	}
	if len(f.held) != 0 {
		t.Errorf("held = %d keys, want none left", len(f.held))
	}
}
