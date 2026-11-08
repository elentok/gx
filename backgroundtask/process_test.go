package backgroundtask_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/elentok/gx/backgroundtask"
	"github.com/elentok/gx/syshealth"
)

func startSleeper(t *testing.T, out string) *exec.Cmd {
	t.Helper()
	f, err := os.Create(out)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.Close() })
	cmd := exec.Command("sleep", "30")
	cmd.Stdout = f
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _, _ = cmd.Process.Wait() })
	return cmd
}

func finder() backgroundtask.ProcessFinder {
	return backgroundtask.ProcessFinder{Procs: syshealth.DefaultProcess()}
}

func TestProcessFinder_PicksProcessByOutputFile(t *testing.T) {
	dir := t.TempDir()
	out1, out2 := filepath.Join(dir, "a.out"), filepath.Join(dir, "b.out")
	startSleeper(t, out1)
	c2 := startSleeper(t, out2)
	if err := os.WriteFile(out2, []byte("1\n2\n3\n4\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	m, err := finder().Match(os.Getpid(), out2, "sleep 30")
	if err != nil {
		t.Fatal(err)
	}
	if !m.Alive || m.PID != c2.Process.Pid || m.Ambiguous {
		t.Fatalf("match = %+v, want pid %d", m, c2.Process.Pid)
	}
	if want := []string{"2", "3", "4"}; !slices.Equal(m.LastLines, want) {
		t.Fatalf("lines = %v, want %v", m.LastLines, want)
	}
}

func TestProcessFinder_CommandTextAloneIsAmbiguous(t *testing.T) {
	dir := t.TempDir()
	startSleeper(t, filepath.Join(dir, "a.out"))
	startSleeper(t, filepath.Join(dir, "b.out"))

	m, err := finder().Match(os.Getpid(), "", "sleep 30")
	if err != nil {
		t.Fatal(err)
	}
	if !m.Ambiguous || m.Alive || m.PID != 0 {
		t.Fatalf("match = %+v, want ambiguous and not alive", m)
	}
}

func TestProcessFinder_DeadProcessIsNotAlive(t *testing.T) {
	out := filepath.Join(t.TempDir(), "a.out")
	c := startSleeper(t, out)
	_ = c.Process.Kill()
	_, _ = c.Process.Wait()

	m, err := finder().Match(os.Getpid(), out, "")
	if err != nil {
		t.Fatal(err)
	}
	if m.Alive || m.Ambiguous {
		t.Fatalf("match = %+v, want dead", m)
	}
}

func TestProcessFinder_QuietOutputWithLiveProcessIsAlive(t *testing.T) {
	out := filepath.Join(t.TempDir(), "a.out")
	c := startSleeper(t, out)
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(out, old, old); err != nil {
		t.Fatal(err)
	}

	m, err := finder().Match(os.Getpid(), out, "")
	if err != nil {
		t.Fatal(err)
	}
	if !m.Alive || m.PID != c.Process.Pid {
		t.Fatalf("match = %+v, want alive pid %d", m, c.Process.Pid)
	}
	if !m.LastOutput.Equal(old.Truncate(time.Second)) && m.LastOutput.Sub(old).Abs() > time.Second {
		t.Fatalf("LastOutput = %v, want about %v", m.LastOutput, old)
	}
}
