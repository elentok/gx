package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseMatchesLoad(t *testing.T) {
	tmp := t.TempDir()
	prev := userConfigDirFn
	userConfigDirFn = func() (string, error) { return tmp, nil }
	t.Cleanup(func() { userConfigDirFn = prev })

	data := []byte(`{
		"execution-queue": {"max-agents": 7, "max-agents-per-epic": 0},
		"budget": {"soft-limit": 50, "hard-limit": 40},
		"server": {"log-retention": "48h", "tab-env": ["A=1"]},
		"recovery": {"disabled": ["R1"], "notify-hold": "2m"},
		"evolve": "maintainer"
	}`)
	if err := os.MkdirAll(filepath.Join(tmp, "gx"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "gx", "config.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	parsed, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !reflect.DeepEqual(parsed, loaded) {
		t.Fatalf("Parse = %+v\nLoad  = %+v", parsed, loaded)
	}
}

func TestParseEmptyObjectIsDefault(t *testing.T) {
	cfg, err := Parse([]byte(`{}`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !reflect.DeepEqual(cfg, Default()) {
		t.Fatalf("Parse({}) = %+v, want Default()", cfg)
	}
}

func TestParseErrors(t *testing.T) {
	for name, tc := range map[string]struct{ data, want string }{
		"malformed":   {`{"evolve":`, "parse"},
		"bad evolve":  {`{"evolve":"sometimes"}`, "evolve must be"},
		"bad harness": {`{"agents":{"default-harness":"x"}}`, "agents.default-harness must be"},
		"wrong types": {`{"budget":{"soft-limit":"x"}}`, "parse"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Parse([]byte(tc.data))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Parse(%s) err = %v, want containing %q", tc.data, err, tc.want)
			}
		})
	}
}

func TestLoadErrorNamesPath(t *testing.T) {
	tmp := t.TempDir()
	prev := userConfigDirFn
	userConfigDirFn = func() (string, error) { return tmp, nil }
	t.Cleanup(func() { userConfigDirFn = prev })
	if err := os.MkdirAll(filepath.Join(tmp, "gx"), 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(tmp, "gx", "config.json")
	if err := os.WriteFile(path, []byte(`{"evolve":"sometimes"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), path) {
		t.Fatalf("Load err = %v, want naming %s", err, path)
	}
}
