package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
)

const diskGB = 1_000_000_000

func diskByID(disks []server.DiskStatus, id string) *server.DiskStatus {
	for i := range disks {
		if disks[i].ID == id {
			return &disks[i]
		}
	}
	return nil
}

// startDiskStatusServer starts a server with one low disk and one healthy disk,
// and waits until the watcher has read both.
func startDiskStatusServer(t *testing.T, ds config.DiskSpaceConfig) *servertest.Harness {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	store := t.TempDir()
	lowRepo, okRepo := t.TempDir(), t.TempDir()
	for name, repo := range map[string]string{"low": lowRepo, "healthy": okRepo} {
		if err := os.MkdirAll(filepath.Join(store, name), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := servertest.WriteProjectRepo(store, name, repo, ""); err != nil {
			t.Fatal(err)
		}
	}
	disks := &servertest.FakeDisks{}
	disks.Put(lowRepo, "d-low", "/vol/low", 5*diskGB)
	disks.Put(okRepo, "d-ok", "/vol/ok", 500*diskGB)
	h := servertest.StartWithStore(t, store, func(c *server.Config) {
		c.FreeSpace = disks.Read
		c.GoCacheDir = func() (string, error) { return okRepo, nil }
		c.DiskPollInterval = 10 * time.Millisecond
		c.DiskSpace = ds
	})
	var rep server.DiskReport
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		var err error
		if rep, err = h.Client.DiskStatus(context.Background()); err != nil {
			t.Fatal(err)
		}
		if diskByID(rep.Disks, "d-low") != nil && diskByID(rep.Disks, "d-ok") != nil {
			return h
		}
	}
	t.Fatalf("watcher never read both disks, last report = %+v", rep)
	return nil
}

func TestDiskStatus_PrintsThresholdsAndEveryDisk(t *testing.T) {
	h := startDiskStatusServer(t, config.DiskSpaceConfig{WarnBelow: 20 * diskGB, CleanBelow: 10 * diskGB})

	var out bytes.Buffer
	if err := runDiskStatus(context.Background(), h.Client, &out, false); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if want := "warn below 20G, clean below 10G"; lines[0] != want {
		t.Errorf("thresholds line = %q, want %q", lines[0], want)
	}
	line := func(name string) string {
		for _, l := range lines[1:] {
			if strings.HasPrefix(l, name+":") {
				return l
			}
		}
		t.Fatalf("no line for %s in %q", name, out.String())
		return ""
	}
	ok, low := line("/vol/ok"), line("/vol/low")
	for _, w := range []string{"500G free", "ago", "healthy"} {
		if !strings.Contains(ok, w) {
			t.Errorf("line = %q, want %q", ok, w)
		}
	}
	if strings.Contains(ok, "warned") {
		t.Errorf("line = %q, must not be marked warned", ok)
	}
	for _, w := range []string{"5G free", "ago", "projects: low", "[warned]"} {
		if !strings.Contains(low, w) {
			t.Errorf("line = %q, want %q", low, w)
		}
	}
}

func TestDiskStatus_OffThresholdPrintsOff(t *testing.T) {
	h := startDiskStatusServer(t, config.DiskSpaceConfig{})
	var out bytes.Buffer
	if err := runDiskStatus(context.Background(), h.Client, &out, false); err != nil {
		t.Fatal(err)
	}
	if first, _, _ := strings.Cut(out.String(), "\n"); first != "warn below off, clean below off" {
		t.Errorf("thresholds line = %q", first)
	}
}

func TestDiskClean_PrintsFreedAndFreeAfterAndRefusalFails(t *testing.T) {
	h := startDiskStatusServer(t, config.DiskSpaceConfig{WarnBelow: 20 * diskGB, CleanBelow: 10 * diskGB})

	var out bytes.Buffer
	if err := runDiskClean(context.Background(), h.Client, &out, "d-low", false); err != nil {
		t.Fatal(err)
	}
	if want := "freed 0, 5G free now, dispatch held\n"; out.String() != want {
		t.Errorf("output = %q, want %q", out.String(), want)
	}

	out.Reset()
	if err := runDiskClean(context.Background(), h.Client, &out, "nope", false); err == nil {
		t.Errorf("unknown disk returned no error, output = %q", out.String())
	}
}

func TestDiskStatus_JSONMatchesTheRoute(t *testing.T) {
	h := startDiskStatusServer(t, config.DiskSpaceConfig{WarnBelow: 20 * diskGB, CleanBelow: 10 * diskGB})

	var out bytes.Buffer
	if err := runDiskStatus(context.Background(), h.Client, &out, true); err != nil {
		t.Fatal(err)
	}
	var rep server.DiskReport
	if err := json.Unmarshal(out.Bytes(), &rep); err != nil {
		t.Fatal(err)
	}
	low, ok := diskByID(rep.Disks, "d-low"), diskByID(rep.Disks, "d-ok")
	if rep.WarnBelow != "20G" || rep.CleanBelow != "10G" || low == nil || ok == nil {
		t.Fatalf("--json = %s", out.String())
	}
	if !low.Warned || low.FreeBytes != 5*diskGB || low.ReadAt.IsZero() || ok.Warned || ok.FreeBytes != 500*diskGB {
		t.Errorf("disks = %+v", rep.Disks)
	}
}
