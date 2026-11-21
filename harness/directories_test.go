package harness

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/elentok/gx/agentrunner"
)

func TestWritableDirectoryArgs(t *testing.T) {
	t.Parallel()
	for _, kind := range []agentrunner.Kind{Claude, Codex} {
		t.Run(string(kind), func(t *testing.T) {
			var args []string
			if kind == Codex {
				args = CodexArgs(CodexArgsInput{WritableRoots: []string{"/tickets"}})
			}
			original := slices.Clone(args)
			got := WritableDirectoryArgs(kind, args, []string{"~/Library/pnpm", "cache", "~/Library/pnpm"}, "/isolated/home", "", "/repo")
			joined := strings.Join(got, " ")
			for _, path := range []string{"/isolated/home/Library/pnpm", "/repo/cache"} {
				if strings.Count(joined, path) != 1 {
					t.Fatalf("expected one %s in %v", path, got)
				}
			}
			if !slices.Equal(args, original) {
				t.Fatal("mutated caller arguments")
			}
			if kind == Codex && (!strings.Contains(joined, "/tickets") || strings.Count(joined, "writable_roots=") != 1) {
				t.Fatalf("existing roots not preserved: %v", got)
			}
			if got := WritableDirectoryArgs(kind, args, nil, "/home", "", "/repo"); !slices.Equal(got, args) {
				t.Fatal("empty list changed arguments")
			}
		})
	}
}

// claude asks about each --add-dir directory on every new session; a settings
// additionalDirectories entry gives the same access without asking.
func TestWritableDirectoryArgs_ClaudeUsesSettingsNotAddDir(t *testing.T) {
	t.Parallel()
	args := []string{"--settings", `{"permissions":{"allow":["Read"]}}`, "--model", "m"}
	got := WritableDirectoryArgs(Claude, args, []string{"/a", "/b"}, "/home", "", "/repo")
	if slices.Contains(got, "--add-dir") {
		t.Fatalf("--add-dir in %v", got)
	}
	var settings []string
	for i, a := range got {
		if a == "--settings" {
			settings = append(settings, got[i+1])
		}
	}
	if len(settings) != 1 {
		t.Fatalf("want one --settings flag, got %v", got)
	}
	var parsed struct {
		Permissions struct {
			Allow                 []string `json:"allow"`
			AdditionalDirectories []string `json:"additionalDirectories"`
		} `json:"permissions"`
	}
	if err := json.Unmarshal([]byte(settings[0]), &parsed); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(parsed.Permissions.AdditionalDirectories, []string{"/a", "/b"}) || !slices.Equal(parsed.Permissions.Allow, []string{"Read"}) {
		t.Fatalf("settings = %s", settings[0])
	}
}

// A -c writable_roots override replaces config.toml's list, so the launch
// carries the config's roots too, even with no configured directories.
func TestWritableDirectoryArgs_CodexKeepsConfigRoots(t *testing.T) {
	t.Parallel()
	codexHome := t.TempDir()
	config := `model = "x"

[sandbox_workspace_write]
# comment
writable_roots = [
  "/Users/me/Library/Mobile Documents/com~apple~CloudDocs/Pages",
  "relative/skipped",
  "/Users/me/work",
]
`
	if err := os.WriteFile(filepath.Join(codexHome, "config.toml"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	args := CodexArgs(CodexArgsInput{WritableRoots: []string{"/repo/.git"}})
	for _, dirs := range [][]string{nil, {"/extra", "/Users/me/work"}} {
		got := strings.Join(WritableDirectoryArgs(Codex, args, dirs, "/home", codexHome, "/repo"), " ")
		if strings.Count(got, "writable_roots=") != 1 || strings.Contains(got, "relative/skipped") {
			t.Fatalf("dirs %v: %s", dirs, got)
		}
		for _, want := range []string{"/repo/.git", "/Users/me/Library/Mobile Documents/com~apple~CloudDocs/Pages", "/Users/me/work"} {
			if strings.Count(got, want) != 1 {
				t.Fatalf("dirs %v: want one %q in %s", dirs, want, got)
			}
		}
	}
}

func TestCodexConfigWritableRoots_MissingOrBadConfig(t *testing.T) {
	t.Parallel()
	if got := CodexConfigWritableRoots(""); got != nil {
		t.Errorf("empty home: %v", got)
	}
	dir := t.TempDir()
	if got := CodexConfigWritableRoots(dir); got != nil {
		t.Errorf("missing config: %v", got)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte("not = [toml"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := CodexConfigWritableRoots(dir); got != nil {
		t.Errorf("bad config: %v", got)
	}
}
