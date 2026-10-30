package skills

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestInstallationLockSerializesSeparateFileDescriptors(t *testing.T) {
	manifest := filepath.Join(t.TempDir(), "manifest.json")
	unlock, err := lockInstallation(manifest)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if unlock != nil {
			unlock()
		}
	}()
	second, err := os.OpenFile(manifest+".lock", os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	err = unix.Flock(int(second.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	if !errors.Is(err, unix.EWOULDBLOCK) {
		t.Fatalf("second writer acquired active installation lock: %v", err)
	}
	unlock()
	unlock = nil
	if err := unix.Flock(int(second.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		t.Fatal("released lock unavailable", err)
	}
	_ = unix.Flock(int(second.Fd()), unix.LOCK_UN)
}

func TestDiscoveryRootDanglingSymlinkIsPreserved(t *testing.T) {
	destination := filepath.Join(t.TempDir(), "new", "physical-skills")
	alias := filepath.Join(t.TempDir(), "skills")
	mustLink(t, destination, alias)
	source := writeSourceFile(t, t.TempDir(), "gx-example/SKILL.md", "source")
	manifest := filepath.Join(t.TempDir(), "manifest.json")
	mustInstall(t, manifest, InstallRequest{AgentRoots: []string{alias, destination}, Mode: ModeSymlink, Files: []SourceFile{source}})
	if target, err := os.Readlink(alias); err != nil || target != destination {
		t.Fatal("discovery root link replaced", target, err)
	}
	if _, err := Uninstall(manifest, ForcePolicy{}); err != nil {
		t.Fatal(err)
	}
	assertSource(t, source)
}
