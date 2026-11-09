package limitsflows

import (
	"github.com/elentok/gx/testutil/gxfixture/fixturetest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server"
)

func (r *running) waitDiskStatus(t *testing.T, want string) {
	t.Helper()
	var out string
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		out = r.Gx(t, "server", "disk", "status")
		if strings.Contains(out, want) {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("disk status never showed %q:\n%s", want, out)
}

func TestDiskLow_TheFreeAndCleanFilesSteerTheDisk(t *testing.T) {
	r := start(t, "disk-low")
	defer r.Stop(t)

	r.SetDisk(t, "free", "15G")
	r.waitDiskStatus(t, "fixture disk: 15G free")

	r.SetDisk(t, "clean", "25G")
	out := r.Gx(t, "server", "disk", "clean", "fixture disk")
	if !strings.Contains(out, "25G free now") {
		t.Errorf("clean output = %q, want 25G free now", out)
	}

	r.SetDisk(t, "clean", "fail")
	cmd := exec.Command(fixturetest.GxBin, "server", "disk", "clean", "fixture disk")
	cmd.Env = r.Env
	failed, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(failed), "clean failed") {
		t.Errorf("failed clean: err=%v output=%q, want a failure", err, failed)
	}
	// The verb names the command; the script's own message goes to the server log.
	log, err := os.ReadFile(server.LogPath(r.StateDir))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(log), "fixture cleanup failed") {
		t.Errorf("server log lacks the script's message:\n%s", log)
	}
}

func TestDisk_EveryOtherScenarioStartsAt100GWithNoDisk(t *testing.T) {
	r := start(t, "ready")
	defer r.Stop(t)

	got, err := os.ReadFile(filepath.Join(r.DiskDir, "free"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(got)) != "100G" {
		t.Errorf("free = %q, want 100G", got)
	}
	// The strip and hold depend on the watcher having read the disk.
	time.Sleep(time.Second)
	if out := r.Gx(t, "server", "disk", "status"); strings.Contains(out, "[warned]") {
		t.Errorf("disk status warns:\n%s", out)
	}
}
