package skills

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func mustInstall(t *testing.T, path string, req InstallRequest) []Target {
	t.Helper()
	targets, err := Install(path, req)
	if err != nil {
		t.Fatal(err)
	}
	return targets
}
func mustLink(t *testing.T, target, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
}
func mustSave(t *testing.T, path string, m Manifest) {
	t.Helper()
	if err := Save(path, m); err != nil {
		t.Fatal(err)
	}
}
func assertSource(t *testing.T, f SourceFile) {
	t.Helper()
	info, err := os.Lstat(f.AbsPath)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Mode().IsRegular() {
		t.Fatalf("source is not a regular file: %s", f.AbsPath)
	}
	if readFile(t, f.AbsPath) != "source" {
		t.Fatal("source content changed")
	}
}

func TestDirectoryDevLifecycleAndAliasedRoots(t *testing.T) {
	src := t.TempDir()
	root := t.TempDir()
	alias := filepath.Join(t.TempDir(), "alias")
	mustLink(t, root, alias)
	f := writeSourceFile(t, src, "gx-example/SKILL.md", "source")
	writeSourceFile(t, src, "gx-example/support.txt", "extra dev content") // not in the embedded list
	shared := writeSourceFile(t, src, "gx.md", "shared")
	nested := writeSourceFile(t, src, "gx-example/agents/openai.yaml", "nested")
	assertTreePreserved(t, src)
	manifest := filepath.Join(t.TempDir(), "manifest.json")
	req := InstallRequest{AgentRoots: []string{root, alias}, Files: []SourceFile{f, shared, nested}, Mode: ModeSymlink}
	targets := mustInstall(t, manifest, req)
	if len(targets) != 2 {
		t.Fatalf("duplicate writes for root aliases: %+v", targets)
	}
	link := filepath.Join(root, "gx-example")
	if got, err := os.Readlink(link); err != nil || got != filepath.Dir(f.AbsPath) {
		t.Fatalf("directory link %q: %v", got, err)
	}
	if readFile(t, filepath.Join(link, "agents/openai.yaml")) != "nested" {
		t.Fatal("nested support file missing")
	}
	if readFile(t, filepath.Join(link, "support.txt")) != "extra dev content" {
		t.Fatal("support not exposed")
	}
	info, _ := os.Lstat(link)
	for _, target := range mustInstall(t, manifest, req) {
		if target.Status != StatusSkipped {
			t.Fatalf("repeat: %+v", target)
		}
	}
	after, _ := os.Lstat(link)
	if !os.SameFile(info, after) {
		t.Fatal("repeat replaced directory link")
	}
	assertSource(t, f)
	if _, err := Uninstall(manifest, ForcePolicy{}); err != nil {
		t.Fatal(err)
	}
	assertSource(t, f)
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Fatal("link not removed")
	}
	if _, err := os.Readlink(alias); err != nil {
		t.Fatal("user root alias removed", err)
	}
	if _, err := Uninstall(manifest, ForcePolicy{}); !errors.Is(err, ErrNotExist) {
		t.Fatal(err)
	}
}

