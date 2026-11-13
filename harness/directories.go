package harness

import (
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"

	"github.com/elentok/gx/agentrunner"
)

// WritableDirectoryArgs adds configured directories while preserving existing
// sandbox roots. Home comes from the launch environment, never the process.
func WritableDirectoryArgs(kind agentrunner.Kind, args, directories []string, home, cwd string) []string {
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
	if len(dirs) == 0 {
		return args
	}
	args = slices.Clone(args)
	switch kind {
	case Claude:
		for _, dir := range dedupe(dirs) {
			args = append(args, "--add-dir", dir)
		}
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
