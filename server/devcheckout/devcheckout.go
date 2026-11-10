// Package devcheckout finds the checkout Dev mode runs Vite from.
package devcheckout

import (
	"os"
	"path/filepath"
	"strings"
)

// Resolve returns the Dev checkout root, or "" when there is none. A set
// override always wins, even when invalid: it never falls back to stamp.
// A leading ~ is expanded with home; any other value must be absolute. A path
// counts only when it holds webui/package.json.
func Resolve(override, stamp, home string) string {
	path := stamp
	if override != "" {
		path = override
		if path == "~" || strings.HasPrefix(path, "~/") {
			path = filepath.Join(home, path[1:])
		}
	}
	if !filepath.IsAbs(path) {
		return ""
	}
	if _, err := os.Stat(filepath.Join(path, "webui", "package.json")); err != nil {
		return ""
	}
	return filepath.Clean(path)
}