func TestMigrateLegacyDirectoryAndRetiredPromptRoot(t *testing.T) {
	for _, mode := range []InstallMode{ModeSymlink, ModeManagedCopy} {
		t.Run(string(mode), func(t *testing.T) {
			src := t.TempDir()
			oldRoot := t.TempDir()
			newRoot := t.TempDir()
			manifest := filepath.Join(t.TempDir(), "manifest.json")
			f := writeSourceFile(t, src, "gx-example/SKILL.md", "source")
			roots := []string{oldRoot, newRoot}
			m := Manifest{SchemaVersion: 1, Mode: mode, AgentRoots: roots, Files: []ManagedFile{{Path: f.RelPath}}}
			if mode == ModeSymlink {
				m.Files[0].LinkTarget = f.AbsPath
			} else {
				m.Files[0].Hash = hashBytes([]byte("source"))
			}
			for _, root := range roots {
				if mode == ModeSymlink {
					mustLink(t, f.AbsPath, filepath.Join(root, f.RelPath))
				} else {
					writeSourceFile(t, root, f.RelPath, "source")
				}
			}
			writeSourceFile(t, oldRoot, "personal.md", "keep")
			mustSave(t, manifest, m)
			targets := mustInstall(t, manifest, InstallRequest{Mode: mode, AgentRoots: []string{newRoot}, Files: []SourceFile{f}})
			if _, err := os.Lstat(filepath.Join(oldRoot, f.RelPath)); !os.IsNotExist(err) {
				t.Fatal("old prompt left behind", err)
			}
			if readFile(t, filepath.Join(oldRoot, "personal.md")) != "keep" {
				t.Fatal("unrelated prompt removed")
			}
			if targets[0].BackupDir == "" {
				t.Fatal("no retained backup")
			}
			m, err := Load(manifest)
			if err != nil {
				t.Fatal(err)
			}
			if m.SchemaVersion != 2 {
				t.Fatal(m)
			}
			if mode == ModeSymlink && m.Files[0].Kind != KindDirectoryLink {
				t.Fatal(m.Files)
			}
			assertSource(t, f)
		})
	}
}

func TestAdoptCorrectDirectoryLinkFromV1WithoutTouchingSource(t *testing.T) {
	src := t.TempDir()
	root := t.TempDir()
	f := writeSourceFile(t, src, "gx-example/SKILL.md", "source")
	manifest := filepath.Join(t.TempDir(), "manifest.json")
	mustLink(t, filepath.Dir(f.AbsPath), filepath.Join(root, "gx-example"))
	mustSave(t, manifest, Manifest{SchemaVersion: 1, Mode: ModeSymlink, AgentRoots: []string{root}, Files: []ManagedFile{{Path: f.RelPath, LinkTarget: f.AbsPath}}})
	targets := mustInstall(t, manifest, InstallRequest{Mode: ModeSymlink, AgentRoots: []string{root}, Files: []SourceFile{f}})
	if targets[0].Status != StatusSkipped {
		t.Fatal(targets)
	}
	assertSource(t, f)
	if _, err := Uninstall(manifest, ForceAll()); err != nil {
		t.Fatal(err)
	}
	assertSource(t, f)
}

func TestLegacyMigrationConflictsAreAtomic(t *testing.T) {
	for _, conflict := range []string{"modified", "unrelated", "wrong-link", "empty-directory", "replaced-directory"} {
		t.Run(conflict, func(t *testing.T) {
			src := t.TempDir()
			root := t.TempDir()
			manifest := filepath.Join(t.TempDir(), "manifest.json")
			f := writeSourceFile(t, src, "gx-example/SKILL.md", "source")
			mustLink(t, f.AbsPath, filepath.Join(root, f.RelPath))
			m := Manifest{SchemaVersion: 1, Mode: ModeSymlink, AgentRoots: []string{root}, Files: []ManagedFile{{Path: f.RelPath, LinkTarget: f.AbsPath}}}
			switch conflict {
			case "modified":
				os.Remove(filepath.Join(root, f.RelPath))
				writeSourceFile(t, root, f.RelPath, "edit")
			case "unrelated":
				writeSourceFile(t, root, "gx-example/mine.txt", "mine")
			case "wrong-link":
				os.Remove(filepath.Join(root, f.RelPath))
				mustLink(t, "/missing", filepath.Join(root, f.RelPath))
			case "replaced-directory":
				os.RemoveAll(filepath.Join(root, "gx-example"))
				writeSourceFile(t, root, "gx-example", "personal file")
			case "empty-directory":
				os.Mkdir(filepath.Join(root, "gx-example/mine"), 0755)
			}
			mustSave(t, manifest, m)
			before := readFile(t, manifest)
			_, err := Install(manifest, InstallRequest{Mode: ModeSymlink, AgentRoots: []string{root}, Files: []SourceFile{f}})
			var ce *ConflictError
			if !errors.As(err, &ce) {
				t.Fatalf("wanted conflict: %v", err)
			}
			if readFile(t, manifest) != before {
				t.Fatal("manifest changed")
			}
			assertSource(t, f)
		})
	}
}

