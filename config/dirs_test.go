package config

import (
	"os"
	"path/filepath"
	"strings"
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

func TestDirsFrom(t *testing.T) {
	// The process HOME points at a decoy that holds a config file; DirsFrom
	// must never see it.
	decoy := t.TempDir()
	decoyCfg := filepath.Join(decoy, ".config", "gx", "config.json")
	if err := os.MkdirAll(filepath.Dir(decoyCfg), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(decoyCfg, []byte(`{"use-nerdfont-icons":false}`), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", decoy)
	t.Setenv("XDG_STATE_HOME", filepath.Join(decoy, "state"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(decoy, "data"))

	home := t.TempDir()
	abs := t.TempDir()
	tests := []struct {
		name                          string
		env                           []string
		data, state, userState, uconf string
		cache                         string
	}{
		{
			name:      "HOME alone",
			env:       []string{"HOME=" + home},
			data:      filepath.Join(home, ".local", "share", "gx"),
			state:     filepath.Join(home, ".local", "state", "gx"),
			userState: filepath.Join(home, ".local", "state"),
			uconf:     filepath.Join(home, ".config"),
			cache:     filepath.Join(home, ".cache"),
		},
		{
			name:      "absolute XDG wins, XDG_CONFIG_HOME and XDG_CACHE_HOME ignored",
			env:       []string{"HOME=" + home, "XDG_DATA_HOME=" + abs + "/d", "XDG_STATE_HOME=" + abs + "/s", "XDG_CONFIG_HOME=" + abs + "/c", "XDG_CACHE_HOME=" + abs + "/k"},
			data:      filepath.Join(abs, "d", "gx"),
			state:     filepath.Join(abs, "s", "gx"),
			userState: filepath.Join(abs, "s"),
			uconf:     filepath.Join(home, ".config"),
			cache:     filepath.Join(home, ".cache"),
		},
		{
			name:      "relative XDG ignored",
			env:       []string{"HOME=" + home, "XDG_DATA_HOME=rel/d", "XDG_STATE_HOME=rel/s"},
			data:      filepath.Join(home, ".local", "share", "gx"),
			state:     filepath.Join(home, ".local", "state", "gx"),
			userState: filepath.Join(home, ".local", "state"),
			uconf:     filepath.Join(home, ".config"),
			cache:     filepath.Join(home, ".cache"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := DirsFrom(tt.env)
			for _, c := range []struct {
				name string
				got  func() (string, error)
				want string
			}{
				{"DataDir", d.DataDir, tt.data},
				{"StateDir", d.StateDir, tt.state},
				{"UserStateDir", d.UserStateDir, tt.userState},
				{"UserConfigDir", d.UserConfigDir, tt.uconf},
				{"UserCacheDir", d.UserCacheDir, tt.cache},
			} {
				got, err := c.got()
				if err != nil || got != c.want {
					t.Errorf("%s = %q, %v; want %q", c.name, got, err, c.want)
				}
			}
			cfg, err := d.Load()
			if err != nil {
				t.Fatal(err)
			}
			if !cfg.UseNerdFontIcons {
				t.Error("Load read the decoy process HOME's config")
			}
			if want := filepath.Join(tt.data, "tickets"); cfg.TicketStore.Path != want {
				t.Errorf("ticket-store.path = %q, want %q", cfg.TicketStore.Path, want)
			}
		})
	}

	t.Run("no HOME is an error", func(t *testing.T) {
		d := DirsFrom(nil)
		for name, f := range map[string]func() (string, error){
			"DataDir": d.DataDir, "StateDir": d.StateDir, "UserStateDir": d.UserStateDir,
			"UserConfigDir": d.UserConfigDir, "UserCacheDir": d.UserCacheDir, "FilePath": d.FilePath, "Init": d.Init,
		} {
			if _, err := f(); err == nil || !strings.Contains(err.Error(), "no HOME in env") {
				t.Errorf("%s error = %v, want no HOME in env", name, err)
			}
		}
		if _, err := d.Load(); err == nil {
			t.Error("Load with no HOME: want error")
		}
	})

	t.Run("Load reads config under the given env", func(t *testing.T) {
		p := filepath.Join(home, ".config", "gx", "config.json")
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(`{"use-nerdfont-icons":false}`), 0644); err != nil {
			t.Fatal(err)
		}
		cfg, err := DirsFrom([]string{"HOME=" + home}).Load()
		if err != nil || cfg.UseNerdFontIcons {
			t.Errorf("Load = %+v, %v; want nerd-font icons off", cfg.UseNerdFontIcons, err)
		}
	})
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

func TestTicketStoreCommitDebounce_DefaultAndOverride(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TicketStore.CommitDebounce != 60 {
		t.Errorf("default ticket-store.commit-debounce = %d, want 60", cfg.TicketStore.CommitDebounce)
	}

	path, err := FilePath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"ticket-store":{"commit-debounce":5}}`), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TicketStore.CommitDebounce != 5 {
		t.Errorf("ticket-store.commit-debounce = %d, want 5", cfg.TicketStore.CommitDebounce)
	}
}
