package harness

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/elentok/gx/agentrunner"
)

// WritableDirectoryArgs adds configured directories while preserving existing
// sandbox roots. Home comes from the launch environment, never the process.
//
// Claude gets them as settings additionalDirectories, never --add-dir: claude
// asks about every --add-dir directory on each new session, and a settings
// entry gives the same access without the dialog.
//
// Codex gets them in a -c writable_roots override. That override replaces the
// list in codexHome's config.toml, so the roots listed there are merged in
// too. An empty codexHome skips the config file.
func WritableDirectoryArgs(kind agentrunner.Kind, args, directories []string, home, codexHome, cwd string) []string {
	var dirs []string
	for _, dir := range directories {
		if dir == "" {
			continue
		}
		if dir == "~" || strings.HasPrefix(dir, "~/") {
			if home == "" {
				continue
			}
			dir = filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(dir, "~"), "/"))
		}
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(cwd, dir)
		}
		dirs = append(dirs, filepath.Clean(dir))
	}
	if kind == Codex {
		dirs = append(CodexConfigWritableRoots(codexHome), dirs...)
	}
	if len(dirs) == 0 {
		return args
	}
	args = slices.Clone(args)
	switch kind {
	case Claude:
		data, _ := json.Marshal(map[string]any{"permissions": map[string]any{"additionalDirectories": dedupe(dirs)}})
		return MergeClaudeSettings(append(args, settingsFlag, string(data)))
	case Codex:
		const key = "sandbox_workspace_write.writable_roots="
		for i, arg := range args {
			if strings.HasPrefix(arg, key) {
				var roots []string
				if json.Unmarshal([]byte(strings.TrimPrefix(arg, key)), &roots) == nil {
					args[i] = key + tomlStringArray(dedupe(append(roots, dirs...)))
					return args
				}
			}
		}
		args = append(args, "-c", key+tomlStringArray(dedupe(dirs)))
	}
	return args
}

// CodexConfigWritableRoots is the sandbox_workspace_write.writable_roots list
// in codexHome's config.toml: nil when the file is missing or unreadable.
// Only absolute paths are kept, since Codex resolves the rest against a cwd gx
// does not know here.
func CodexConfigWritableRoots(codexHome string) []string {
	if codexHome == "" {
		return nil
	}
	data, err := os.ReadFile(filepath.Join(codexHome, "config.toml"))
	if err != nil {
		return nil
	}
	var cfg struct {
		SandboxWorkspaceWrite struct {
			WritableRoots []string `toml:"writable_roots"`
		} `toml:"sandbox_workspace_write"`
	}
	if _, err := toml.Decode(string(data), &cfg); err != nil {
		return nil
	}
	var roots []string
	for _, r := range cfg.SandboxWorkspaceWrite.WritableRoots {
		if filepath.IsAbs(r) {
			roots = append(roots, filepath.Clean(r))
		}
	}
	return roots
}