func TestModeSwitchAndRetargetRequireForceAndBackUpLinks(t *testing.T) {
	src := t.TempDir()
	root := t.TempDir()
	manifest := filepath.Join(t.TempDir(), "manifest.json")
	f := writeSourceFile(t, src, "gx-example/SKILL.md", "source")
	req := InstallRequest{Mode: ModeSymlink, AgentRoots: []string{root}, Files: []SourceFile{f}}
	mustInstall(t, manifest, req)
	req.Mode = ModeManagedCopy
	if _, err := Install(manifest, req); err == nil {
		t.Fatal("mode switch succeeded without force")
	}
	req.Force = NewForcePolicy("gx-example")
	targets := mustInstall(t, manifest, req)
	backup := targets[0].BackupDir
	saved, err := os.Readlink(filepath.Join(backup, "0000"))
	if err != nil || saved != filepath.Dir(f.AbsPath) {
		t.Fatalf("backup followed directory link: %q %v", saved, err)
	}
	assertSource(t, f)
	writeSourceFile(t, root, "gx-example/personal", "keep")
	if _, err := Uninstall(manifest, ForcePolicy{}); err != nil {
		t.Fatal(err)
	}
	if readFile(t, filepath.Join(root, "gx-example/personal")) != "keep" {
		t.Fatal("unrelated content lost")
	}
	assertSource(t, f)
}

func TestStaleEntriesAndPhysicalRootAliases(t *testing.T) {
	src := t.TempDir()
	root := t.TempDir()
	alias := filepath.Join(t.TempDir(), "alias")
	mustLink(t, root, alias)
	a := writeSourceFile(t, src, "a.md", "a")
	b := writeSourceFile(t, src, "b.md", "b")
	manifest := filepath.Join(t.TempDir(), "manifest.json")
	mustInstall(t, manifest, InstallRequest{AgentRoots: []string{root}, Files: []SourceFile{a, b}})
	mustInstall(t, manifest, InstallRequest{AgentRoots: []string{alias}, Files: []SourceFile{a}})
	if readFile(t, filepath.Join(root, "a.md")) != "a" {
		t.Fatal("retained alias removed")
	}
	if _, err := os.Lstat(filepath.Join(root, "b.md")); !os.IsNotExist(err) {
		t.Fatal("stale entry remains")
	}
}

