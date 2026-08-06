package config

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestMigrateDirMovesContentsWhenOldExistsAndNewDoesNot(t *testing.T) {
	root := t.TempDir()
	oldPath := filepath.Join(root, "old", "gx")
	newPath := filepath.Join(root, "new", "gx")
	if err := os.MkdirAll(oldPath, 0755); err != nil {
		t.Fatalf("mkdir old: %v", err)
	}
	if err := os.WriteFile(filepath.Join(oldPath, "config.json"), []byte("old"), 0644); err != nil {
		t.Fatalf("write old file: %v", err)
	}

	if err := migrateDir(oldPath, newPath); err != nil {
		t.Fatalf("migrateDir: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(newPath, "config.json"))
	if err != nil {
		t.Fatalf("read migrated file: %v", err)
	}
	if string(data) != "old" {
		t.Errorf("migrated content = %q, want %q", data, "old")
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Errorf("old path still exists after migration: %v", err)
	}
}

func TestMigrateDirNoopWhenNewAlreadyExists(t *testing.T) {
	root := t.TempDir()
	oldPath := filepath.Join(root, "old", "gx")
	newPath := filepath.Join(root, "new", "gx")
	if err := os.MkdirAll(oldPath, 0755); err != nil {
		t.Fatalf("mkdir old: %v", err)
	}
	if err := os.WriteFile(filepath.Join(oldPath, "config.json"), []byte("old"), 0644); err != nil {
		t.Fatalf("write old file: %v", err)
	}
	if err := os.MkdirAll(newPath, 0755); err != nil {
		t.Fatalf("mkdir new: %v", err)
	}
	if err := os.WriteFile(filepath.Join(newPath, "config.json"), []byte("new"), 0644); err != nil {
		t.Fatalf("write new file: %v", err)
	}

	if err := migrateDir(oldPath, newPath); err != nil {
		t.Fatalf("migrateDir: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(newPath, "config.json"))
	if err != nil {
		t.Fatalf("read new file: %v", err)
	}
	if string(data) != "new" {
		t.Errorf("new path content = %q, want unchanged %q", data, "new")
	}
	if _, err := os.Stat(oldPath); err != nil {
		t.Errorf("old path should be left alone: %v", err)
	}
}

func TestMigrateDirNoopWhenNeitherExists(t *testing.T) {
	root := t.TempDir()
	oldPath := filepath.Join(root, "old", "gx")
	newPath := filepath.Join(root, "new", "gx")

	if err := migrateDir(oldPath, newPath); err != nil {
		t.Fatalf("migrateDir: %v", err)
	}
	if _, err := os.Stat(newPath); !os.IsNotExist(err) {
		t.Errorf("new path should not have been created: %v", err)
	}
}

func TestWarnOnMigrateFailureWritesWarningWhenRenameFails(t *testing.T) {
	root := t.TempDir()
	legacyBase := filepath.Join(root, "legacy")
	oldPath := filepath.Join(legacyBase, "gx")
	if err := os.MkdirAll(oldPath, 0755); err != nil {
		t.Fatalf("mkdir old: %v", err)
	}

	// A read-only new base makes os.Rename fail with a permission error when
	// it tries to create the "gx" entry inside it.
	newBase := filepath.Join(root, "new")
	if err := os.MkdirAll(newBase, 0555); err != nil {
		t.Fatalf("mkdir new base: %v", err)
	}
	t.Cleanup(func() { os.Chmod(newBase, 0755) })

	prevLegacy := legacyUserConfigDirFn
	legacyUserConfigDirFn = func() (string, error) { return legacyBase, nil }
	t.Cleanup(func() { legacyUserConfigDirFn = prevLegacy })

	prevNew := userConfigDirFn
	userConfigDirFn = func() (string, error) { return newBase, nil }
	t.Cleanup(func() { userConfigDirFn = prevNew })

	var stderr bytes.Buffer
	WarnOnMigrateFailure(&stderr)

	if stderr.Len() == 0 {
		t.Errorf("expected a warning to be written to stderr, got none")
	}
}

func TestFilePathIgnoresXDGConfigHome(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "xdg-config"))
	home := t.TempDir()
	t.Setenv("HOME", home)

	path, err := FilePath()
	if err != nil {
		t.Fatalf("FilePath: %v", err)
	}
	want := filepath.Join(home, ".config", "gx", "config.json")
	if path != want {
		t.Errorf("FilePath() = %q, want %q", path, want)
	}
}
