package skills

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInstallNeverWritesThroughDirectoryLinkedToSource(t *testing.T) {
	for _, mode := range []InstallMode{ModeSymlink, ModeManagedCopy} {
		t.Run(string(mode), func(t *testing.T) {
			source := t.TempDir()
			file := writeSourceFile(t, source, "gx-example/SKILL.md", "source")
			root := t.TempDir()
			mustLink(t, filepath.Join(source, "gx-example"), filepath.Join(root, "gx-example"))
			manifest := filepath.Join(t.TempDir(), "manifest.json")
			mustInstall(t, manifest, InstallRequest{AgentRoots: []string{root}, Files: []SourceFile{file}, Mode: mode, Force: ForceAll()})
			assertSource(t, file)
			if _, err := Uninstall(manifest, ForceAll()); err != nil {
				t.Fatal(err)
			}
			assertSource(t, file)
		})
	}
}

func TestV1UninstallRefusesLinkedParentEvenWithForce(t *testing.T) {
	source := t.TempDir()
	file := writeSourceFile(t, source, "gx-example/SKILL.md", "source")
	root := t.TempDir()
	mustLink(t, filepath.Dir(file.AbsPath), filepath.Join(root, "gx-example"))
	manifest := filepath.Join(t.TempDir(), "manifest.json")
	mustSave(t, manifest, Manifest{SchemaVersion: 1, Mode: ModeSymlink, AgentRoots: []string{root}, Files: []ManagedFile{{Path: file.RelPath, LinkTarget: file.AbsPath}}})
	if _, err := Uninstall(manifest, ForceAll()); err == nil {
		t.Fatal("v1 uninstall traversed linked parent")
	}
	assertSource(t, file)
	if _, err := os.Stat(manifest); err != nil {
		t.Fatal(err)
	}
}