func TestTransactionRollsBackAndReportsRecoveryFailure(t *testing.T) {
	for _, restoreFails := range []bool{false, true} {
		t.Run(map[bool]string{false: "restored", true: "restore-fails"}[restoreFails], func(t *testing.T) {
			src := t.TempDir()
			root := t.TempDir()
			manifest := filepath.Join(t.TempDir(), "manifest.json")
			a := writeSourceFile(t, src, "a.md", "old a")
			b := writeSourceFile(t, src, "b.md", "old b")
			req := InstallRequest{AgentRoots: []string{root}, Files: []SourceFile{a, b}}
			mustInstall(t, manifest, req)
			before := readFile(t, manifest)
			writeSourceFile(t, src, "a.md", "new a")
			writeSourceFile(t, src, "b.md", "new b")
			p, err := buildInstallPlan(manifest, req)
			if err != nil {
				t.Fatal(err)
			}
			calls := 0
			io := transactionIO{apply: func(op operation) error {
				calls++
				if calls == 2 {
					return errors.New("injected write failure")
				}
				return applyOperation(op)
			}, restore: restorePath}
			if restoreFails {
				io.restore = func(s savedPath) error { return errors.New("injected restore failure") }
			}
			backup, err := executePlanWithIO(p, manifest, func() error { t.Fatal("published failed operation"); return nil }, io)
			if err == nil || backup == "" {
				t.Fatal("missing failure and recovery backup", err)
			}
			if readFile(t, manifest) != before {
				t.Fatal("manifest changed")
			}
			if restoreFails {
				if !strings.Contains(err.Error(), "rollback failed") || !strings.Contains(err.Error(), root) {
					t.Fatal(err)
				}
			} else {
				if readFile(t, filepath.Join(root, "a.md")) != "old a" || readFile(t, filepath.Join(root, "b.md")) != "old b" {
					t.Fatal("partial changes remain")
				}
			}
			if _, err := os.Stat(filepath.Join(backup, "paths.json")); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestManifestV1AndV2Kinds(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manifest.json")
	entries := []ManagedFile{{Path: "gx-a", Kind: KindDirectoryLink, LinkTarget: "/source/gx-a"}, {Path: "gx.md", Kind: KindFileLink, LinkTarget: "/source/gx.md"}, {Path: "gx-b/SKILL.md", Kind: KindCopiedFile, Hash: "hash"}}
	mustSave(t, path, Manifest{Mode: ModeSymlink, Files: entries})
	m, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(m.Files, entries) {
		t.Fatal(m.Files)
	}
	mustSave(t, path, Manifest{SchemaVersion: 1, Mode: ModeSymlink, Files: []ManagedFile{{Path: "gx-a/SKILL.md", LinkTarget: "/source/gx-a/SKILL.md"}}})
	m, err = Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if entryKind(m.Files[0]) != KindFileLink {
		t.Fatal("v1 inferred directory ownership")
	}
}

func TestForceNeverReplacesSourceThroughRootAlias(t *testing.T) {
	for _, uninstall := range []bool{false, true} {
		t.Run(map[bool]string{false: "install", true: "uninstall"}[uninstall], func(t *testing.T) {
			src := t.TempDir()
			f := writeSourceFile(t, src, "gx-example/SKILL.md", "source")
			root := filepath.Join(t.TempDir(), "root")
			mustLink(t, src, root)
			manifest := filepath.Join(t.TempDir(), "manifest.json")
			mustSave(t, manifest, Manifest{Mode: ModeSymlink, AgentRoots: []string{root}, Files: []ManagedFile{{Kind: KindDirectoryLink, Path: "gx-example", LinkTarget: filepath.Dir(f.AbsPath)}}})
			var err error
			if uninstall {
				_, err = Uninstall(manifest, ForceAll())
			} else {
				bundled := writeSourceFile(t, t.TempDir(), f.RelPath, "bundled")
				_, err = Install(manifest, InstallRequest{AgentRoots: []string{root}, Files: []SourceFile{bundled}, Force: ForceAll()})
			}
			if err == nil {
				t.Fatal("forced operation damaged source through root alias")
			}
			assertSource(t, f)
		})
	}
}

func TestMissingLegacyFilesAndModifiedRetiredEntries(t *testing.T) {
	for _, modified := range []bool{false, true} {
		t.Run(map[bool]string{false: "missing", true: "modified-retired"}[modified], func(t *testing.T) {
			src := t.TempDir()
			root := t.TempDir()
			retired := t.TempDir()
			manifest := filepath.Join(t.TempDir(), "manifest.json")
			f := writeSourceFile(t, src, "gx-example/SKILL.md", "source")
			old := Manifest{SchemaVersion: 1, Mode: ModeManagedCopy, AgentRoots: []string{root, retired}, Files: []ManagedFile{{Path: f.RelPath, Hash: hashBytes([]byte("source"))}, {Path: "gx-example/missing.txt", Hash: "old"}}}
			writeSourceFile(t, root, f.RelPath, "source")
			if modified {
				writeSourceFile(t, retired, f.RelPath, "local edit")
			}
			mustSave(t, manifest, old)
			before := readFile(t, manifest)
			_, err := Install(manifest, InstallRequest{AgentRoots: []string{root}, Files: []SourceFile{f}})
			if modified {
				var ce *ConflictError
				if !errors.As(err, &ce) {
					t.Fatal("modified retired file did not block", err)
				}
				if readFile(t, manifest) != before {
					t.Fatal("manifest changed")
				}
			} else if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestManifestPublishFailureRollsBackMissingParents(t *testing.T) {
	src := t.TempDir()
	root := filepath.Join(t.TempDir(), "new", "root")
	manifest := filepath.Join(t.TempDir(), "manifest.json")
	f := writeSourceFile(t, src, "gx-example/SKILL.md", "source")
	p, err := buildInstallPlan(manifest, InstallRequest{AgentRoots: []string{root}, Files: []SourceFile{f}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = executePlan(p, manifest, func() error { return errors.New("publish failure") })
	if err == nil {
		t.Fatal("missing publish failure")
	}
	if _, err := os.Stat(filepath.Dir(root)); !os.IsNotExist(err) {
		t.Fatal("created parents remain after rollback", err)
	}
	assertSource(t, f)
}

func TestRetargetDirectoryLinksBacksUpOldCheckoutLinks(t *testing.T) {
	root := t.TempDir()
	a := t.TempDir()
	b := t.TempDir()
	manifest := filepath.Join(t.TempDir(), "manifest.json")
	fa := writeSourceFile(t, a, "gx-example/SKILL.md", "source")
	fb := writeSourceFile(t, b, "gx-example/SKILL.md", "source")
	req := InstallRequest{AgentRoots: []string{root}, Mode: ModeSymlink, Files: []SourceFile{fa}}
	mustInstall(t, manifest, req)
	req.Files = []SourceFile{fb}
	if _, err := Install(manifest, req); err == nil {
		t.Fatal("retargeted without force")
	}
	req.Force = NewForcePolicy("gx-example")
	targets := mustInstall(t, manifest, req)
	if link, err := os.Readlink(filepath.Join(targets[0].BackupDir, "0000")); err != nil || link != filepath.Dir(fa.AbsPath) {
		t.Fatal("old checkout link not backed up", link, err)
	}
	if _, err := Uninstall(manifest, ForceAll()); err != nil {
		t.Fatal(err)
	}
	assertSource(t, fa)
	assertSource(t, fb)
}

func TestUninstallRollbackRestoresDirectoryLink(t *testing.T) {
	src := t.TempDir()
	root := t.TempDir()
	manifest := filepath.Join(t.TempDir(), "manifest.json")
	f := writeSourceFile(t, src, "gx-example/SKILL.md", "source")
	mustInstall(t, manifest, InstallRequest{AgentRoots: []string{root}, Mode: ModeSymlink, Files: []SourceFile{f}})
	before := readFile(t, manifest)
	p, err := buildUninstallPlan(manifest, ForceAll())
	if err != nil {
		t.Fatal(err)
	}
	_, err = executePlan(p, manifest, func() error { return errors.New("manifest removal failure") })
	if err == nil {
		t.Fatal("missing failure")
	}
	if link, err := os.Readlink(filepath.Join(root, "gx-example")); err != nil || link != filepath.Dir(f.AbsPath) {
		t.Fatal("uninstall rollback did not restore link", link, err)
	}
	if readFile(t, manifest) != before {
		t.Fatal("manifest changed")
	}
	assertSource(t, f)
}

// Check paths, types, permissions and bytes, including supporting files that
// are not listed in the installation request.
func assertTreePreserved(t *testing.T, root string) {
	t.Helper()
	snapshot := func() map[string]string {
		state := map[string]string{}
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(root, path)
			state[rel] = info.Mode().String()
			if info.Mode()&os.ModeSymlink != 0 {
				target, err := os.Readlink(path)
				if err != nil {
					return err
				}
				state[rel] += target
			} else if !info.IsDir() {
				state[rel] += readFile(t, path)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		return state
	}
	before := snapshot()
	t.Cleanup(func() {
		if after := snapshot(); !reflect.DeepEqual(before, after) {
			t.Errorf("source tree changed: before=%v after=%v", before, after)
		}
	})
}
