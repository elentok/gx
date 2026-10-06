package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDirs_DefaultsUnderHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("XDG_STATE_HOME", "")

	data, err := DataDir()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(home, ".local", "share", "gx"); data != want {
		t.Errorf("DataDir() = %q, want %q", data, want)
	}
	state, err := StateDir()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(home, ".local", "state", "gx"); state != want {
		t.Errorf("StateDir() = %q, want %q", state, want)
	}
}

func TestDirs_HonorXDGEnv(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "data"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(root, "state"))

	data, _ := DataDir()
	if want := filepath.Join(root, "data", "gx"); data != want {
		t.Errorf("DataDir() = %q, want %q", data, want)
	}
	state, _ := StateDir()
	if want := filepath.Join(root, "state", "gx"); state != want {
		t.Errorf("StateDir() = %q, want %q", state, want)
	}
}

func TestDirs_IgnoreRelativeXDGEnv(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", "relative/data")

	data, _ := DataDir()
	if want := filepath.Join(home, ".local", "share", "gx"); data != want {
		t.Errorf("DataDir() = %q, want %q", data, want)
	}
}

func TestTicketStorePath_DefaultAndOverride(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "data"))

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(root, "data", "gx", "tickets"); cfg.TicketStore.Path != want {
		t.Errorf("default ticket-store.path = %q, want %q", cfg.TicketStore.Path, want)
	}

	path, err := FilePath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	custom := filepath.Join(root, "custom")
	if err := os.WriteFile(path, []byte(`{"ticket-store":{"path":"`+custom+`"}}`), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TicketStore.Path != custom {
		t.Errorf("ticket-store.path = %q, want %q", cfg.TicketStore.Path, custom)
	}
}
