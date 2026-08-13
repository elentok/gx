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

func TestUserStateDirIgnoresXDGStateHome(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", filepath.Join(t.TempDir(), "xdg-state"))
	home := t.TempDir()
	t.Setenv("HOME", home)

	dir, err := UserStateDir()
	if err != nil {
		t.Fatalf("UserStateDir: %v", err)
	}
	want := filepath.Join(home, ".local", "state")
	if dir != want {
		t.Errorf("UserStateDir() = %q, want %q", dir, want)
	}
}

func TestMigrateStateFilesMovesEachFileWhenOldExistsAndNewDoesNot(t *testing.T) {
	root := t.TempDir()
	oldBase := filepath.Join(root, "old")
	newBase := filepath.Join(root, "new")
	oldGx := filepath.Join(oldBase, "gx")
	if err := os.MkdirAll(oldGx, 0755); err != nil {
		t.Fatalf("mkdir old: %v", err)
	}
	if err := os.WriteFile(filepath.Join(oldGx, "queue-state.json"), []byte("queue"), 0644); err != nil {
		t.Fatalf("write queue-state.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(oldGx, "notifications-state.json"), []byte("notif"), 0644); err != nil {
		t.Fatalf("write notifications-state.json: %v", err)
	}
	// config.json must stay behind - only the two state files migrate.
	if err := os.WriteFile(filepath.Join(oldGx, "config.json"), []byte("config"), 0644); err != nil {
		t.Fatalf("write config.json: %v", err)
	}

	prevConfig := userConfigDirFn
	userConfigDirFn = func() (string, error) { return oldBase, nil }
	t.Cleanup(func() { userConfigDirFn = prevConfig })
	prevState := userStateDirFn
	userStateDirFn = func() (string, error) { return newBase, nil }
	t.Cleanup(func() { userStateDirFn = prevState })

	if err := MigrateStateFiles(); err != nil {
		t.Fatalf("MigrateStateFiles: %v", err)
	}

	for _, name := range []string{"queue-state.json", "notifications-state.json"} {
		if _, err := os.Stat(filepath.Join(oldGx, name)); !os.IsNotExist(err) {
			t.Errorf("%s still exists in old dir: %v", name, err)
		}
		if _, err := os.Stat(filepath.Join(newBase, "gx", name)); err != nil {
			t.Errorf("%s missing from new dir: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(oldGx, "config.json")); err != nil {
		t.Errorf("config.json should have been left behind: %v", err)
	}
}

func TestMigrateStateFilesNoopWhenNewAlreadyExists(t *testing.T) {
	root := t.TempDir()
	oldBase := filepath.Join(root, "old")
	newBase := filepath.Join(root, "new")
	oldGx := filepath.Join(oldBase, "gx")
	newGx := filepath.Join(newBase, "gx")
	if err := os.MkdirAll(oldGx, 0755); err != nil {
		t.Fatalf("mkdir old: %v", err)
	}
	if err := os.WriteFile(filepath.Join(oldGx, "queue-state.json"), []byte("old"), 0644); err != nil {
		t.Fatalf("write old queue-state.json: %v", err)
	}
	if err := os.MkdirAll(newGx, 0755); err != nil {
		t.Fatalf("mkdir new: %v", err)
	}
	if err := os.WriteFile(filepath.Join(newGx, "queue-state.json"), []byte("new"), 0644); err != nil {
		t.Fatalf("write new queue-state.json: %v", err)
	}

	prevConfig := userConfigDirFn
	userConfigDirFn = func() (string, error) { return oldBase, nil }
	t.Cleanup(func() { userConfigDirFn = prevConfig })
	prevState := userStateDirFn
	userStateDirFn = func() (string, error) { return newBase, nil }
	t.Cleanup(func() { userStateDirFn = prevState })

	if err := MigrateStateFiles(); err != nil {
		t.Fatalf("MigrateStateFiles: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(newGx, "queue-state.json"))
	if err != nil {
		t.Fatalf("read new file: %v", err)
	}
	if string(data) != "new" {
		t.Errorf("new file content = %q, want unchanged %q", data, "new")
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
